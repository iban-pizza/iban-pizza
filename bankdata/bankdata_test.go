package bankdata

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func sampleBanks() []Bank {
	return []Bank{
		{Country: "DE", BankCode: "50010517", Name: "ING-DiBa", Zip: "60628",
			City: "Frankfurt am Main", BIC: "INGDDEFFXXX", CheckAlgo: "C1", Source: "Bundesbank"},
		{Country: "DE", BankCode: "37040044", Name: "Commerzbank",
			City: "Koeln", BIC: "COBADEFFXXX", CheckAlgo: "13", Source: "Bundesbank"},
		{Country: "DE", BankCode: "50010060", Name: "Postbank",
			City: "Frankfurt am Main", BIC: "PBNKDEFFXXX", Source: "Bundesbank"},
	}
}

func loadedStore(t *testing.T) *MemoryStore {
	t.Helper()
	s := NewMemoryStore()
	info := SourceInfo{Country: "DE", Name: "Bundesbank", RetrievedAt: time.Now().UTC()}
	if err := s.Replace(context.Background(), info, sampleBanks()); err != nil {
		t.Fatalf("Replace returned %v", err)
	}
	return s
}

func TestMemoryStoreFind(t *testing.T) {
	s := loadedStore(t)
	ctx := context.Background()

	got, err := s.Find(ctx, Key{Country: "DE", BankCode: "50010517"})
	if err != nil {
		t.Fatalf("Find returned %v", err)
	}
	if got.Name != "ING-DiBa" || got.CheckAlgo != "C1" {
		t.Errorf("Find returned %+v", got)
	}

	// Lookups must tolerate the casing and padding a caller might send.
	if _, err := s.Find(ctx, Key{Country: "de", BankCode: " 50010517 "}); err != nil {
		t.Errorf("normalised lookup returned %v", err)
	}

	if _, err := s.Find(ctx, Key{Country: "DE", BankCode: "00000000"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing bank returned %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreFindByBIC(t *testing.T) {
	s := loadedStore(t)
	got, err := s.FindByBIC(context.Background(), "ingddeffxxx")
	if err != nil {
		t.Fatalf("FindByBIC returned %v", err)
	}
	if len(got) != 1 || got[0].BankCode != "50010517" {
		t.Errorf("FindByBIC returned %+v", got)
	}
	if _, err := s.FindByBIC(context.Background(), "NOPEDEFFXXX"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown BIC returned %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreSearchIsDeterministic(t *testing.T) {
	s := loadedStore(t)
	ctx := context.Background()

	// Map iteration is random. Repeating a capped search must still return the
	// same page, otherwise paging silently loses and repeats records.
	first, err := s.Search(ctx, Query{Country: "DE", Limit: 2})
	if err != nil {
		t.Fatalf("Search returned %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("Search returned %d records, want 2", len(first))
	}
	for range 20 {
		again, err := s.Search(ctx, Query{Country: "DE", Limit: 2})
		if err != nil {
			t.Fatalf("Search returned %v", err)
		}
		if again[0].BankCode != first[0].BankCode || again[1].BankCode != first[1].BankCode {
			t.Fatalf("Search is not deterministic: %v then %v", first, again)
		}
	}
}

func TestSearchLimits(t *testing.T) {
	if got := (Query{}).clampLimit(); got != DefaultLimit {
		t.Errorf("zero limit clamped to %d, want %d", got, DefaultLimit)
	}
	if got := (Query{Limit: -5}).clampLimit(); got != DefaultLimit {
		t.Errorf("negative limit clamped to %d, want %d", got, DefaultLimit)
	}
	if got := (Query{Limit: MaxLimit + 1000}).clampLimit(); got != MaxLimit {
		t.Errorf("oversized limit clamped to %d, want %d", got, MaxLimit)
	}
}

func TestReplaceSwapsOnlyOneSource(t *testing.T) {
	s := loadedStore(t)
	ctx := context.Background()

	other := SourceInfo{Country: "CZ", Name: "CNB", RetrievedAt: time.Now().UTC()}
	czech := []Bank{{Country: "CZ", BankCode: "0100", Name: "Komercni banka",
		BIC: "KOMBCZPP", Source: "CNB"}}
	if err := s.Replace(ctx, other, czech); err != nil {
		t.Fatalf("Replace returned %v", err)
	}
	if s.Len() != 4 {
		t.Fatalf("store holds %d records, want 4", s.Len())
	}

	// Reloading Germany with fewer records must drop the missing ones and
	// leave the Czech records untouched.
	if err := s.Replace(ctx, SourceInfo{Country: "DE", Name: "Bundesbank",
		RetrievedAt: time.Now().UTC()}, sampleBanks()[:1]); err != nil {
		t.Fatalf("Replace returned %v", err)
	}
	if s.Len() != 2 {
		t.Errorf("store holds %d records, want 2", s.Len())
	}
	if _, err := s.Find(ctx, Key{Country: "CZ", BankCode: "0100"}); err != nil {
		t.Errorf("Czech record was lost when reloading Germany: %v", err)
	}
	if _, err := s.Find(ctx, Key{Country: "DE", BankCode: "37040044"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("stale German record survived the reload")
	}
}

func TestStatsOldestRetrieval(t *testing.T) {
	if _, ok := (Stats{}).OldestRetrieval(); ok {
		t.Error("empty stats reported a source")
	}
	old := time.Now().Add(-200 * 24 * time.Hour)
	st := Stats{Sources: []SourceInfo{
		{Name: "fresh", RetrievedAt: time.Now()},
		{Name: "old", RetrievedAt: old},
	}}
	got, ok := st.OldestRetrieval()
	if !ok || got.Name != "old" {
		t.Errorf("OldestRetrieval returned %+v, %v", got, ok)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	src := loadedStore(t)
	ctx := context.Background()

	var buf bytes.Buffer
	if err := WriteSnapshot(ctx, &buf, src); err != nil {
		t.Fatalf("WriteSnapshot returned %v", err)
	}

	got, err := ReadSnapshot(&buf)
	if err != nil {
		t.Fatalf("ReadSnapshot returned %v", err)
	}
	if got.Len() != src.Len() {
		t.Errorf("snapshot holds %d records, want %d", got.Len(), src.Len())
	}
	bank, err := got.Find(ctx, Key{Country: "DE", BankCode: "50010517"})
	if err != nil {
		t.Fatalf("Find after round trip returned %v", err)
	}
	if bank.CheckAlgo != "C1" || bank.BIC != "INGDDEFFXXX" {
		t.Errorf("record changed across the round trip: %+v", bank)
	}

	// The BIC index has to survive too, since it is rebuilt rather than stored.
	if _, err := got.FindByBIC(ctx, "INGDDEFFXXX"); err != nil {
		t.Errorf("BIC index was not rebuilt: %v", err)
	}

	stats, err := got.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats returned %v", err)
	}
	if len(stats.Sources) != 1 || stats.Sources[0].Name != "Bundesbank" {
		t.Errorf("source metadata was lost: %+v", stats.Sources)
	}
}

func TestSnapshotIsDeterministic(t *testing.T) {
	s := loadedStore(t)
	ctx := context.Background()

	// Two snapshots of identical content must be byte identical, otherwise
	// every rebuild produces a spurious diff in git.
	var a, b bytes.Buffer
	if err := WriteSnapshot(ctx, &a, s); err != nil {
		t.Fatalf("WriteSnapshot returned %v", err)
	}
	if err := WriteSnapshot(ctx, &b, s); err != nil {
		t.Fatalf("WriteSnapshot returned %v", err)
	}
	// The header carries a timestamp, so compare the record bodies only.
	ra, err := ReadSnapshot(bytes.NewReader(a.Bytes()))
	if err != nil {
		t.Fatalf("ReadSnapshot returned %v", err)
	}
	rb, err := ReadSnapshot(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatalf("ReadSnapshot returned %v", err)
	}
	la, _ := ra.Search(ctx, Query{Limit: MaxLimit})
	lb, _ := rb.Search(ctx, Query{Limit: MaxLimit})
	if len(la) != len(lb) {
		t.Fatalf("record counts differ: %d and %d", len(la), len(lb))
	}
	for i := range la {
		if la[i] != lb[i] {
			t.Errorf("record %d differs: %+v and %+v", i, la[i], lb[i])
		}
	}
}

func TestSnapshotRejectsBadInput(t *testing.T) {
	if _, err := ReadSnapshot(bytes.NewReader([]byte("not gzip"))); err == nil {
		t.Error("ReadSnapshot accepted non-gzip input")
	}

	// A future format version must be refused rather than parsed optimistically.
	future := newGzipLines(t, `{"version":99,"createdAt":"2026-01-01T00:00:00Z"}`)
	if _, err := ReadSnapshot(bytes.NewReader(future)); err == nil {
		t.Error("ReadSnapshot accepted an unsupported version")
	}

	// An empty stream and a corrupt record must both fail loudly.
	if _, err := ReadSnapshot(bytes.NewReader(newGzipLines(t))); err == nil {
		t.Error("ReadSnapshot accepted an empty snapshot")
	}
	corrupt := newGzipLines(t,
		`{"version":1,"createdAt":"2026-01-01T00:00:00Z"}`,
		`{"country":"DE","bankCode":`)
	if _, err := ReadSnapshot(bytes.NewReader(corrupt)); err == nil {
		t.Error("ReadSnapshot accepted a truncated record")
	}
}

func TestSaveAndLoadSnapshotFile(t *testing.T) {
	s := loadedStore(t)
	path := filepath.Join(t.TempDir(), "nested", "snapshot.jsonl.gz")

	if err := SaveSnapshotFile(context.Background(), path, s); err != nil {
		t.Fatalf("SaveSnapshotFile returned %v", err)
	}
	got, err := LoadSnapshotFile(path)
	if err != nil {
		t.Fatalf("LoadSnapshotFile returned %v", err)
	}
	if got.Len() != s.Len() {
		t.Errorf("loaded %d records, want %d", got.Len(), s.Len())
	}
	if _, err := LoadSnapshotFile(filepath.Join(t.TempDir(), "missing.gz")); err == nil {
		t.Error("LoadSnapshotFile accepted a missing file")
	}
}
