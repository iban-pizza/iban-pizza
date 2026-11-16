package sources

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestHBAParse(t *testing.T) {
	// Invented rows in the layout of the published file: a title line above
	// the header, codes in apostrophes, a trailing semicolon on every row,
	// Windows-1253.
	text := strings.Join([]string{
		"LIST OF EXAMPLE INSTITUTIONS; INVENTED FOR THIS TEST",
		"Code Number;Name of Credit Institution;Address;Telephone;Fax;URL",
		"'007';EXAMPLE BANK S.A.;1 EXAMPLE STR., 100 00 ATHENS;(+30) 2100000000;(+30) 2100000001;http://www.example.com;",
		"'912';ΤΡΑΠΕΖΑ ΠΑΡΑΔΕΙΓΜΑ Α.Ε.;2 SAMPLE AV.; 2nd FLOOR, 200 00 PATRAS;(+30) 2610000000;;http://www.example.org;",
		"'007';REPEATED BANK S.A.;3 OTHER STR.;;;;",
		"'12';SHORT CODE BANK;4 OTHER STR.;;;;",
		"",
	}, "\r\n")
	data, err := charmap.Windows1253.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}

	banks, err := HBA{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 2 {
		t.Fatalf("got %d records %v, want 2", len(banks), got)
	}
	// The apostrophes go, the leading zeros stay. The source has no BIC.
	if b := got["007"]; b.Name != "EXAMPLE BANK S.A." || b.BIC != "" || b.LEI != "" || b.Country != "GR" {
		t.Errorf("007 = %+v", b)
	}
	// A semicolon inside the address adds a field, which must not disturb
	// code and name; the Greek name survives the code page.
	if b := got["912"]; b.Name != "ΤΡΑΠΕΖΑ ΠΑΡΑΔΕΙΓΜΑ Α.Ε." {
		t.Errorf("912 = %+v", b)
	}
}
