package api

import (
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// MaxRequestBody bounds a request body. Only the batch endpoint has one, and a
// batch of the documented size fits comfortably.
const MaxRequestBody = 1 << 20 // 1 MiB

// withRequestLimits caps the request body and sets headers that cost nothing
// and remove whole classes of problem.
func withRequestLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBody)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// withCORS grants cross origin access to the configured origins only.
//
// The upstream service sent "Access-Control-Allow-Origin: *" on every
// response. That is a reasonable choice for a public demo and a poor default
// for a service someone runs themselves, so it is configured rather than
// assumed.
func withCORS(next http.Handler, allowed []string) http.Handler {
	allowAll := slices.Contains(allowed, "*")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		switch {
		case origin == "":
			// Not a cross origin request.
		case allowAll:
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case slices.Contains(allowed, origin):
			w.Header().Set("Access-Control-Allow-Origin", origin)
			// The response varies by origin, so caches must not serve one
			// origin's response to another.
			w.Header().Add("Vary", "Origin")
		}

		if origin != "" && (allowAll || slices.Contains(allowed, origin)) {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withRecovery turns a panic into a 500 instead of a dropped connection.
func withRecovery(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("handler panicked", "panic", v, "path", r.URL.Path)
				writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code for logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// withLogging emits one structured line per request.
//
// The IBAN is deliberately not logged. It is an account identifier, and a
// validation service has no reason to keep a record of what people looked up.
func withLogging(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		log.Info("request",
			"method", r.Method,
			"route", r.Pattern,
			"status", rec.status,
			"duration", time.Since(start),
		)
	})
}

// limiter is a fixed window counter per client address.
//
// A window counter is coarser than a token bucket but needs no background
// goroutine and no third party package, and the goal here is to bound abuse
// rather than to shape traffic precisely.
type limiter struct {
	mu      sync.Mutex
	perMin  int
	windows map[string]*window
	last    time.Time
}

type window struct {
	count int
	start time.Time
}

func newLimiter(perMin int) *limiter {
	return &limiter{perMin: perMin, windows: make(map[string]*window), last: time.Now()}
}

// allow reports whether the client may proceed.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Drop expired entries occasionally so the map cannot grow without bound
	// as clients come and go. This is the failure the upstream cache had.
	if now.Sub(l.last) > time.Minute {
		for k, w := range l.windows {
			if now.Sub(w.start) > time.Minute {
				delete(l.windows, k)
			}
		}
		l.last = now
	}

	w, ok := l.windows[key]
	if !ok || now.Sub(w.start) > time.Minute {
		l.windows[key] = &window{count: 1, start: now}
		return true
	}
	if w.count >= l.perMin {
		return false
	}
	w.count++
	return true
}

// withRateLimit limits requests per client address per minute.
func withRateLimit(next http.Handler, perMin int) http.Handler {
	if perMin <= 0 {
		return next
	}
	l := newLimiter(perMin)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r), time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, errorBody{Error: "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP returns the address to rate limit on.
//
// Forwarded headers are not trusted: anyone can set them, so honouring them
// without knowing the proxy in front would let a caller bypass the limit by
// inventing an address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

// errorBody is the shape of a v2 error response.
type errorBody struct {
	Error string `json:"error"`
}
