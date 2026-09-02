package api

import (
	"net/http"
	"time"
)

// SourceHealth reports the freshness of one data source.
type SourceHealth struct {
	Country     string `json:"country"`
	Name        string `json:"name"`
	RetrievedAt string `json:"retrievedAt"`
	AgeDays     int    `json:"ageDays"`
	Records     int    `json:"records"`
	Stale       bool   `json:"stale"`
}

// Health is the body of the health endpoint.
type Health struct {
	Status  string         `json:"status"`
	Version string         `json:"version"`
	Uptime  string         `json:"uptime"`
	Records int            `json:"records"`
	Stale   bool           `json:"stale"`
	Sources []SourceHealth `json:"sources"`
	Schemes *SchemeHealth  `json:"schemes,omitempty"`
}

// SchemeHealth reports the state of the EPC register.
type SchemeHealth struct {
	Institutions int    `json:"institutions"`
	AsOf         string `json:"asOf,omitempty"`
}

// handleHealth reports whether the service is up and how old its data is.
//
// The age matters as much as the liveness. The upstream project shipped bank
// data that silently froze in 2019 and kept answering, so freshness is a first
// class part of the health signal rather than something an operator has to
// infer.
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
		age := now.Sub(src.RetrievedAt)
		stale := age > s.cfg.StaleAfter
		if stale {
			h.Stale = true
		}
		h.Sources = append(h.Sources, SourceHealth{
			Country:     src.Country,
			Name:        src.Name,
			RetrievedAt: src.RetrievedAt.Format(time.RFC3339),
			AgeDays:     int(age.Hours() / 24),
			Records:     src.RecordCount,
			Stale:       stale,
		})
	}

	if s.cfg.Schemes != nil {
		sh := &SchemeHealth{Institutions: s.cfg.Schemes.Len()}
		if t := s.cfg.Schemes.AsOf(); !t.IsZero() {
			sh.AsOf = t.Format("2006-01-02")
		}
		h.Schemes = sh
	}

	if h.Stale {
		h.Status = "stale"
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

// handleOpenAPI serves the API description embedded in the binary.
func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(openAPISpec)
}
