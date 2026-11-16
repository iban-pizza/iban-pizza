package sources

import (
	"context"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(HNB{}) }

// hnbURL is the Croatian list of leading numbers of payment service
// providers. The document address is stable across revisions.
const hnbURL = "https://www.hnb.hr/documents/d/guest/h-vodeci-brojevi-pruzatelja-platnih-usluga"

// HNB reads the Croatian list. The "vodeći broj" (leading number) is the
// seven digit bank code of the IBAN.
type HNB struct{}

func (HNB) Country() string { return "HR" }
func (HNB) Name() string    { return "Croatian National Bank" }

func (HNB) Terms() Terms {
	return Terms{
		Licence:     "HNB terms of use",
		URL:         "https://www.hnb.hr/uvjeti-koristenja",
		Attribution: "Izvor: Hrvatska narodna banka",
		Note:        "Copying is allowed with the source named; the content may not be modified.",
	}
}

func (HNB) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, hnbURL)
	return data, hnbURL, err
}

func (b HNB) Parse(data []byte) ([]bankdata.Bank, error) {
	rows, err := readXLSX(data, "")
	if err != nil {
		return nil, err
	}
	// The header is neither on the first row nor in the first column, and the
	// BIC header is broken over two lines, so all three are found by name.
	table, err := tableFromRows(rows, "Naziv pružatelja platnih usluga", "Vodeći broj")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("Naziv pružatelja platnih usluga")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("Vodeći broj")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("SWIFT adresa (BIC)", "SWIFT adresa", "BIC")
	if err != nil {
		return nil, err
	}

	out := make([]bankdata.Bank, 0, len(table.rows))
	seen := make(map[string]bool, len(table.rows))
	for _, row := range table.rows {
		code := field(row, codeAt)
		name := field(row, nameAt)
		if !isDigits(code, 7) || name == "" {
			continue
		}
		// The code is stored as a number, which would drop a leading zero.
		code = strings.Repeat("0", 7-len(code)) + code
		if seen[code] {
			continue
		}
		seen[code] = true

		// BICs are written in groups, "ABCD HR 2X", some with a trailing
		// space. Providers that are not banks have none, and are still listed
		// because their leading number appears in IBANs all the same.
		out = append(out, bankdata.Bank{
			Country:  "HR",
			BankCode: code,
			Name:     name,
			BIC:      strings.ToUpper(strings.Join(strings.Fields(field(row, bicAt)), "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}
