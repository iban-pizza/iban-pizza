package sources

import (
	"context"
	"strings"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
)

func init() { Register(OeNB{}) }

// oenbURL is the Austrian SEPA payment directory, refreshed daily.
//
// It replaces the "kiverzeichnis" endpoint used by the upstream project, which
// now returns 404.
const oenbURL = "https://www.oenb.at/docroot/downloads_observ/sepa-zv-vz_gesamt.csv"

// OeNB reads the Austrian bank directory.
//
// The file is semicolon separated, ISO-8859-1 encoded, and carries three lines
// of legal notice plus a query date above the header row.
type OeNB struct{}

func (OeNB) Country() string { return "AT" }
func (OeNB) Name() string    { return "Oesterreichische Nationalbank" }

func (o OeNB) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, oenbURL)
	return data, oenbURL, err
}

func (o OeNB) Parse(data []byte) ([]bankdata.Bank, error) {
	table, err := parseCSV(DecodeAuto(data), ';', "bankleitzahl", "bankenname")
	if err != nil {
		return nil, err
	}

	codeAt, err := table.mustIndex("Bankleitzahl")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("Bankenname")
	if err != nil {
		return nil, err
	}
	bicAt, _ := table.index("SWIFT-Code", "SWIFTCode", "BIC")
	zipAt, _ := table.index("PLZ")
	cityAt, _ := table.index("Ort")

	// One institution can appear more than once. Keep the first occurrence so
	// the result does not depend on file order.
	seen := make(map[string]bool)
	out := make([]bankdata.Bank, 0, len(table.rows))
	for _, row := range table.rows {
		code := field(row, codeAt)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "AT",
			BankCode: code,
			Name:     field(row, nameAt),
			Zip:      field(row, zipAt),
			City:     field(row, cityAt),
			BIC:      strings.ToUpper(strings.ReplaceAll(field(row, bicAt), " ", "")),
			Source:   o.Name(),
		})
	}
	return out, nil
}
