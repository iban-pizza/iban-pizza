package sources

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// buildECBList packs rows the way the ECB publishes its list: tab separated,
// UTF-16 little endian with a byte order mark, gzipped.
func buildECBList(t *testing.T, rows [][]string) []byte {
	t.Helper()
	var text strings.Builder
	for _, row := range rows {
		text.WriteString(strings.Join(row, "\t") + "\r\n")
	}
	raw := []byte{0xFF, 0xFE}
	for _, unit := range utf16.Encode([]rune(text.String())) {
		raw = binary.LittleEndian.AppendUint16(raw, unit)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestECBPortugalParse(t *testing.T) {
	// Invented rows in the layout of the published list.
	data := buildECBList(t, [][]string{
		{"RIAD_CODE", "LEI", "COUNTRY_OF_REGISTRATION", "NAME", "BOX", "ADDRESS", "POSTAL", "CITY", "CATEGORY", "HEAD_COUNTRY_OF_REGISTRATION", "HEAD_NAME", "HEAD_RIAD_CODE", "HEAD_LEI", "REPORT"},
		// Another country, with a quoted name as the list writes some of them.
		{"LV40009990001", "", "LV", `Sabiedrība "Piemērs"`, "", "Piemēra iela 1", "LV-1001", "Rīga", "Credit Institution", "", "", "", "", ""},
		{"PT77", "AAAAAAAAAAAAAAAAAA01", "PT", "Banco Exemplo, S.A.", "", "Rua do Exemplo, 1", "1000-001", "LISBOA", "Credit Institution", "", "", "", "", ""},
		{"PT5990", "", "PT", "Caixa de Crédito Agrícola Mútuo de Exemplo, CRL", "", "Rua do Teste, 2", "4000-001", "PORTO", "Credit Institution", "", "", "", "", ""},
		// The central agricultural credit institution, filed under 9000.
		{"PT9000", "AAAAAAAAAAAAAAAAAA02", "PT", "Caixa Central de Exemplo, CRL", "", "Rua Central, 3", "1000-002", "LISBOA", "Credit Institution", "", "", "", "", ""},
		// A branch identified by tax number and a fund: no bank code in either.
		{"PT980999999", "", "PT", "BANQUE EXEMPLE - SUCURSAL EM PORTUGAL", "", "Avenida do Exemplo, 4", "1000-003", "LISBOA", "Credit Institution", "FR", "Banque Exemple", "FR99999", "", ""},
		{"PTIF9999", "", "PT", "FUNDO EXEMPLO DO MERCADO MONETÁRIO", "", "Avenida do Exemplo, 5", "1000-004", "LISBOA", "Money Market Fund", "", "", "", "", ""},
		// A code of the same shape registered elsewhere is not Portuguese.
		{"PT78", "", "ES", "Banco Ejemplo, S.A.", "", "Calle Ejemplo 6", "28001", "MADRID", "Credit Institution", "", "", "", "", ""},
	})

	banks, err := ECBPortugal{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 3 || len(got) != 3 {
		t.Fatalf("got %d records %v, want 3", len(banks), got)
	}
	if b := got["0077"]; b.Name != "Banco Exemplo, S.A." || b.LEI != "AAAAAAAAAAAAAAAAAA01" || b.Zip != "1000-001" || b.City != "LISBOA" || b.Country != "PT" || b.BIC != "" {
		t.Errorf("0077 = %+v", b)
	}
	// Characters outside ASCII survive the UTF-16 decoding.
	if b := got["5990"]; b.Name != "Caixa de Crédito Agrícola Mútuo de Exemplo, CRL" || b.LEI != "" {
		t.Errorf("5990 = %+v", b)
	}
	if b := got["0045"]; b.Name != "Caixa Central de Exemplo, CRL" || b.LEI != "AAAAAAAAAAAAAAAAAA02" {
		t.Errorf("0045 = %+v, want the institution the list files under PT9000", b)
	}
	if b, ok := got["9000"]; ok {
		t.Errorf("9000 was emitted: %+v", b)
	}
}

func TestECBInstitutions(t *testing.T) {
	data := buildECBList(t, [][]string{
		{"RIAD_CODE", "LEI", "COUNTRY_OF_REGISTRATION", "NAME", "POSTAL", "CITY"},
		{"LV40009990001", "aaaaaaaaaaaaaaaaaa03", "LV", `"""PIEMĒRA SABIEDRĪBA"""`, "LV-1001", "Rīga"},
		{"PT77", "", "PT", "Banco Exemplo, S.A.", "1000-001", "LISBOA"},
	})
	// The reader serves any country, and hands the RIAD code on untouched.
	got, err := ecbInstitutions(data, "LV")
	if err != nil {
		t.Fatalf("ecbInstitutions returned %v", err)
	}
	if len(got) != 1 || got[0].RIAD != "LV40009990001" || got[0].Name != `"PIEMĒRA SABIEDRĪBA"` || got[0].LEI != "AAAAAAAAAAAAAAAAAA03" || got[0].City != "Rīga" {
		t.Errorf("LV = %+v", got)
	}
	if _, err := ecbInstitutions([]byte("not the list"), "PT"); err == nil {
		t.Error("ecbInstitutions accepted data without a header")
	}
}

func TestECBLinks(t *testing.T) {
	// The feed lists releases newest first, and a release offers several
	// formats; both patterns have to pick the right one.
	feed := `<rss><channel>
	  <item><title>Euro foreign exchange reference rates</title><link>https://mid.example/rel/aaa.xml</link></item>
	  <item>
	    <title>List of monetary financial institutions (MFIs)</title>
	    <link>https://mid.example/rel/new.xml</link>
	  </item>
	  <item><title>List of monetary financial institutions (MFIs)</title><link>https://mid.example/rel/old.xml</link></item>
	</channel></rss>`
	if m := ecbFeedItem.FindStringSubmatch(feed); m == nil || m[1] != "https://mid.example/rel/new.xml" {
		t.Errorf("feed item = %q, want the newest release", m)
	}
	release := `<escb:attachment><escb:fileName>mfi_xml_260901.xml.gz</escb:fileName>
	  <escb:linkFile>https://mid.example/rel/new/mfi_xml_260901.xml.gz</escb:linkFile></escb:attachment>
	  <escb:attachment><escb:fileName>mfi_csv_260901.csv.gz</escb:fileName>
	  <escb:linkFile>https://mid.example/rel/new/mfi_csv_260901.csv.gz</escb:linkFile></escb:attachment>`
	if m := ecbReleaseFile.FindStringSubmatch(release); m == nil || m[1] != "https://mid.example/rel/new/mfi_csv_260901.csv.gz" {
		t.Errorf("release file = %q, want the CSV attachment", m)
	}
}
