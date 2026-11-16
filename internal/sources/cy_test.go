package sources

import "testing"

func TestCBCParse(t *testing.T) {
	// Invented rows in the layout of the published sheet.
	data := buildXLSX(t, "FIs and EMIs BICs", [][]string{
		{"#", "Credit Institution", "Bank identifiers used in IBAN", "Full BIC \n(8 Characters)"},
		{},
		{"#1", "EXAMPLE BANK LTD ", "001", "EXAMCY2N"},
		{"#2", "TEST BANK PUBLIC COMPANY", "#12", "TESTCY2N "},
		{},
		{"#", "Electronic Money Institution", "Bank identifiers used in IBAN", "Full BIC\n(8 Characters)"},
		{},
		{"#1", "Example E-Money Ltd", "#901", "EMON CY 22"},
		{"", "", "", "EMONCY2A"},
		{"#2", "Sample Pay Ltd", "902", "SAMPCY2L"},
		{},
		{"Updated on September 2026 (The table will be updated whenever a change occurs)"},
	})

	banks, err := CBC{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 4 || len(got) != 4 {
		t.Fatalf("got %d records %v, want 4 (headers, blanks, footer and continuation rows must be dropped)", len(banks), banks)
	}
	if b := got["001"]; b.BIC != "EXAMCY2N" || b.Name != "EXAMPLE BANK LTD" || b.Country != "CY" || b.Source != "Central Bank of Cyprus" {
		t.Errorf("001 = %+v", b)
	}
	// A numeric cell is padded back to three digits.
	if b := got["012"]; b.BIC != "TESTCY2N" || b.Name != "TEST BANK PUBLIC COMPANY" {
		t.Errorf("012 = %+v", b)
	}
	// The row below 901 carries a second BIC only; the first one stays.
	if b := got["901"]; b.BIC != "EMONCY22" || b.Name != "Example E-Money Ltd" {
		t.Errorf("901 = %+v", b)
	}
	if b := got["902"]; b.BIC != "SAMPCY2L" {
		t.Errorf("902 = %+v", b)
	}
}

func TestCBCLink(t *testing.T) {
	// Shaped like the page: the legacy register comes first and must not match.
	page := `<p>The register is <a href="/images/media/redirectfile/FMIP/IBAN/CY-IBAN-Register-Example-August_2026.xls" target="_blank">here</a>
	and the list is <a href="/images/media/redirectfile/FMIP/IBAN/CIs_and_EMIs_BICs_updated_September_2026.xlsx" target="_blank">Credit_Institutions</a>.</p>`
	m := cbcLink.FindStringSubmatch(page)
	if m == nil || m[1] != "/images/media/redirectfile/FMIP/IBAN/CIs_and_EMIs_BICs_updated_September_2026.xlsx" {
		t.Errorf("link = %q", m)
	}
}
