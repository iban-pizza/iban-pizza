package sources

import "testing"

// nbsPage is an invented page in the layout of the published one: a second
// table that is not the directory, header rows spanning columns, a header row
// that is never closed, and the date split over tags and non-breaking spaces.
const nbsPage = `<html><body>
<table><tr><td>Year</td><td>2026</td></tr></table>
<h1>Directory of identification codes</h1>
<p>
    Version:
    <strong>301</strong>
</p>
<p>
    Effective from:
    <strong>5.&nbsp;3.&nbsp;2026</strong>
</p>
<table>
  <thead>
    <tr><th colspan="8">SLOVAK REPUBLIC</th></tr>
    <tr><th></th><th colspan="3">SWIFT</th><th colspan="2">CODE</th><th></th><th></th></tr>
    <tr><th></th><th>BANK</th><th colspan="2">BIC</th><th>Numeric</th><th>Alphabetic</th><th>Active party</th><th>NAME Domestic payment service provider</th></tr>
  </thead>
  <tbody>
    <tr><td></td><td>EXAM</td><td>SK</td><td>BX</td><td>0123</td><td>EXAM</td><td>C</td><td>Example banka, a.s.</td></tr>
    <tr><td>+</td><td>TEST</td><td>SK</td><td>2X</td><td>4567</td><td>TSTB</td><td>C</td><td>Skúšobná banka, a.s. &ndash; kód pre dobeh platieb</td></tr>
    <tr><td></td><td></td><td></td><td></td><td>8999</td><td>NOBIC</td><td>&Oslash;</td><td>Provider without BIC, a.s.</td></tr>
    <tr><td></td><td>EXAM</td><td>SK</td><td>BX</td><td>0123</td><td>EXAM</td><td>C</td><td>Repeated code, a.s.</td></tr>
    <tr><th></th><th>BANK</th><th colspan="2">BIC</th><th>Numeric</th><th>Alphabetic</th><th>Active party</th><th>NAME Foreign payment service provider</th>
    <tr>
    <tr><td></td><td>EXCZ</td><td>CZ</td><td>PP</td><td>0777</td><td>EXCZ</td><td>C</td><td>Example banka CZ, a.s.</td></tr>
  </tbody>
</table>
</body></html>`

func TestNBSParse(t *testing.T) {
	banks, err := NBS{}.Parse([]byte(nbsPage))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 4 {
		t.Fatalf("got %d records %v, want 4", len(banks), got)
	}
	// The BIC is the three SWIFT cells joined, and the leading zero stays.
	if b := got["0123"]; b.BIC != "EXAMSKBX" || b.Name != "Example banka, a.s." || b.Country != "SK" {
		t.Errorf("0123 = %+v", b)
	}
	// A changed row carries "+" in front; a run-off code is still a code.
	if b := got["4567"]; b.BIC != "TESTSK2X" || b.Name != "Skúšobná banka, a.s. – kód pre dobeh platieb" {
		t.Errorf("4567 = %+v", b)
	}
	if b := got["8999"]; b.BIC != "" || b.Name != "Provider without BIC, a.s." {
		t.Errorf("8999 = %+v", b)
	}
	// A Czech bank listed for Slovak payments is a Slovak bank code with a
	// Czech BIC.
	if b := got["0777"]; b.BIC != "EXCZCZPP" || b.Country != "SK" {
		t.Errorf("0777 = %+v", b)
	}

	if d := (NBS{}).PublishedAt([]byte(nbsPage)); d.Format("2006-01-02") != "2026-03-05" {
		t.Errorf("PublishedAt = %v, want 2026-03-05", d)
	}
	if d := (NBS{}).PublishedAt([]byte("<p>no date here</p>")); !d.IsZero() {
		t.Errorf("PublishedAt without a date = %v, want zero", d)
	}
}
