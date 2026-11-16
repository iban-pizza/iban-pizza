package sources

import "testing"

func TestABBLParse(t *testing.T) {
	// Invented rows in the layout of the published sheet.
	data := buildXLSX(t, "Organizations", [][]string{
		{"ABBL\n\nList of IBAN and BIC codes of Luxembourg credit institutions\n"},
		{"Credit institution", "IBAN Code ", "BICCode"},
		{"Example Bank S.A.", "001", "EXAMLULL"},
		{"Test Bank, Luxembourg Branch", "#45", "testlull xxx"},
		{"Sample Payments S.A.", "#816", "SAMPLU22"},
		{"Row without a code", "", "NONELULL"},
	})

	banks, err := ABBL{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 3 || len(got) != 3 {
		t.Fatalf("got %d records %v, want 3", len(banks), banks)
	}
	if b := got["001"]; b.BIC != "EXAMLULL" || b.Name != "Example Bank S.A." || b.Country != "LU" || b.Source != "ABBL" {
		t.Errorf("001 = %+v", b)
	}
	// A numeric cell is padded back to three digits; the BIC is tidied.
	if b := got["045"]; b.BIC != "TESTLULLXXX" || b.Name != "Test Bank, Luxembourg Branch" {
		t.Errorf("045 = %+v", b)
	}
	if b := got["816"]; b.BIC != "SAMPLU22" {
		t.Errorf("816 = %+v", b)
	}
}

func TestABBLLink(t *testing.T) {
	page := `<div class="download"><a class="btn" href="https://office-membernet.abbl.lu/newDocRequest/AB12CD34EF56GH78/ABBL_ExampleRegister123.xlsx" target="_blank">Download</a>
	<a href="https://www.abbl.lu/other/file.xlsx">Other</a></div>`
	m := abblLink.FindStringSubmatch(page)
	if m == nil || m[1] != "https://office-membernet.abbl.lu/newDocRequest/AB12CD34EF56GH78/ABBL_ExampleRegister123.xlsx" {
		t.Errorf("link = %q", m)
	}
}
