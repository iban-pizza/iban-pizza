package sources

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

// buildXLSX writes a minimal workbook with one sheet, so tests can exercise
// the spreadsheet parsers on invented rows without a binary fixture and
// without checking a publisher's file into the repository.
//
// Cells are written as shared strings, except those prefixed with "#", which
// are written as numbers the way a spreadsheet stores a numeric code.
func buildXLSX(t *testing.T, sheet string, rows [][]string) []byte {
	t.Helper()

	var shared []string
	index := map[string]int{}
	var sheetXML strings.Builder
	sheetXML.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&sheetXML, `<row r="%d">`, r+1)
		for c, cell := range row {
			if cell == "" {
				continue
			}
			ref := fmt.Sprintf("%c%d", 'A'+c, r+1)
			if strings.HasPrefix(cell, "#") {
				fmt.Fprintf(&sheetXML, `<c r="%s"><v>%s</v></c>`, ref, cell[1:])
				continue
			}
			n, ok := index[cell]
			if !ok {
				n = len(shared)
				index[cell] = n
				shared = append(shared, cell)
			}
			fmt.Fprintf(&sheetXML, `<c r="%s" t="s"><v>%d</v></c>`, ref, n)
		}
		sheetXML.WriteString(`</row>`)
	}
	sheetXML.WriteString(`</sheetData></worksheet>`)

	var sst strings.Builder
	sst.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range shared {
		var esc bytes.Buffer
		if err := xml.EscapeText(&esc, []byte(s)); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sst, `<si><t xml:space="preserve">%s</t></si>`, esc.String())
	}
	sst.WriteString(`</sst>`)

	var sheetName bytes.Buffer
	if err := xml.EscapeText(&sheetName, []byte(sheet)); err != nil {
		t.Fatal(err)
	}
	sheet = sheetName.String()

	files := map[string]string{
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + sheet + `" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheetXML.String(),
		"xl/sharedStrings.xml":       sst.String(),
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadXLSX(t *testing.T) {
	data := buildXLSX(t, "List", [][]string{
		{"Code", "Name", "", "BIC"},
		{"#42", "Example & Sons", "", "EXAMBEBB"},
	})
	rows, err := readXLSX(data, "list")
	if err != nil {
		t.Fatalf("readXLSX returned %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	// The empty third column must keep the BIC in the fourth.
	if got := rows[1]; len(got) != 4 || got[0] != "42" || got[1] != "Example & Sons" || got[3] != "EXAMBEBB" {
		t.Errorf("row = %q", got)
	}
	if _, err := readXLSX(data, "missing"); err == nil {
		t.Error("readXLSX accepted an unknown sheet name")
	}
	if _, err := readXLSX([]byte("not a zip"), ""); err == nil {
		t.Error("readXLSX accepted a file that is not an archive")
	}
}

func TestHTMLRows(t *testing.T) {
	page := `<html><body><table>
	  <tr><th>Code</th><th>Name</th></tr>
	  <tr><td> 0200 </td><td>Example&nbsp;Bank,&amp; Co<br>a.s.</td></tr>
	  <tr><td><table><tr><td>inner</td><td>row</td></tr></table></td></tr>
	</table><!-- <tr><td>hidden</td></tr> --></body></html>`
	rows := htmlRows(page)
	want := [][]string{{"Code", "Name"}, {"0200", "Example Bank,& Co a.s."}, {"inner", "row"}}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows %q, want %d", len(rows), rows, len(want))
	}
	for i := range want {
		if strings.Join(rows[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}

func TestBICMappingLookup(t *testing.T) {
	m, err := ParseBICMapping([]byte("LEI,BIC\n" +
		"AAAAAAAAAAAAAAAAAA01,EXAMFRPPXXX\n" +
		"AAAAAAAAAAAAAAAAAA01,EXAMFRPP123\n" +
		"AAAAAAAAAAAAAAAAAA01,EXAMDEFFXXX\n" +
		"AAAAAAAAAAAAAAAAAA02,ONEAFRPPXXX\n" +
		"AAAAAAAAAAAAAAAAAA02,TWOBFRPPXXX\n"))
	if err != nil {
		t.Fatalf("ParseBICMapping returned %v", err)
	}
	// Several BICs of one institution in the country: the head office form.
	if got := m.Lookup("aaaaaaaaaaaaaaaaaa01", "FR"); got != "EXAMFRPPXXX" {
		t.Errorf("FR lookup = %q, want EXAMFRPPXXX", got)
	}
	// The same LEI in another country resolves to that country's BIC.
	if got := m.Lookup("AAAAAAAAAAAAAAAAAA01", "DE"); got != "EXAMDEFFXXX" {
		t.Errorf("DE lookup = %q, want EXAMDEFFXXX", got)
	}
	// Two different institution codes in one country are ambiguous, and an
	// ambiguous BIC must not be guessed.
	if got := m.Lookup("AAAAAAAAAAAAAAAAAA02", "FR"); got != "" {
		t.Errorf("ambiguous lookup = %q, want empty", got)
	}
	if got := m.Lookup("AAAAAAAAAAAAAAAAAA01", "IT"); got != "" {
		t.Errorf("lookup without a local BIC = %q, want empty", got)
	}
}
