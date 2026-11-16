package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// No bank data ships with the program, so a missing data file is the state of
// every fresh installation and must not stop the service from starting.
func TestOpenRepositoryWithoutDataFile(t *testing.T) {
	d := dataSource{file: filepath.Join(t.TempDir(), "absent.jsonl.gz")}

	repo, closeRepo, err := d.openRepository(context.Background(), quietLogger())
	if err != nil {
		t.Fatalf("openRepository returned %v for a missing data file", err)
	}
	defer closeRepo()

	stats, err := repo.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats returned %v", err)
	}
	if stats.TotalRecords != 0 {
		t.Errorf("an absent data file produced %d records", stats.TotalRecords)
	}
}

// A file that exists but cannot be read is a different matter: treating it as
// a fresh installation would silently serve nothing where data was expected.
func TestOpenRepositoryRejectsCorruptDataFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.jsonl.gz")
	if err := os.WriteFile(path, []byte("not a gzip stream"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := dataSource{file: path}

	if _, _, err := d.openRepository(context.Background(), quietLogger()); err == nil {
		t.Error("openRepository accepted a corrupt data file")
	}
	if _, _, _, _, err := d.openWritableStore(context.Background(), quietLogger()); err == nil {
		t.Error("openWritableStore accepted a corrupt data file, and would have overwritten it")
	}
}

// What update and import write has to be what serve reads, at the same path,
// and a second write must add to the first instead of replacing it.
func TestWritableStoreSavesToTheDataFile(t *testing.T) {
	ctx := context.Background()
	d := dataSource{file: filepath.Join(t.TempDir(), "data", "bankdata.jsonl.gz")}

	write := func(country, code string) {
		t.Helper()
		_, w, save, closeStore, err := d.openWritableStore(ctx, quietLogger())
		if err != nil {
			t.Fatalf("openWritableStore returned %v", err)
		}
		defer closeStore()
		info := bankdata.SourceInfo{Country: country, Name: "Registry " + country, RetrievedAt: time.Now().UTC()}
		bank := bankdata.Bank{Country: country, BankCode: code, Name: "Example Bank", Source: info.Name}
		if err := w.Replace(ctx, info, []bankdata.Bank{bank}); err != nil {
			t.Fatalf("Replace returned %v", err)
		}
		if err := save(ctx); err != nil {
			t.Fatalf("save returned %v", err)
		}
	}
	write("DE", "10000000")
	write("AT", "20000")

	repo, closeRepo, err := d.openRepository(ctx, quietLogger())
	if err != nil {
		t.Fatalf("openRepository returned %v", err)
	}
	defer closeRepo()
	for _, key := range []bankdata.Key{{Country: "DE", BankCode: "10000000"}, {Country: "AT", BankCode: "20000"}} {
		if _, err := repo.Find(ctx, key); err != nil {
			t.Errorf("Find(%v) returned %v after two writes", key, err)
		}
	}
}
