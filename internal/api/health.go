package api

import (
	"net/http"
	"time"
)

// Health is the body of the liveness endpoint.
//
// It is deliberately small. Orchestrators poll it every few seconds, and the
// answer to "is the service up and is its data current" fits in five fields.
// Where the data came from, per country and with dates, is a question API
// consumers ask rather than load balancers, so it lives at /v2/data.
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
	Records int    `json:"records"`
	Stale   bool   `json:"stale"`
}

// handleHealth reports whether the service is up and whether its data is old.
//
// The age matters as much as the liveness. The upstream project shipped bank
// data that silently froze in 2019 and kept answering, so staleness is part of
// the health signal rather than something an operator has to infer.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	h := Health{
		Status:  "ok",
		Version: s.cfg.Version,
		Uptime:  time.Since(s.started).Round(time.Second).String(),
	}

	stats, err := s.cfg.Repo.Stats(r.Context())
	if err != nil {
		h.Status = "degraded"
		writeJSON(w, http.StatusInternalServerError, h)
		return
	}
	h.Records = stats.TotalRecords

	now := time.Now()
	for _, src := range stats.Sources {
		if now.Sub(src.RetrievedAt) > s.cfg.StaleAfter {
			h.Stale = true
			h.Status = "stale"
			break
		}
	}

	// Stale data is a warning, not an outage: the service still answers, and
	// returning a failure here would take a working instance out of a load
	// balancer for a data problem.
	writeJSON(w, http.StatusOK, h)
}

// handleReady reports whether the service can answer lookups at all.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	stats, err := s.cfg.Repo.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "data store unavailable"})
		return
	}
	if stats.TotalRecords == 0 {
		writeJSON(w, http.StatusServiceUnavailable, errorBody{Error: "no bank data loaded"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "records": stats.TotalRecords})
}

// DataSource describes one loaded registry.
type DataSource struct {
	Country     string `json:"country"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	RetrievedAt string `json:"retrievedAt"`
	AgeDays     int    `json:"ageDays"`
	Records     int    `json:"records"`
	Stale       bool   `json:"stale"`
}

// DataSchemes describes the loaded EPC register.
type DataSchemes struct {
	Loaded       bool           `json:"loaded"`
	Institutions int            `json:"institutions,omitempty"`
	AsOf         string         `json:"asOf,omitempty"`
	Participants map[string]int `json:"participants,omitempty"`
}

// DataReport is the body of /v2/data: what the service is answering from.
//
// This is the provenance an integrator needs to judge an answer. "Bank code
// valid" means something different when the German register is two days old
// than when it is two years old, and a caller should be able to tell without
// reading logs.
type DataReport struct {
	Records    int          `json:"records"`
	Countries  []string     `json:"countries"`
	Stale      bool         `json:"stale"`
	StaleAfter string       `json:"staleAfter"`
	Sources    []DataSource `json:"sources"`
	Schemes    DataSchemes  `json:"schemes"`
}

func (s *Server) handleV2Data(w http.ResponseWriter, r *http.Request) {
	stats, err := s.cfg.Repo.Stats(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "data store unavailable"})
		return
	}

	report := DataReport{
		Records:    stats.TotalRecords,
		Countries:  []string{},
		StaleAfter: s.cfg.StaleAfter.String(),
		Sources:    []DataSource{},
	}

	now := time.Now()
	for _, src := range stats.Sources {
		age := now.Sub(src.RetrievedAt)
		stale := age > s.cfg.StaleAfter
		if stale {
			report.Stale = true
		}
		report.Countries = append(report.Countries, src.Country)
		report.Sources = append(report.Sources, DataSource{
			Country:     src.Country,
			Name:        src.Name,
			URL:         src.URL,
			RetrievedAt: src.RetrievedAt.UTC().Format(time.RFC3339),
			AgeDays:     int(age.Hours() / 24),
			Records:     src.RecordCount,
			Stale:       stale,
		})
	}

	if s.cfg.Schemes != nil {
		report.Schemes = DataSchemes{
			Loaded:       true,
			Institutions: s.cfg.Schemes.Len(),
			Participants: map[string]int{},
		}
		if t := s.cfg.Schemes.AsOf(); !t.IsZero() {
			report.Schemes.AsOf = t.Format("2006-01-02")
		}
		for scheme, n := range s.cfg.Schemes.Counts() {
			report.Schemes.Participants[string(scheme)] = n
		}
	}

	// Provenance changes only when data is reloaded, which is rare, but the
	// answer must never be served from a cache across a reload.
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, http.StatusOK, report)
}

// handleOpenAPI serves the API description embedded in the binary.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(openAPISpec)
}
