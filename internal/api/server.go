// Package api serves the HTTP interface.
//
// Two surfaces live side by side. The v1 routes reproduce the openiban.com
// responses byte for byte so existing clients migrate without a code change,
// and the v2 routes carry the information v1 has no room for: scheme
// membership, account check digit results, logos and data provenance.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
	"github.com/netzfabrikcom/iban-pizza/internal/logo"
	"github.com/netzfabrikcom/iban-pizza/internal/sepa"
)

// Config configures the server.
type Config struct {
	// Repo supplies bank data.
	Repo bankdata.Repository

	// Schemes supplies EPC scheme membership. May be nil, in which case v2
	// omits the schemes block rather than reporting false negatives.
	Schemes *sepa.Registry

	// Logos resolves bank logos.
	Logos *logo.Service

	// Version is reported by the health endpoint.
	Version string

	// AllowedOrigins lists the origins permitted by CORS. Empty means no
	// cross origin access is granted. The upstream service allowed every
	// origin unconditionally; that is a deployment decision, not a default.
	AllowedOrigins []string

	// RateLimit is the number of requests per minute allowed per client
	// address. Zero disables limiting.
	RateLimit int

	// StaleAfter is how old the newest data may get before health reports the
	// dataset as stale. Zero uses DefaultStaleAfter.
	StaleAfter time.Duration

	// Logger receives request and error logs.
	Logger *slog.Logger
}

// DefaultStaleAfter is the age at which data counts as stale. The German
// register is republished quarterly, so a little over one quarter without a
// refresh means an update has been missed.
const DefaultStaleAfter = 120 * 24 * time.Hour

// Server holds the HTTP handlers.
type Server struct {
	cfg     Config
	started time.Time
}

// New returns a Server.
func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = DefaultStaleAfter
	}
	if cfg.Logos == nil {
		cfg.Logos = logo.New("")
	}
	return &Server{cfg: cfg, started: time.Now()}
}

// Handler returns the routed, wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// v1, kept byte compatible with openiban.com.
	mux.HandleFunc("GET /validate/{iban}", s.handleV1Validate)
	mux.HandleFunc("GET /countries", s.handleV1Countries)
	mux.HandleFunc("GET /calculate/{countryCode}/{bankCode}/{accountNumber}", s.handleV1Calculate)
	mux.HandleFunc("GET /v2/calculate/{countryCode}/{bankCode}/{accountNumber}", s.handleV1CalculateAndValidate)

	// v2.
	mux.HandleFunc("GET /v2/iban/{iban}", s.handleV2IBAN)
	mux.HandleFunc("POST /v2/iban:batch", s.handleV2Batch)
	mux.HandleFunc("GET /v2/banks", s.handleV2Banks)
	mux.HandleFunc("GET /v2/banks/{country}/{bankCode}", s.handleV2Bank)
	mux.HandleFunc("GET /v2/banks/{country}/{bankCode}/logo.svg", s.handleV2Logo)
	mux.HandleFunc("GET /v2/countries", s.handleV2Countries)
	mux.HandleFunc("GET /v2/data", s.handleV2Data)

	// Operations.
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)

	// The interface is registered last and catches every unmatched path.
	mux.HandleFunc("GET /", s.handleWeb)

	var h http.Handler = mux
	h = withRateLimit(h, s.cfg.RateLimit)
	h = withCORS(h, s.cfg.AllowedOrigins)
	h = withRequestLimits(h)
	h = withRecovery(h, s.cfg.Logger)
	h = withLogging(h, s.cfg.Logger)
	return h
}

// writeJSON encodes v as JSON.
//
// Everything goes through an encoder. The upstream service passed response
// bodies to fmt.Fprintf, which interpreted percent signs coming from the
// request as format verbs.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

// writeJSONIndent is the v1 encoder. The original used json.MarshalIndent with
// two spaces, and clients that compare response bodies would notice a change.
func writeJSONIndent(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, `{"error":"encoding failed"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
