package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
	"github.com/iban-pizza/iban-pizza/internal/embedded"
	"github.com/iban-pizza/iban-pizza/internal/sources"
)

func runImport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import", flag.ExitOnError)

	var data dataSource
	data.bind(fs)

	country := fs.String("country", "", "ISO 3166-1 alpha-2 code of the file's country")
	file := fs.String("file", "", "path to the registry file to import")
	snapshot := fs.String("snapshot", "",
		`copy every source from a snapshot instead: a file path, or "embedded" for the one built into this binary`)
	source := fs.String("source", "", "source name to record; defaults to the registry's name")
	format := fs.String("format", "auto",
		`"auto" uses the built in parser for the country, "generic" reads the documented CSV layout`)
	dryRun := fs.Bool("dry-run", false, "parse and report but do not store anything")
	snapshotPath := fs.String("write-snapshot", "", "also write the result to this snapshot file")
	logFormat := fs.String("log-format", envOr("IBAN_PIZZA_LOG_FORMAT", "text"), "json or text")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *snapshot != "" {
		if *country != "" || *file != "" || *format != "auto" {
			return fmt.Errorf("-snapshot cannot be combined with -country, -file or -format")
		}
		return importSnapshot(ctx, &data, *snapshot, *dryRun, *snapshotPath, newLogger(*logFormat, "info"))
	}
	if *country == "" || *file == "" {
		return fmt.Errorf("both -country and -file are required (or use -snapshot)")
	}
	cc := strings.ToUpper(strings.TrimSpace(*country))
	if len(cc) != 2 {
		return fmt.Errorf("-country must be a two letter code, got %q", *country)
	}

	log := newLogger(*logFormat, "info")

	raw, err := os.ReadFile(*file)
	if err != nil {
		return fmt.Errorf("read %s: %w", *file, err)
	}
	if len(raw) == 0 {
		return fmt.Errorf("%s is empty", *file)
	}

	var (
		banks    []bankdata.Bank
		recorded string
	)
	switch *format {
	case "auto":
		banks, recorded, err = sources.ParseFile(cc, *source, raw)
	case "generic":
		banks, err = sources.ParseGeneric(cc, *source, raw)
		recorded = *source
		if recorded == "" {
			recorded = sources.GenericName
		}
	default:
		return fmt.Errorf("-format must be auto or generic, got %q", *format)
	}
	if err != nil {
		return fmt.Errorf("parse %s: %w", *file, err)
	}

	// A file for one country must not smuggle in records for another, unless
	// the generic format was asked for and carries its own country column.
	for _, b := range banks {
		if b.Country != cc && *format != "generic" {
			return fmt.Errorf("parser produced a record for %s from a file declared as %s", b.Country, cc)
		}
	}

	log.Info("parsed", "country", cc, "source", recorded, "records", len(banks), "file", *file)
	if *dryRun {
		log.Info("dry run, nothing stored")
		return nil
	}

	repo, writer, closeStore, err := data.openWritableStore(ctx, log)
	if err != nil {
		return err
	}
	defer closeStore()

	info := bankdata.SourceInfo{
		Country:     cc,
		Name:        recorded,
		URL:         "file://" + *file,
		RetrievedAt: fileTime(*file),
		RecordCount: len(banks),
	}
	if err := writer.Replace(ctx, info, banks); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	log.Info("stored", "country", cc, "source", recorded, "records", len(banks))

	if *snapshotPath != "" {
		if err := bankdata.SaveSnapshotFile(ctx, *snapshotPath, repo); err != nil {
			return fmt.Errorf("write snapshot: %w", err)
		}
		log.Info("snapshot written", "path", *snapshotPath)
	}
	return nil
}

// fileTime uses the file's modification time as the retrieval time, since a
// file handed to import was obtained at some earlier point. Falling back to
// now would make stale data look fresh.
func fileTime(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime().UTC()
	}
	return time.Now().UTC()
}

// importSnapshot copies every source of a snapshot into the target store.
//
// With "embedded" this is the secure way to fill a database: the data was
// fetched, tested and reviewed in the release pipeline and shipped inside the
// binary, and the host running this never contacts a publisher. It is also
// idempotent, so running it on every rollout keeps the database in step with
// the image.
func importSnapshot(ctx context.Context, data *dataSource, from string, dryRun bool, snapshotPath string, log *slog.Logger) error {
	var (
		src *bankdata.MemoryStore
		err error
	)
	if from == "embedded" {
		src, err = embedded.Load()
		if err != nil {
			return fmt.Errorf("load embedded snapshot: %w", err)
		}
	} else {
		src, err = bankdata.LoadSnapshotFile(from)
		if err != nil {
			return fmt.Errorf("load %s: %w", from, err)
		}
	}

	stats, err := src.Stats(ctx)
	if err != nil {
		return err
	}
	for _, s := range stats.Sources {
		log.Info("snapshot source", "country", s.Country, "source", s.Name,
			"records", s.RecordCount, "retrieved", s.RetrievedAt.Format("2006-01-02"))
	}
	if dryRun {
		log.Info("dry run, nothing stored", "sources", len(stats.Sources), "records", stats.TotalRecords)
		return nil
	}

	repo, writer, closeStore, err := data.openWritableStore(ctx, log)
	if err != nil {
		return err
	}
	defer closeStore()

	copied, err := bankdata.CopySources(ctx, src, writer)
	for _, c := range copied {
		log.Info("stored", "country", c.Country, "source", c.Name, "records", c.RecordCount)
	}
	if err != nil {
		return fmt.Errorf("copy snapshot: %w", err)
	}

	if snapshotPath != "" {
		if err := bankdata.SaveSnapshotFile(ctx, snapshotPath, repo); err != nil {
			return fmt.Errorf("write snapshot: %w", err)
		}
		log.Info("snapshot written", "path", snapshotPath)
	}
	return nil
}
