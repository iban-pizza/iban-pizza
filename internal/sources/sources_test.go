package sources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func byCode(banks []bankdata.Bank, code string) (bankdata.Bank, bool) {
	for _, b := range banks {
		if b.BankCode == code {
			return b, true
		}
	}
	return bankdata.Bank{}, false
}

func TestBundesbankParse(t *testing.T) {
	banks, err := Bundesbank{}.Parse(fixture(t, "bundesbank.txt"))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}

	// The fixture has six rows, one of which is a branch office. Branches
	// share their head office bank code, so keeping them would overwrite the
	// head office record.
	if len(banks) != 5 {
		t.Fatalf("parsed %d records, want 5 (one branch must be skipped)", len(banks))
	}
	if _, ok := byCode(banks, "10020890"); ok {
		t.Error("branch office record was not skipped")
	}

	ing, ok := byCode(banks, "50010517")
	if !ok {
		t.Fatal("ING-DiBa record is missing")
	}
	for _, tc := range []struct{ field, got, want string }{
		{"Name", ing.Name, "ING-DiBa"},
		{"BIC", ing.BIC, "INGDDEFFXXX"},
		{"CheckAlgo", ing.CheckAlgo, "C1"},
		{"Zip", ing.Zip, "60628"},
		{"City", ing.City, "Frankfurt am Main"},
		{"Country", ing.Country, "DE"},
	} {
		if tc.got != tc.want {
			t.Errorf("ING %s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

// TestBundesbankUmlautColumns is the regression guard for the encoding trap.
//
// The published file is ISO-8859-1 and fixed width in characters. If it is
// decoded wrongly, or sliced by byte instead of by rune, every column after an
// umlaut shifts and BICs land on the wrong institution. These two records both
// carry an umlaut in the name, so their trailing columns only come out right
// when both steps are correct.
func TestBundesbankUmlautColumns(t *testing.T) {
	banks, err := Bundesbank{}.Parse(fixture(t, "bundesbank.txt"))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}

	cases := []struct{ code, name, bic, city string }{
		{"10030500", "M.M. Warburg & Co (vormals Bankhaus Löbbecke)", "LOEBDEBBXXX", "Hamburg"},
		{"10014001", "Swan Zweigniederlassung Deutschland (Geschäftsfeld)", "SWNBDEBBXXX", "Berlin"},
	}
	for _, tc := range cases {
		b, ok := byCode(banks, tc.code)
		if !ok {
			t.Errorf("record %s is missing", tc.code)
			continue
		}
		if b.Name != tc.name {
			t.Errorf("%s Name = %q, want %q", tc.code, b.Name, tc.name)
		}
		if b.BIC != tc.bic {
			t.Errorf("%s BIC = %q, want %q (columns shifted after the umlaut)", tc.code, b.BIC, tc.bic)
		}
		if b.City != tc.city {
			t.Errorf("%s City = %q, want %q", tc.code, b.City, tc.city)
		}
	}
}

func TestBundesbankRejectsShortLines(t *testing.T) {
	if _, err := (Bundesbank{}).Parse([]byte("far too short\r\n")); err == nil {
		t.Error("Parse accepted a truncated record")
	}
	if _, err := (Bundesbank{}).Parse(nil); err == nil {
		t.Error("Parse accepted an empty file")
	}
}

func TestOeNBParse(t *testing.T) {
	banks, err := OeNB{}.Parse(fixture(t, "oenb.csv"))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	if len(banks) != 3 {
		t.Fatalf("parsed %d records, want 3", len(banks))
	}

	// The file carries five lines of legal notice and a query date above the
	// header, so the parser has to find the header by content.
	b, ok := byCode(banks, "11000")
	if !ok {
		t.Fatal("UniCredit Bank Austria record is missing")
	}
	for _, tc := range []struct{ field, got, want string }{
		{"Name", b.Name, "UniCredit Bank Austria AG"},
		{"BIC", b.BIC, "BKAUATWWXXX"},
		{"Zip", b.Zip, "1020"},
		{"City", b.City, "Wien"},
		{"Country", b.Country, "AT"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}

	// The Austrian file is Latin-1 too; the national bank's own name is the
	// simplest check that decoding happened.
	if nb, ok := byCode(banks, "100"); !ok || nb.BIC != "NABAATWWXXX" {
		t.Errorf("OeNB record = %+v", nb)
	}
}

func TestOeNBRejectsMissingHeader(t *testing.T) {
	if _, err := (OeNB{}).Parse([]byte("just;some;columns\n1;2;3\n")); err == nil {
		t.Error("Parse accepted a file without the expected header")
	}
}

func TestCNBParse(t *testing.T) {
	banks, err := CNB{}.Parse(fixture(t, "cnb.csv"))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	if len(banks) != 3 {
		t.Fatalf("parsed %d records, want 3", len(banks))
	}

	b, ok := byCode(banks, "0100")
	if !ok {
		t.Fatal("Komercni banka record is missing")
	}
	if b.BIC != "KOMBCZPP" {
		t.Errorf("BIC = %q, want KOMBCZPP", b.BIC)
	}
	if b.Country != "CZ" {
		t.Errorf("Country = %q, want CZ", b.Country)
	}
	// The Czech file is UTF-8, unlike the German and Austrian ones. Decoding
	// must not mangle it.
	if b.Name != "Komerční banka, a.s." {
		t.Errorf("Name = %q, want the UTF-8 original", b.Name)
	}
}

func TestRegistryIsPopulated(t *testing.T) {
	got := Countries()
	want := map[string]bool{"DE": true, "AT": true, "CZ": true}
	for _, c := range got {
		delete(want, c)
	}
	if len(want) > 0 {
		t.Errorf("missing sources for %v, registry has %v", want, got)
	}

	for _, s := range All() {
		if s.Country() == "" || s.Name() == "" {
			t.Errorf("source %T has an empty country or name", s)
		}
	}
}
