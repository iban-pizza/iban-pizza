package sources

import "testing"

func TestCBMParse(t *testing.T) {
	// Invented rows in the layout of the published sheet.
	data := buildXLSX(t, "Sheet1", [][]string{
		{"BIC Found in IBAN (4 Characters)", "Full BIC (8 Characters)", "Financial Institution Name"},
		{},
		{"EXAM", "EXAMMTMT", "Example Bank Ltd"},
		{"test", "TESTMTMB ", "Test Payments ltd."},
		{"DUPL", "DUPLMTMTXXX", "Duplicate Finance ltd."},
		{"DUPL", "DUPLMTMTMAL", "Duplicate Finance ltd."},
		{"", "NONEMTMT", "Row without a code"},
	})

	banks, err := CBM{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 3 || len(got) != 3 {
		t.Fatalf("got %d records %v, want 3", len(banks), banks)
	}
	if b := got["EXAM"]; b.BIC != "EXAMMTMT" || b.Name != "Example Bank Ltd" || b.Country != "MT" || b.Source != "Central Bank of Malta" {
		t.Errorf("EXAM = %+v", b)
	}
	// Trailing whitespace in the BIC cell is dropped and the code is upper case.
	if b := got["TEST"]; b.BIC != "TESTMTMB" {
		t.Errorf("TEST = %+v", b)
	}
	// Two rows for the same four letters: the first BIC stands.
	if b := got["DUPL"]; b.BIC != "DUPLMTMTXXX" {
		t.Errorf("DUPL = %+v", b)
	}
}
