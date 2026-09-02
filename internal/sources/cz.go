package sources

import (
	"context"
	"strings"

	"github.com/netzfabrikcom/iban-pizza/bankdata"
)

func init() { Register(CNB{}) }

// cnbURL is the Czech National Bank payment code list.
//
// It is served with Content-Type text/html even though the body is CSV, which
// is why the fetch layer does not gate on the content type.
const cnbURL = "https://www.cnb.cz/cs/platebni-styk/.galleries/ucty_kody_bank/download/kody_bank_CR.csv"

// CNB reads the Czech bank code list.
type CNB struct{}

func (CNB) Country() string { return "CZ" }
func (CNB) Name() string    { return "Ceska narodni banka" }

func (b CNB) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, cnbURL)
	return data, cnbURL, err
}

func (b CNB) Parse(data []byte) ([]bankdata.Bank, error) {
	table, err := parseCSV(DecodeAuto(data), ';', "bic")
	if err != nil {
		return nil, err
	}

	codeAt, err := table.mustIndex("Kod platebniho styku", "Kód platebního styku")
	if err != nil {
		// The first column is the payment code regardless of its exact
		// spelling, which varies with the file's diacritics handling.
		codeAt = 0
	}
	nameAt, err := table.mustIndex("Poskytovatel platebnich sluzeb", "Poskytovatel platebních služeb")
	if err != nil {
		nameAt = 1
	}
	bicAt, ok := table.index("BIC kod (SWIFT)", "BIC kód (SWIFT)", "BIC")
	if !ok {
		bicAt = 2
	}

	out := make([]bankdata.Bank, 0, len(table.rows))
	for _, row := range table.rows {
		code := field(row, codeAt)
		if code == "" {
			continue
		}
		out = append(out, bankdata.Bank{
			Country:  "CZ",
			BankCode: code,
			Name:     field(row, nameAt),
			BIC:      strings.ToUpper(strings.ReplaceAll(field(row, bicAt), " ", "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}
