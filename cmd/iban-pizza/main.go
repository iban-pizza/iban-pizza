// Command iban-pizza serves and maintains the iban.pizza service.
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

	"github.com/iban-pizza/iban-pizza/bankdata"
	"github.com/iban-pizza/iban-pizza/bankdata/postgres"
)

// version is set at link time with -ldflags "-X main.version=..."
var version = "dev"

const usage = `iban-pizza serves IBAN validation and bank lookup.

No bank data ships with this program. Run "iban-pizza update" once to download
it from the official registries; until then IBANs are checked structurally and
no bank is resolved.

Usage:
  iban-pizza serve      [flags]   Run the HTTP service
  iban-pizza update     [flags]   Download bank data from the official registries
  iban-pizza sources              List the registries update downloads from, with their terms
  iban-pizza import     [flags]   Load a registry file per country, or copy a data file into a store
  iban-pizza snapshot   [flags]   Write the current data to a separate file
  iban-pizza version              Print the version

Run "iban-pizza <command> -h" for the flags of a command.
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
	case "sources":
		err = runSources(os.Args[2:])
	case "import":
		err = runImport(ctx, os.Args[2:])
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
		fmt.Fprintf(os.Stderr, "iban-pizza: %v\n", err)
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

// defaultDataFile is where bank data lives when nothing else is configured.
//
// The program ships without any bank data: every registry has its own terms of
// use, and most of them allow a user to download and use the data but not a
// project to redistribute it. "iban-pizza update" therefore downloads it onto
// the machine that will use it, and this is where it lands.
const defaultDataFile = "data/bankdata.jsonl.gz"

// dataSource describes where a command should read or write bank data.
type dataSource struct {
	file        string
	databaseURL string
}

func (d *dataSource) bind(fs *flag.FlagSet) {
	fs.StringVar(&d.file, "data-file", envOr("IBAN_PIZZA_DATA_FILE", defaultDataFile),
		"path to the bank data file written by \"iban-pizza update\"")
	fs.StringVar(&d.databaseURL, "database-url", envOr("IBAN_PIZZA_DATABASE_URL", ""),
		"PostgreSQL connection string, used instead of the data file")
}

// openRepository resolves the configured backend.
//
// A missing data file is not an error. The service then starts with an empty
// store, which still validates the structure and check digits of an IBAN but
// resolves no bank, and says so loudly. Any other problem with the file is an
// error: a corrupt file must not be mistaken for a fresh installation.
func (d *dataSource) openRepository(ctx context.Context, log *slog.Logger) (bankdata.Repository, func(), error) {
	if d.databaseURL != "" {
		store, err := postgres.Open(ctx, d.databaseURL)
		if err != nil {
			return nil, nil, fmt.Errorf("open database: %w", err)
		}
		log.Info("using the PostgreSQL backend")
		return store, store.Close, nil
	}

	store, err := bankdata.LoadSnapshotFile(d.file)
	if errors.Is(err, os.ErrNotExist) {
		log.Warn("no bank data found, IBANs are checked structurally only; "+
			"run \"iban-pizza update\" to download the registries", "path", d.file)
		return bankdata.NewMemoryStore(), func() {}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load %s: %w", d.file, err)
	}
	log.Info("using the data file", "path", d.file, "records", store.Len())
	return store, func() {}, nil
}

// openWritableStore returns a store that can be written to, and a save
// function that makes the writes durable.
//
// For PostgreSQL every write is durable already and save does nothing. For the
// file backend the store lives in memory, so save writes it back to the data
// file. Forgetting to call it would make an update look successful and leave
// nothing behind.
func (d *dataSource) openWritableStore(ctx context.Context, log *slog.Logger) (repo bankdata.Repository, w bankdata.Writer, save func(context.Context) error, closeStore func(), err error) {
	if d.databaseURL != "" {
		store, err := postgres.Open(ctx, d.databaseURL)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("open database: %w", err)
		}
		log.Info("writing to the PostgreSQL backend")
		return store, store, func(context.Context) error { return nil }, store.Close, nil
	}

	// Start from whatever is already there so that refreshing one country does
	// not discard the others.
	store := bankdata.NewMemoryStore()
	loaded, err := bankdata.LoadSnapshotFile(d.file)
	switch {
	case err == nil:
		store = loaded
		log.Info("starting from the existing data file", "path", d.file, "records", store.Len())
	case errors.Is(err, os.ErrNotExist):
		// First run.
	default:
		return nil, nil, nil, nil, fmt.Errorf("load %s: %w", d.file, err)
	}

	save = func(ctx context.Context) error {
		if store.Len() == 0 {
			return nil
		}
		if err := bankdata.SaveSnapshotFile(ctx, d.file, store); err != nil {
			return fmt.Errorf("write %s: %w", d.file, err)
		}
		log.Info("data file written", "path", d.file, "records", store.Len())
		return nil
	}
	return store, store, save, func() {}, nil
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
