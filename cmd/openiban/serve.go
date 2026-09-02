package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/netzfabrikcom/iban-pizza/internal/api"
	"github.com/netzfabrikcom/iban-pizza/internal/logo"
	"github.com/netzfabrikcom/iban-pizza/internal/sepa"
)

func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)

	var data dataSource
	data.bind(fs)

	addr := fs.String("addr", envOr("OPENIBAN_ADDR", ":8080"), "address to listen on")
	baseURL := fs.String("base-url", envOr("OPENIBAN_BASE_URL", ""),
		"public base URL, used to build absolute logo links")
	origins := fs.String("cors-origins", envOr("OPENIBAN_CORS_ORIGINS", ""),
		`comma separated allowed origins, or "*" for any (default: no cross origin access)`)
	rateLimit := fs.Int("rate-limit", envInt("OPENIBAN_RATE_LIMIT", 600),
		"requests per minute per client address, 0 disables the limit")
	staleAfter := fs.Duration("stale-after", envDuration("OPENIBAN_STALE_AFTER", api.DefaultStaleAfter),
		"age at which data is reported as stale")
	schemeFile := fs.String("scheme-file", envOr("OPENIBAN_SCHEME_FILE", ""),
		"directory holding EPC register CSV exports, enabling the schemes block")
	logFormat := fs.String("log-format", envOr("OPENIBAN_LOG_FORMAT", "json"), "json or text")
	logLevel := fs.String("log-level", envOr("OPENIBAN_LOG_LEVEL", "info"), "debug, info, warn or error")

	if err := fs.Parse(args); err != nil {
		return err
	}

	log := newLogger(*logFormat, *logLevel)

	repo, closeRepo, err := data.openRepository(ctx, log)
	if err != nil {
		return err
	}
	defer closeRepo()

	// Warn loudly at startup when the data is old. The upstream project served
	// data frozen in 2019 without ever saying so.
	if stats, err := repo.Stats(ctx); err == nil {
		if oldest, ok := stats.OldestRetrieval(); ok {
			age := time.Since(oldest.RetrievedAt)
			if age > *staleAfter {
				log.Warn("bank data is stale, run \"openiban update\"",
					"source", oldest.Name,
					"retrieved", oldest.RetrievedAt.Format(time.RFC3339),
					"ageDays", int(age.Hours()/24))
			}
		}
		log.Info("bank data loaded", "records", stats.TotalRecords, "sources", len(stats.Sources))
	}

	var schemes *sepa.Registry
	if *schemeFile != "" {
		schemes, err = sepa.LoadDir(*schemeFile)
		if err != nil {
			return fmt.Errorf("load scheme register: %w", err)
		}
		log.Info("scheme register loaded", "institutions", schemes.Len())
	}

	srv := api.New(api.Config{
		Repo:           repo,
		Schemes:        schemes,
		Logos:          logo.New(*baseURL),
		Version:        version,
		AllowedOrigins: splitOrigins(*origins),
		RateLimit:      *rateLimit,
		StaleAfter:     *staleAfter,
		Logger:         log,
	})

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv.Handler(),
		// Without these a slow client can hold a connection open forever. The
		// upstream service set none of them.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", *addr, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

// splitOrigins parses the comma separated origin list.
func splitOrigins(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
