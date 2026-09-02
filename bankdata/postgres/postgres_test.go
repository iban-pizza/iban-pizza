package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
	"github.com/netzfabrikcom/iban-pizza/bankdata/postgres"
)

// testStore connects to the database named by OPENIBAN_TEST_DATABASE_URL.
//
// The tests skip when that variable is unset, so a plain "go test ./..." on a
// laptop stays green without a database. CI sets it against a service
// container, which is where the backend actually gets exercised.
func testStore(t *testing.T) *postgres.Store {
	t.Helper()

	url := os.Getenv("OPENIBAN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set OPENIBAN_TEST_DATABASE_URL to run the PostgreSQL tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := postgres.Open(ctx, url)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(store.Close)

	// Each test starts from an empty table. The database may hold real data
	// from an "openiban update" run, and a search assertion counting rows
	// would then measure that instead of the fixtures.
	//
	// Clearing goes through Replace rather than a truncate helper, so the
	// tests exercise only the public API and the package needs no method that
	// exists solely for testing.
	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	sources := map[string]bool{"Test Registry": true, "Other Registry": true}
	for _, src := range stats.Sources {
		sources[src.Name] = true
	}
	for name := range sources {
		if err := store.Replace(ctx, bankdata.SourceInfo{
			Country: "XX", Name: name, RetrievedAt: time.Now().UTC(),
		}, nil); err != nil {
			t.Fatalf("clear %s: %v", name, err)
		}
	}

	if after, err := store.Stats(ctx); err != nil {
		t.Fatalf("stats: %v", err)
	} else if after.TotalRecords != 0 {
		t.Fatalf("the table still holds %d records after clearing", after.TotalRecords)
	}
	return store
}

func fixtures() []bankdata.Bank {
	return []bankdata.Bank{
		{Country: "DE", BankCode: "50010517", Name: "ING-DiBa", Zip: "60628",
			City: "Frankfurt am Main", BIC: "INGDDEFFXXX", CheckAlgo: "C1", Source: "Test Registry"},
		{Country: "DE", BankCode: "37040044", Name: "Commerzbank", Zip: "50447",
			City: "Koeln", BIC: "COBADEFFXXX", CheckAlgo: "13", Source: "Test Registry"},
		{Country: "DE", BankCode: "10000000", Name: "Bundesbank",
			City: "Berlin", BIC: "MARKDEF1100", CheckAlgo: "09", Source: "Test Registry"},
	}
}

func seed(t *testing.T, store *postgres.Store) {
	t.Helper()
	info := bankdata.SourceInfo{
		Country: "DE", Name: "Test Registry",
		URL: "https://example.invalid/blz", RetrievedAt: time.Now().UTC(),
	}
	if err := store.Replace(context.Background(), info, fixtures()); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestFind(t *testing.T) {
	store := testStore(t)
	seed(t, store)
	ctx := context.Background()

	got, err := store.Find(ctx, bankdata.Key{Country: "DE", BankCode: "50010517"})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.Name != "ING-DiBa" || got.BIC != "INGDDEFFXXX" || got.CheckAlgo != "C1" {
		t.Errorf("Find returned %+v", got)
	}

	// Casing and padding must be normalised the same way the memory store
	// does it, otherwise a backend swap changes behaviour.
	if _, err := store.Find(ctx, bankdata.Key{Country: "de", BankCode: " 50010517 "}); err != nil {
		t.Errorf("normalised lookup: %v", err)
	}

	if _, err := store.Find(ctx, bankdata.Key{Country: "DE", BankCode: "00000000"}); !errors.Is(err, bankdata.ErrNotFound) {
		t.Errorf("missing bank returned %v, want ErrNotFound", err)
	}
}

func TestFindByBIC(t *testing.T) {
	store := testStore(t)
	seed(t, store)
	ctx := context.Background()

	got, err := store.FindByBIC(ctx, "ingddeffxxx")
	if err != nil {
		t.Fatalf("FindByBIC: %v", err)
	}
	if len(got) != 1 || got[0].BankCode != "50010517" {
		t.Errorf("FindByBIC returned %+v", got)
	}
	if _, err := store.FindByBIC(ctx, "NOPEDEFFXXX"); !errors.Is(err, bankdata.ErrNotFound) {
		t.Errorf("unknown BIC returned %v, want ErrNotFound", err)
	}
}

func TestSearch(t *testing.T) {
	store := testStore(t)
	seed(t, store)
	ctx := context.Background()

	all, err := store.Search(ctx, bankdata.Query{Country: "DE"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Search returned %d records, want 3", len(all))
	}
	// Ordering must be deterministic so that paging is stable.
	if all[0].BankCode != "10000000" {
		t.Errorf("results are not ordered by bank code: %+v", all[0])
	}

	byBIC, err := store.Search(ctx, bankdata.Query{BIC: "coba"})
	if err != nil {
		t.Fatalf("Search by BIC: %v", err)
	}
	if len(byBIC) != 1 || byBIC[0].Name != "Commerzbank" {
		t.Errorf("BIC prefix search returned %+v", byBIC)
	}

	byName, err := store.Search(ctx, bankdata.Query{Name: "bundes"})
	if err != nil {
		t.Fatalf("Search by name: %v", err)
	}
	if len(byName) != 1 {
		t.Errorf("case insensitive name search returned %d records", len(byName))
	}

	limited, err := store.Search(ctx, bankdata.Query{Country: "DE", Limit: 2})
	if err != nil {
		t.Fatalf("Search with limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("limit was not applied, got %d records", len(limited))
	}
}

// TestSearchTermsAreNotInterpreted checks that a search term cannot reach the
// statement. The conditions are built with placeholders, so these inputs must
// come back as ordinary misses rather than as errors or as extra rows.
func TestSearchTermsAreNotInterpreted(t *testing.T) {
	store := testStore(t)
	seed(t, store)
	ctx := context.Background()

	for _, term := range []string{
		"'; DROP TABLE bank; --",
		"' OR '1'='1",
		"%' --",
	} {
		got, err := store.Search(ctx, bankdata.Query{Name: term})
		if err != nil {
			t.Errorf("term %q produced an error: %v", term, err)
			continue
		}
		if len(got) != 0 {
			t.Errorf("term %q matched %d records", term, len(got))
		}
	}

	// The table must still be there afterwards.
	if _, err := store.Find(ctx, bankdata.Key{Country: "DE", BankCode: "50010517"}); err != nil {
		t.Fatalf("the fixtures did not survive: %v", err)
	}
}

func TestStats(t *testing.T) {
	store := testStore(t)
	seed(t, store)

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalRecords < 3 {
		t.Errorf("TotalRecords = %d, want at least 3", stats.TotalRecords)
	}

	var found bool
	for _, src := range stats.Sources {
		if src.Name == "Test Registry" {
			found = true
			if src.RecordCount != 3 || src.Country != "DE" {
				t.Errorf("source metadata is wrong: %+v", src)
			}
			if src.RetrievedAt.IsZero() {
				t.Error("the retrieval time was not stored")
			}
		}
	}
	if !found {
		t.Error("the seeded source is missing from Stats")
	}
}

// TestReplaceIsScopedToOneSource is the property the update pipeline depends
// on: refreshing one registry must not disturb another.
func TestReplaceIsScopedToOneSource(t *testing.T) {
	store := testStore(t)
	seed(t, store)
	ctx := context.Background()

	other := bankdata.SourceInfo{Country: "CZ", Name: "Other Registry", RetrievedAt: time.Now().UTC()}
	czech := []bankdata.Bank{{Country: "CZ", BankCode: "0100", Name: "Komercni banka",
		BIC: "KOMBCZPP", Source: "Other Registry"}}
	if err := store.Replace(ctx, other, czech); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	// Reload the first source with fewer records.
	info := bankdata.SourceInfo{Country: "DE", Name: "Test Registry", RetrievedAt: time.Now().UTC()}
	if err := store.Replace(ctx, info, fixtures()[:1]); err != nil {
		t.Fatalf("Replace: %v", err)
	}

	if _, err := store.Find(ctx, bankdata.Key{Country: "CZ", BankCode: "0100"}); err != nil {
		t.Errorf("the other source was lost: %v", err)
	}
	if _, err := store.Find(ctx, bankdata.Key{Country: "DE", BankCode: "37040044"}); !errors.Is(err, bankdata.ErrNotFound) {
		t.Error("a stale record survived the reload")
	}
	if _, err := store.Find(ctx, bankdata.Key{Country: "DE", BankCode: "50010517"}); err != nil {
		t.Errorf("the surviving record is gone: %v", err)
	}
}

// TestBackendsAgree runs the same operations against PostgreSQL and the memory
// store and compares them. The two are interchangeable by design, so a
// difference here is a bug in one of them.
func TestBackendsAgree(t *testing.T) {
	pg := testStore(t)
	seed(t, pg)

	mem := bankdata.NewMemoryStore()
	ctx := context.Background()
	info := bankdata.SourceInfo{Country: "DE", Name: "Test Registry", RetrievedAt: time.Now().UTC()}
	if err := mem.Replace(ctx, info, fixtures()); err != nil {
		t.Fatalf("seed memory store: %v", err)
	}

	queries := []bankdata.Query{
		{Country: "DE"},
		{Country: "DE", Limit: 2},
		{BIC: "COBA"},
		{Name: "bank"},
		{Country: "XX"},
	}
	for _, q := range queries {
		a, err := pg.Search(ctx, q)
		if err != nil {
			t.Fatalf("postgres search %+v: %v", q, err)
		}
		b, err := mem.Search(ctx, q)
		if err != nil {
			t.Fatalf("memory search %+v: %v", q, err)
		}
		if len(a) != len(b) {
			t.Errorf("query %+v returned %d rows from postgres and %d from memory", q, len(a), len(b))
			continue
		}
		for i := range a {
			if a[i].BankCode != b[i].BankCode || a[i].Name != b[i].Name {
				t.Errorf("query %+v differs at %d: %+v and %+v", q, i, a[i], b[i])
			}
		}
	}
}

func TestOpenRejectsBadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if _, err := postgres.Open(ctx, "not a url"); err == nil {
		t.Error("Open accepted a malformed connection string")
	}
	if _, err := postgres.Open(ctx, "postgres://nobody@127.0.0.1:1/none?sslmode=disable"); err == nil {
		t.Error("Open accepted an unreachable database")
	}
}

// TestReplaceSurvivesASourceRename is the regression test for a bug this suite
// found on its first run.
//
// Replace used to clear only the rows carrying the source name and then copy
// the new ones in. The primary key is (country, bank_code), so a record that
// had been loaded under a different source name still occupied the row and the
// copy failed on a duplicate key. Renaming a source therefore broke every
// subsequent update permanently, and it made the PostgreSQL backend behave
// differently from the in memory one, which simply overwrites.
func TestReplaceSurvivesASourceRename(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	oldName := bankdata.SourceInfo{Country: "DE", Name: "Bundesbank", RetrievedAt: time.Now().UTC()}
	banks := fixtures()
	for i := range banks {
		banks[i].Source = oldName.Name
	}
	if err := store.Replace(ctx, oldName, banks); err != nil {
		t.Fatalf("initial load: %v", err)
	}

	// The same banks arrive again under a new source name, which is what a
	// rename in the loader looks like.
	newName := bankdata.SourceInfo{Country: "DE", Name: "Deutsche Bundesbank", RetrievedAt: time.Now().UTC()}
	renamed := fixtures()
	for i := range renamed {
		renamed[i].Source = newName.Name
	}
	if err := store.Replace(ctx, newName, renamed); err != nil {
		t.Fatalf("reload under a new source name: %v", err)
	}

	got, err := store.Find(ctx, bankdata.Key{Country: "DE", BankCode: "50010517"})
	if err != nil {
		t.Fatalf("Find after the rename: %v", err)
	}
	if got.Source != newName.Name {
		t.Errorf("Source = %q, want %q", got.Source, newName.Name)
	}

	// No duplicates may be left behind.
	all, err := store.Search(ctx, bankdata.Query{Country: "DE"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(all) != len(fixtures()) {
		t.Errorf("holding %d records after the rename, want %d", len(all), len(fixtures()))
	}
}
