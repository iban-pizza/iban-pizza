package sources

import (
	"context"
	"regexp"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(ABBL{}) }

// abblPage is the publication page of the Luxembourg register. The download
// sits on another host under an opaque token and a version number, both of
// which change with each release, so the link is read from the page.
const abblPage = "https://www.abbl.lu/publications/abbl-luxembourg-register-of-iban-bic-codes/"

var abblLink = regexp.MustCompile(`(https://office-membernet\.abbl\.lu/[^"'\s<>]+\.xlsx)`)

// ABBL reads the Luxembourg register of IBAN and BIC codes, which the
// bankers' association maintains.
type ABBL struct{}

func (ABBL) Country() string { return "LU" }
func (ABBL) Name() string    { return "ABBL" }

func (ABBL) Terms() Terms {
	return Terms{
		Licence:     "ABBL legal notice",
		URL:         "https://www.abbl.lu/legal-notice/",
		Attribution: "Source: ABBL, Luxembourg register of IBAN/BIC codes",
		Note:        "For information or reference purposes only, unmodified and with the source indicated.",
	}
}

func (ABBL) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	url, err := c.ResolveLink(ctx, abblPage, abblLink)
	if err != nil {
		return nil, abblPage, err
	}
	data, err := c.Get(ctx, url)
	return data, url, err
}

func (b ABBL) Parse(data []byte) ([]bankdata.Bank, error) {
	rows, err := readXLSX(data, "Organizations")
	if err != nil {
		return nil, err
	}
	// A title row sits above the header.
	table, err := tableFromRows(rows, "Credit institution", "IBAN Code", "BICCode")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("Credit institution")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("IBAN Code")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("BICCode")
	if err != nil {
		return nil, err
	}

	out := make([]bankdata.Bank, 0, len(table.rows))
	seen := make(map[string]bool, len(table.rows))
	for _, row := range table.rows {
		code := field(row, codeAt)
		name := field(row, nameAt)
		if !isDigits(code, 3) || name == "" {
			continue
		}
		// The column mixes text and number cells, and a number loses its
		// leading zeros.
		code = strings.Repeat("0", 3-len(code)) + code
		if seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "LU",
			BankCode: code,
			Name:     name,
			BIC:      strings.ToUpper(strings.Join(strings.Fields(field(row, bicAt)), "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}
