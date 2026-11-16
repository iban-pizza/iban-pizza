package sources

import (
	"testing"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

// codeIndex indexes parsed records by bank code for assertions.
func codeIndex(banks []bankdata.Bank) map[string]bankdata.Bank {
	out := make(map[string]bankdata.Bank, len(banks))
	for _, b := range banks {
		out[b.BankCode] = b
	}
	return out
}

func TestNBBParse(t *testing.T) {
	// Invented rows in the layout of the published sheet.
	data := buildXLSX(t, "Q_FULL_LIST_XLS_REPORT", [][]string{
		{"Version 01/09/2026"},
		{"T_Identification_Number", "Biccode", "T_Institutions_Dutch", "T_Institutions_French", "T_Institutions_German", "T_Institutions_English"},
		{"001", "EXAM BE BB", "Voorbeeldbank", "Banque Exemple", "", ""},
		{"#7", "TEST BE 22 123", "", "Banque Test", "", ""},
		{"010", "VRIJ", "VRIJ", "LIBRE", "", ""},
		{"011", "N/A", "Instelling zonder BIC", "", "", "Institution without BIC"},
		{"012", "-", "Onbeschikbaar", "Indisponible", "", ""},
	})

	banks, err := NBB{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(got) != 3 {
		t.Fatalf("got %d records %v, want 3 (the free and the unavailable code must be dropped)", len(got), got)
	}
	if b := got["001"]; b.BIC != "EXAMBEBB" || b.Name != "Voorbeeldbank" || b.Country != "BE" {
		t.Errorf("001 = %+v", b)
	}
	// A numeric cell is padded back to three digits, and a name given only in
	// French is still found.
	if b := got["007"]; b.BIC != "TESTBE22123" || b.Name != "Banque Test" {
		t.Errorf("007 = %+v", b)
	}
	if b := got["011"]; b.BIC != "" || b.Name != "Institution without BIC" {
		t.Errorf("011 = %+v", b)
	}

	if d := (NBB{}).PublishedAt(data); d.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("PublishedAt = %v, want 2026-09-01", d)
	}
}
