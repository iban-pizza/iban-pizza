package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
	"github.com/iban-pizza/iban-pizza/internal/sepa"
	"github.com/iban-pizza/iban-pizza/internal/sources"
)

func runUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ExitOnError)

	var data dataSource
	data.bind(fs)

	only := fs.String("countries", "", "comma separated country codes to refresh, default all")
	dryRun := fs.Bool("dry-run", false, "download and parse but do not store anything")
	allowPartial := fs.Bool("allow-partial", false,
		"exit successfully when at least one registry was downloaded, for unattended first starts")
	snapshotPath := fs.String("write-snapshot", "",
		"also write the result to this additional file")
	schemeDir := fs.String("scheme-dir", envOr("IBAN_PIZZA_SCHEME_FILE", ""),
		"also download the EPC scheme register into this directory")
	logFormat := fs.String("log-format", envOr("IBAN_PIZZA_LOG_FORMAT", "text"), "json or text")
	logLevel := fs.String("log-level", envOr("IBAN_PIZZA_LOG_LEVEL", "info"), "debug, info, warn or error")

	if err := fs.Parse(args); err != nil {
		return err
	}

	log := newLogger(*logFormat, *logLevel)
	client := sources.NewClient()

	// The data is downloaded to this machine from each publisher, under that
	// publisher's terms. Say so once per run, where an operator will see it.
	log.Info("downloading from the official registries; each publisher's terms apply to the data, " +
		"see \"iban-pizza sources\"")

	selected, err := selectSources(*only)
	if err != nil {
		return err
	}

	repo, writer, save, closeStore, err := data.openWritableStore(ctx, log)
	if err != nil {
		return err
	}
	defer closeStore()

	if *dryRun {
		// A dry run must not touch the real store, so writes go to a scratch
		// one and are discarded.
		writer = bankdata.NewMemoryStore()
		log.Info("dry run, nothing will be stored")
	}

	var failed int
	for _, src := range selected {
		start := time.Now()
		res := sources.Update(ctx, client, src, writer)
		if res.Err != nil {
			failed++
			log.Error("update failed", "country", res.Country, "source", res.Source, "error", res.Err)
			continue
		}
		if res.Warning != "" {
			log.Warn(res.Warning, "country", res.Country, "source", res.Source)
		}
		attrs := []any{
			"country", res.Country,
			"source", res.Source,
			"records", res.Records,
			"took", time.Since(start).Round(time.Millisecond),
		}
		if res.BICsJoined > 0 {
			attrs = append(attrs, "bicsJoined", res.BICsJoined)
		}
		log.Info("updated", attrs...)
	}

	if *schemeDir != "" {
		if err := updateSchemes(ctx, client, *schemeDir, *dryRun, log); err != nil {
			failed++
			log.Error("scheme register update failed", "error", err)
		}
	}

	// Save before reporting failures: the registries that did download are
	// worth keeping even when another publisher was unreachable.
	if !*dryRun {
		if err := save(ctx); err != nil {
			return err
		}
	}

	if *snapshotPath != "" && !*dryRun {
		if err := bankdata.SaveSnapshotFile(ctx, *snapshotPath, repo); err != nil {
			return fmt.Errorf("write snapshot: %w", err)
		}
		if info, err := os.Stat(*snapshotPath); err == nil {
			log.Info("snapshot written", "path", *snapshotPath, "bytes", info.Size())
		}
	}

	if failed > 0 {
		if *allowPartial && failed < len(selected) {
			log.Warn("some registries could not be downloaded, continuing with the rest",
				"failed", failed, "of", len(selected))
			return nil
		}
		return fmt.Errorf("%d of %d sources failed", failed, len(selected))
	}
	return nil
}

// selectSources resolves the country filter.
func selectSources(only string) ([]sources.Source, error) {
	if strings.TrimSpace(only) == "" {
		return sources.All(), nil
	}

	var out []sources.Source
	for _, code := range strings.Split(only, ",") {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code == "" {
			continue
		}
		src, ok := sources.Get(code)
		if !ok {
			return nil, fmt.Errorf("no source for %s, available: %s",
				code, strings.Join(sources.Countries(), ", "))
		}
		out = append(out, src)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no countries selected")
	}
	return out, nil
}

// updateSchemes refreshes the EPC register exports.
func updateSchemes(ctx context.Context, client *sources.Client, dir string, dryRun bool, log logger) error {
	files := make(map[sepa.Scheme][]byte, len(sepa.AllSchemes))
	var failed int

	for _, scheme := range sepa.AllSchemes {
		data, err := client.Get(ctx, sepa.URL(scheme))
		if err != nil {
			failed++
			log.Error("scheme download failed", "scheme", string(scheme), "error", err)
			continue
		}
		// Parse before storing so a truncated or changed export is caught here
		// rather than at the next start.
		participants, err := sepa.ParseCSV(data)
		if err != nil {
			failed++
			log.Error("scheme parse failed", "scheme", string(scheme), "error", err)
			continue
		}
		log.Info("scheme downloaded", "scheme", string(scheme), "participants", len(participants))
		files[scheme] = data
	}

	if len(files) == 0 {
		return fmt.Errorf("no scheme exports could be downloaded")
	}
	if dryRun {
		return nil
	}
	if err := sepa.SaveDir(dir, files); err != nil {
		return err
	}
	log.Info("scheme register written", "dir", filepath.Clean(dir), "schemes", len(files))
	return nil
}

// logger is the subset of slog.Logger this file needs.
type logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}
