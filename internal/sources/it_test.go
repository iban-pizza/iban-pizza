package sources

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestBancaDItaliaParse(t *testing.T) {
	// Invented rows in the layout of the published file: a BOM, a header that
	// starts with a space, and a single space where a cell is empty.
	rows := []string{
		" ID_INT;COD_MECC;DES_TIPO_ALBO;DEN_INT;DES_COM_ITA_SL;CAP_SL;COD_LEI;DATA_I_VAL;DATA_F_VAL",
		// The same bank under its former name: history, dropped.
		"1;9901;ALBO DELLE BANCHE;BANCA ESEMPIO VECCHIA S.P.A.;ROMA;00100;AAAAAAAAAAAAAAAAAA01;1990-01-01;2019-12-31",
		"1;9901;ALBO DELLE BANCHE;BANCA ESEMPIO S.P.A.;ROMA;00100;AAAAAAAAAAAAAAAAAA01;2020-01-01;9999-12-31",
		// Listed as lender and as payment institution: the payment register wins.
		"2;39901;ALBO DEGLI INTERMEDIARI FINANZIARI EX ART. 106 TUB (C.D. \"ALBO UNICO\");PAGAMENTI ESEMPIO SPA;MILANO;20100; ;2021-01-01;9999-12-31",
		"2;39901;ALBO IP ART.114-SEPTIES TUB;PAGAMENTI ESEMPIO SPA;MILANO;20100; ;2021-01-01;9999-12-31",
		// In the banks register and, listed first, in the payment register.
		"3;77;ALBO IP ART.114-SEPTIES TUB;BANCA PROVA (IP);TORINO;10100; ;2021-01-01;9999-12-31",
		"3;77;ALBO DELLE BANCHE;BANCA PROVA;TORINO;10100; ;2021-01-01;9999-12-31",
		"4;39902;ALBO IMEL ITA EX 114-QUATER;MONETA ESEMPIO S.P.A.;NAPOLI;80100; ;2022-01-01;9999-12-31",
		// Registers whose members hold no payment accounts.
		"5;19901;ALBO DEGLI INTERMEDIARI FINANZIARI EX ART. 106 TUB (C.D. \"ALBO UNICO\");FINANZIARIA ESEMPIO SPA;BARI;70100; ;2021-01-01;9999-12-31",
		"6;15901;ALBO DELLE SGR EX ART. 35 TUF - SEZIONE GESTORI DI FIA;GESTIONE ESEMPIO SGR;MILANO;20100; ;2021-01-01;9999-12-31",
		"7;36901;ALBO DEGLI IP - SEZIONE SPECIALE DEI PRESTATORI DEI SERVIZI D'INFORMAZIONE SUI CONTI (AISP);CONTI ESEMPIO S.R.L.;MILANO;20100; ;2021-01-01;9999-12-31",
		// A payment institution that was struck off.
		"8;39903;ALBO IP ART.114-SEPTIES TUB;PAGAMENTI CESSATI SPA;GENOVA;16100; ;2015-01-01;2018-06-30",
	}
	csv := "\xEF\xBB\xBF" + strings.Join(rows, "\r\n") + "\r\n"

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("2026-09-01_INTERMEDIARI.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(csv)); err != nil {
		t.Fatal(err)
	}
	if err := zw.SetComment("Example Archive"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	// The published archive stops one byte short of the end of its comment,
	// which archive/zip does not accept as it stands.
	data := buf.Bytes()[:buf.Len()-1]
	if _, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("the cut archive opened unrepaired, so the test no longer covers the repair")
	}

	banks, err := BancaDItalia{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 4 || len(got) != 4 {
		t.Fatalf("got %d records %v, want 4", len(banks), got)
	}
	// Padded to five digits, the current name only, LEI kept.
	if b := got["09901"]; b.Name != "BANCA ESEMPIO S.P.A." || b.LEI != "AAAAAAAAAAAAAAAAAA01" || b.Zip != "00100" || b.City != "ROMA" || b.Country != "IT" {
		t.Errorf("09901 = %+v", b)
	}
	// The single space of an empty cell is not an LEI.
	if b := got["39901"]; b.Name != "PAGAMENTI ESEMPIO SPA" || b.LEI != "" {
		t.Errorf("39901 = %+v", b)
	}
	if b := got["00077"]; b.Name != "BANCA PROVA" {
		t.Errorf("00077 = %+v, want the row from the banks register", b)
	}
	if b := got["39902"]; b.Name != "MONETA ESEMPIO S.P.A." {
		t.Errorf("39902 = %+v", b)
	}

	if d := (BancaDItalia{}).PublishedAt(data); d.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("PublishedAt = %v, want 2026-09-01", d)
	}

	// The CSV on its own parses the same way as the archive.
	plain, err := BancaDItalia{}.Parse([]byte(csv))
	if err != nil || len(plain) != 4 {
		t.Errorf("Parse of the bare CSV returned %d records, %v", len(plain), err)
	}
}
