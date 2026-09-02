package sources

import (
	"strings"
	"testing"
)

func TestParseGenericCommaSeparated(t *testing.T) {
	in := "bankCode,name,bic,zip,city\n" +
		"12345,Example Bank,EXAMGB2LXXX,SW1A 1AA,London\n" +
		"67890,Another Bank,,,Leeds\n"

	banks, err := ParseGeneric("GB", "", []byte(in))
	if err != nil {
		t.Fatalf("ParseGeneric returned %v", err)
	}
	if len(banks) != 2 {
		t.Fatalf("parsed %d records, want 2", len(banks))
	}
	b := banks[0]
	if b.Country != "GB" || b.BankCode != "12345" || b.Name != "Example Bank" ||
		b.BIC != "EXAMGB2LXXX" || b.Zip != "SW1A 1AA" || b.City != "London" {
		t.Errorf("record = %+v", b)
	}
	if b.Source != GenericName {
		t.Errorf("Source = %q, want %q", b.Source, GenericName)
	}
}

// TestParseGenericHeaderVariants covers the header matching. A customer's
// export will not use our exact spelling, and asking them to rename columns
// is friction the parser can absorb.
func TestParseGenericHeaderVariants(t *testing.T) {
	in := "Bank Code;Name;SWIFT;PLZ;Ort\n" +
		"10000000;Bundesbank;MARKDEF1100;10591;Berlin\n"

	banks, err := ParseGeneric("de", "My Source", []byte(in))
	if err != nil {
		t.Fatalf("ParseGeneric returned %v", err)
	}
	b := banks[0]
	if b.Country != "DE" || b.BIC != "MARKDEF1100" || b.Zip != "10591" || b.City != "Berlin" {
		t.Errorf("record = %+v", b)
	}
	if b.Source != "My Source" {
		t.Errorf("Source = %q", b.Source)
	}
}

func TestParseGenericCountryColumnOverrides(t *testing.T) {
	in := "country,bankCode,name\nAT,20111,Erste Bank\nDE,37040044,Commerzbank\n"

	banks, err := ParseGeneric("XX", "", []byte(in))
	if err != nil {
		t.Fatalf("ParseGeneric returned %v", err)
	}
	got := map[string]string{}
	for _, b := range banks {
		got[b.BankCode] = b.Country
	}
	if got["20111"] != "AT" || got["37040044"] != "DE" {
		t.Errorf("countries = %v", got)
	}
}

func TestParseGenericRejects(t *testing.T) {
	cases := map[string]string{
		"no header":        "12345,Example\n",
		"missing name":     "bankCode,bic\n12345,EXAMGB2L\n",
		"empty":            "",
		"only header":      "bankCode,name\n",
		"blank rows":       "bankCode,name\n,\n,\n",
		"bad country code": "bankCode,name\n12345,Example\n",
	}
	for label, in := range cases {
		country := "GB"
		if label == "bad country code" {
			country = "GBR"
		}
		if _, err := ParseGeneric(country, "", []byte(in)); err == nil {
			t.Errorf("%s: ParseGeneric accepted %q", label, in)
		}
	}
}

func TestParseGenericDeduplicates(t *testing.T) {
	in := "bankCode,name\n1,First\n1,Duplicate\n2,Second\n"
	banks, err := ParseGeneric("GB", "", []byte(in))
	if err != nil {
		t.Fatalf("ParseGeneric returned %v", err)
	}
	if len(banks) != 2 || banks[0].Name != "First" {
		t.Errorf("records = %+v", banks)
	}
}

func TestDetectSeparator(t *testing.T) {
	if got := detectSeparator("a;b;c\n1;2;3"); got != ';' {
		t.Errorf("semicolon file detected as %q", got)
	}
	if got := detectSeparator("a,b,c\n1,2,3"); got != ',' {
		t.Errorf("comma file detected as %q", got)
	}
	// A city like "Frankfurt, Main" in the body must not flip the decision;
	// only the header counts.
	if got := detectSeparator("a;b\n1;Frankfurt, am, Main"); got != ';' {
		t.Errorf("body commas changed the separator to %q", got)
	}
}

// TestParseFilePrefersBuiltInParser checks that a country with a registered
// source uses it, and that one without falls back to the generic layout.
func TestParseFilePrefersBuiltInParser(t *testing.T) {
	// Germany has a parser; feed it the real fixture.
	banks, name, err := ParseFile("de", "", fixture(t, "bundesbank.txt"))
	if err != nil {
		t.Fatalf("ParseFile(DE) returned %v", err)
	}
	if name != (Bundesbank{}).Name() {
		t.Errorf("source name = %q, want the Bundesbank parser", name)
	}
	if _, ok := byCode(banks, "50010517"); !ok {
		t.Error("the built in parser was not used")
	}

	// A source override must land on every record.
	banks, name, err = ParseFile("DE", "Renamed", fixture(t, "bundesbank.txt"))
	if err != nil {
		t.Fatalf("ParseFile with override returned %v", err)
	}
	// The returned name and the records must agree, since the caller records
	// the one and stores the other; a mismatch strands the records under a
	// name no later Replace will look for.
	if name != "Renamed" {
		t.Errorf("returned source name = %q, want the override", name)
	}
	for _, b := range banks {
		if b.Source != "Renamed" {
			t.Fatalf("Source = %q, override was not applied", b.Source)
		}
	}

	// No parser for GB, so the generic layout applies.
	banks, name, err = ParseFile("GB", "", []byte("bankCode,name\n123456,Example\n"))
	if err != nil {
		t.Fatalf("ParseFile(GB) returned %v", err)
	}
	if name != GenericName || len(banks) != 1 {
		t.Errorf("fallback: name=%q records=%d", name, len(banks))
	}

	// The built in parser must still reject junk rather than silently
	// producing nothing.
	if _, _, err := ParseFile("DE", "", []byte(strings.Repeat("x", 10))); err == nil {
		t.Error("ParseFile(DE) accepted junk")
	}
}
