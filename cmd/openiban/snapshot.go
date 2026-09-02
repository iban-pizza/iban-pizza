package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
)

func runSnapshot(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)

	var data dataSource
	data.bind(fs)

	out := fs.String("out", "internal/embedded/snapshot.jsonl.gz", "path to write the snapshot to")
	logFormat := fs.String("log-format", envOr("OPENIBAN_LOG_FORMAT", "text"), "json or text")

	if err := fs.Parse(args); err != nil {
		return err
	}

	log := newLogger(*logFormat, "info")

	repo, closeRepo, err := data.openRepository(ctx, log)
	if err != nil {
		return err
	}
	defer closeRepo()

	stats, err := repo.Stats(ctx)
	if err != nil {
		return fmt.Errorf("read stats: %w", err)
	}
	if stats.TotalRecords == 0 {
		return fmt.Errorf("refusing to write an empty snapshot")
	}

	if err := bankdata.SaveSnapshotFile(ctx, *out, repo); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}

	info, err := os.Stat(*out)
	if err != nil {
		return err
	}
	log.Info("snapshot written",
		"path", *out, "bytes", info.Size(),
		"records", stats.TotalRecords, "sources", len(stats.Sources))
	return nil
}
