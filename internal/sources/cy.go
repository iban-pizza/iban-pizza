package sources

import (
	"context"
	"regexp"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(CBC{}) }

// cbcPage is the Central Bank of Cyprus page on the IBAN. The file it links
// carries the month of its last update in its name, so the link is read from
// the page on every run.
const cbcPage = "https://www.centralbank.cy/en/financial-market-infrastructures-payments/international-bank-account-number-iban"

// cbcLink matches the list of BICs and nothing else in that folder. The page
// also links the full IBAN register next to it, which is a legacy binary .xls
// that cannot be read here, so the name has to say "BICs" and end in .xlsx.
var cbcLink = regexp.MustCompile(`(?i)href="([^"]*/FMIP/IBAN/[^"]*BICs[^"]*\.xlsx)"`)

// CBC reads the Cypriot list of bank identifiers and BICs.
//
// One sheet holds two tables under each other: credit institutions first,
// then electronic money institutions under a header row of their own.
type CBC struct{}

func (CBC) Country() string { return "CY" }
func (CBC) Name() string    { return "Central Bank of Cyprus" }

func (CBC) Terms() Terms {
	return Terms{
		Licence:     "Central Bank of Cyprus terms of use",
		URL:         "https://www.centralbank.cy/en/terms-of-use",
		Attribution: "Source: Central Bank of Cyprus",
		Note:        "Free to use provided it is reproduced accurately and the source is cited; modifications must be stated.",
	}
}

func (CBC) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	url, err := c.ResolveLink(ctx, cbcPage, cbcLink)
	if err != nil {
		return nil, cbcPage, err
	}
	data, err := c.Get(ctx, url)
	return data, url, err
}

func (b CBC) Parse(data []byte) ([]bankdata.Bank, error) {
	rows, err := readXLSX(data, "")
	if err != nil {
		return nil, err
	}
	table, err := tableFromRows(rows, "Bank identifiers used in IBAN", "Full BIC")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("Bank identifiers used in IBAN")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("Full BIC (8 Characters)", "Full BIC")
	if err != nil {
		return nil, err
	}
	// The second table names this column "Electronic Money Institution", but
	// it sits in the same place, so the first header serves both.
	nameAt, err := table.mustIndex("Credit Institution")
	if err != nil {
		return nil, err
	}

	out := make([]bankdata.Bank, 0, len(table.rows))
	seen := make(map[string]bool, len(table.rows))
	for _, row := range table.rows {
		// Requiring a numeric code drops everything that is not an
		// institution: the second header row, the blank rows, the "Updated
		// on" footer, and the rows that give a further BIC for the institution
		// above them and leave the code empty. The first BIC listed is kept.
		code := field(row, codeAt)
		if !isDigits(code, 3) {
			continue
		}
		// Some codes are stored as numbers and lose their leading zeros.
		code = strings.Repeat("0", 3-len(code)) + code

		name := field(row, nameAt)
		if name == "" || seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "CY",
			BankCode: code,
			Name:     name,
			BIC:      strings.ToUpper(strings.Join(strings.Fields(field(row, bicAt)), "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}

// isDigits reports whether s is one to max decimal digits.
func isDigits(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
