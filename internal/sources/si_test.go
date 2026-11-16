package sources

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestBSIParse(t *testing.T) {
	// Invented rows in the layout of the published file: CRLF line ends and
	// Windows-1250, where the carons are not the bytes Latin-1 would expect.
	text := strings.Join([]string{
		"NATIONAL ID;NAME;ADDRESS;ZIP CODE;CITY;BIC11",
		"09001;EXAMPLE BANKA D.D.;PRIMEROVA ULICA 1;1000;LJUBLJANA;EXAMSI2XXXX",
		"09002;EXAMPLE BANKA D.D.;PRIMEROVA ULICA 1;1000;LJUBLJANA;EXAMSI2XXXX",
		"98765;HRANILNICA ČRNI ŽELEZNIK D.O.O.;ŠOLSKA CESTA 2;4000;KRANJ;exam si 22abc",
		"98765;HRANILNICA PONOVITEV D.O.O.;DRUGA CESTA 3;2000;MARIBOR;TESTSI22XXX",
		"123;NOT AN IDENTIFIER;NOWHERE 4;3000;CELJE;TESTSI22XXX",
		"",
	}, "\r\n")
	data, err := charmap.Windows1250.NewEncoder().Bytes([]byte(text))
	if err != nil {
		t.Fatal(err)
	}

	banks, err := BSI{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 3 {
		t.Fatalf("got %d records %v, want 3 (the repeated and the short identifier must be dropped)", len(banks), got)
	}
	// Two branches of one bank are two codes: the IBAN carries all five digits.
	if b := got["09001"]; b.Name != "EXAMPLE BANKA D.D." || b.BIC != "EXAMSI2XXXX" || b.Zip != "1000" || b.City != "LJUBLJANA" || b.Country != "SI" {
		t.Errorf("09001 = %+v", b)
	}
	if _, ok := got["09002"]; !ok {
		t.Errorf("09002 missing, got %v", got)
	}
	// The name survives the code page, and the first of two rows with the
	// same identifier wins.
	if b := got["98765"]; b.Name != "HRANILNICA ČRNI ŽELEZNIK D.O.O." || b.BIC != "EXAMSI22ABC" || b.City != "KRANJ" {
		t.Errorf("98765 = %+v", b)
	}
}

func TestBSIParseUTF8(t *testing.T) {
	// A file that has moved to UTF-8 must not be decoded a second time.
	data := []byte("NATIONAL ID;NAME;ADDRESS;ZIP CODE;CITY;BIC11\r\n09001;BANKA ŠČŽ D.D.;ULICA 1;1000;LJUBLJANA;EXAMSI2XXXX\r\n")
	banks, err := BSI{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	if len(banks) != 1 || banks[0].Name != "BANKA ŠČŽ D.D." {
		t.Errorf("got %+v", banks)
	}
}
