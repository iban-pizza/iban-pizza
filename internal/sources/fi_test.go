package sources

import "testing"

func TestFinanssialaParse(t *testing.T) {
	// Invented rows in the layout of the published sheet.
	data := buildXLSX(t, "Taul1", [][]string{
		{"7.9.2026"},
		{"National ID", "BIC Code", "Financial Institution Name"},
		{"100", "EXAMFIHH", "EXAMPLE BANK PLC"},
		{"101", "EXAMFIHH", "EXAMPLE BANK PLC"},
		{"#31", "TESTFI22 ", "TEST BANK FINLAND BRANCH"},
		{"660", "DUPLFI22", "DUPLICATE BANK PLC"},
		{"660", "OTHRFIHH", "DUPLICATE BANK PLC"},
		{},
	})

	banks, err := Finanssiala{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 4 || len(got) != 4 {
		t.Fatalf("got %d records %v, want 4 (a repeated ID must be kept once)", len(banks), banks)
	}
	if b := got["100"]; b.BIC != "EXAMFIHH" || b.Name != "EXAMPLE BANK PLC" || b.Country != "FI" || b.Source != "Finance Finland" {
		t.Errorf("100 = %+v", b)
	}
	if b := got["101"]; b.BIC != "EXAMFIHH" {
		t.Errorf("101 = %+v", b)
	}
	// A numeric cell is padded back to three digits.
	if b := got["031"]; b.BIC != "TESTFI22" {
		t.Errorf("031 = %+v", b)
	}
	// The first of two rows for one ID stands.
	if b := got["660"]; b.BIC != "DUPLFI22" {
		t.Errorf("660 = %+v", b)
	}

	if d := (Finanssiala{}).PublishedAt(data); d.Format("2006-01-02") != "2026-09-07" {
		t.Errorf("PublishedAt = %v, want 2026-09-07", d)
	}
	// A sheet without the date in its first cell reports none.
	undated := buildXLSX(t, "Taul1", [][]string{
		{"National ID", "BIC Code", "Financial Institution Name"},
		{"100", "EXAMFIHH", "EXAMPLE BANK PLC"},
	})
	if d := (Finanssiala{}).PublishedAt(undated); !d.IsZero() {
		t.Errorf("PublishedAt without a date = %v, want zero", d)
	}
}

func TestFinanssialaLink(t *testing.T) {
	// Shaped like the page: the PDF edition is linked first and must not match.
	page := `<ul><li><a href="https://www.finanssiala.fi/wp-content/uploads/2026/04/finnish-monetary-institution-codes-01012026.pdf">PDF</a></li>
	<li><a href="https://www.finanssiala.fi/wp-content/uploads/2026/04/finnish_monetary_institution_codes_and_bics_in_excel_format-01012026.xlsx">Excel</a></li></ul>`
	m := finanssialaLink.FindStringSubmatch(page)
	if m == nil || m[1] != "https://www.finanssiala.fi/wp-content/uploads/2026/04/finnish_monetary_institution_codes_and_bics_in_excel_format-01012026.xlsx" {
		t.Errorf("link = %q", m)
	}
}
