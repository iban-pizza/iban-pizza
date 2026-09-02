// Command openiban serves and maintains the iban.pizza service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
	"github.com/netzfabrikcom/iban-pizza/bankdata/postgres"
	"github.com/netzfabrikcom/iban-pizza/internal/embedded"
)

// version is set at link time with -ldflags "-X main.version=..."
var version = "dev"

const usage = `openiban serves IBAN validation and bank lookup.

Usage:
  openiban serve      [flags]   Run the HTTP service
  openiban update     [flags]   Refresh bank data from the official registries
  openiban snapshot   [flags]   Write the current data to a snapshot file
  openiban version              Print the version

Run "openiban <command> -h" for the flags of a command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(ctx, os.Args[2:])
	case "update":
		err = runUpdate(ctx, os.Args[2:])
	case "snapshot":
		err = runSnapshot(ctx, os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println(version)
		return
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "openiban: %v\n", err)
		os.Exit(1)
	}
}

// newLogger builds the structured logger.
func newLogger(format, level string) *slog.Logger {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(level)); err != nil {
		lv = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: lv}

	if strings.EqualFold(format, "text") {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, opts))
}

// dataSource describes where a command should read or write bank data.
type dataSource struct {
	file        string
	databaseURL string
}

func (d *dataSource) bind(fs *flag.FlagSet) {
	fs.StringVar(&d.file, "data-file", envOr("OPENIBAN_DATA_FILE", ""),
		"path to a snapshot file, overriding the snapshot built into the binary")
	fs.StringVar(&d.databaseURL, "database-url", envOr("OPENIBAN_DATABASE_URL", ""),
		"PostgreSQL connection string, used instead of a snapshot")
}

// openRepository resolves the configured backend.
//
// The order is deliberate: an explicit database or file wins, and the embedded
// snapshot is the fallback that makes the binary work with no configuration.
func (d *dataSource) openRepository(ctx context.Context, log *slog.Logger) (bankdata.Repository, func(), error) {
	switch {
	case d.databaseURL != "":
		store, err := postgres.Open(ctx, d.databaseURL)
		if err != nil {
			return nil, nil, fmt.Errorf("open database: %w", err)
		}
		log.Info("using the PostgreSQL backend")
		return store, store.Close, nil

	case d.file != "":
		store, err := bankdata.LoadSnapshotFile(d.file)
		if err != nil {
			return nil, nil, fmt.Errorf("load %s: %w", d.file, err)
		}
		log.Info("using a snapshot file", "path", d.file, "records", store.Len())
		return store, func() {}, nil

	default:
		store, err := embedded.Load()
		if err != nil {
			if errors.Is(err, embedded.ErrEmpty) {
				return nil, nil, errors.New(
					"this binary has no embedded snapshot; pass -data-file or -database-url, " +
						"or run \"openiban update --write-snapshot\" before building")
			}
			return nil, nil, fmt.Errorf("load embedded snapshot: %w", err)
		}
		log.Info("using the embedded snapshot", "records", store.Len())
		return store, func() {}, nil
	}
}

// openWritableStore returns a store that can be written to.
func (d *dataSource) openWritableStore(ctx context.Context, log *slog.Logger) (bankdata.Repository, bankdata.Writer, func(), error) {
	if d.databaseURL != "" {
		store, err := postgres.Open(ctx, d.databaseURL)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("open database: %w", err)
		}
		log.Info("writing to the PostgreSQL backend")
		return store, store, store.Close, nil
	}

	// Start from whatever is already there so that refreshing one country does
	// not discard the others.
	store := bankdata.NewMemoryStore()
	if d.file != "" {
		if loaded, err := bankdata.LoadSnapshotFile(d.file); err == nil {
			store = loaded
			log.Info("starting from the existing snapshot", "path", d.file, "records", store.Len())
		}
	} else if embedded.Available() {
		if loaded, err := embedded.Load(); err == nil {
			store = loaded
		}
	}
	return store, store, func() {}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return fallback
}
