package sources

import "testing"

func TestHNBParse(t *testing.T) {
	// Invented rows in the layout of the published sheet: a title, the header
	// on the fourth row, and everything shifted one column to the right.
	data := buildXLSX(t, "Sheet1", [][]string{
		{},
		{"", "Vodeći brojevi pružatelja platnih usluga"},
		{},
		{"", "Naziv pružatelja platnih usluga", "Vodeći broj ", "SWIFT adresa\n(BIC)"},
		{"", "PRIMJER BANKA d.d. Zagreb", "#2999001", "EXAM HR 22"},
		{"", "TEST BANKA d.d. Split", "#2998004", "TEST HR 2X "},
		{"", "PRIMJER PLAĆANJA d.o.o. Zagreb", "#4999007"},
		{"", "KRATKI BROJ d.d.", "#99005", "KRAT HR 22"},
		{},
	})

	banks, err := HNB{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 4 || len(got) != 4 {
		t.Fatalf("got %d records %v, want 4", len(banks), banks)
	}
	// The numeric cell comes out as text and the grouped BIC is joined.
	if b := got["2999001"]; b.BIC != "EXAMHR22" || b.Name != "PRIMJER BANKA d.d. Zagreb" || b.Country != "HR" || b.Source != "Croatian National Bank" {
		t.Errorf("2999001 = %+v", b)
	}
	if b := got["2998004"]; b.BIC != "TESTHR2X" {
		t.Errorf("2998004 = %+v", b)
	}
	// A provider without a BIC is kept.
	if b := got["4999007"]; b.BIC != "" || b.Name != "PRIMJER PLAĆANJA d.o.o. Zagreb" {
		t.Errorf("4999007 = %+v", b)
	}
	// A shorter number is padded to the seven digits of the IBAN field.
	if b := got["0099005"]; b.BIC != "KRATHR22" {
		t.Errorf("0099005 = %+v", b)
	}
}
