package sources

import (
	"context"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(CBM{}) }

// cbmURL is the Maltese list of the bank identifiers found in IBANs. The name
// is stable across revisions.
const cbmURL = "https://www.centralbankmalta.org/site/Payment-Systems/BIC-from-IBAN.xlsx"

// CBM reads the Maltese list.
//
// A Maltese IBAN carries the first four letters of the institution's BIC
// rather than a number, and the list maps those four letters to the full BIC.
type CBM struct{}

func (CBM) Country() string { return "MT" }
func (CBM) Name() string    { return "Central Bank of Malta" }

func (CBM) Terms() Terms {
	return Terms{
		Licence:     "Central Bank of Malta disclaimer and copyright",
		URL:         "https://www.centralbankmalta.org/disclaimercopyright",
		Attribution: "Source: Central Bank of Malta",
		Note:        "May be distributed and reproduced with the source stated and the material not altered or distorted.",
	}
}

func (CBM) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, cbmURL)
	return data, cbmURL, err
}

func (b CBM) Parse(data []byte) ([]bankdata.Bank, error) {
	// The workbook has two empty sheets after the list, so name the one meant.
	rows, err := readXLSX(data, "Sheet1")
	if err != nil {
		return nil, err
	}
	table, err := tableFromRows(rows, "BIC Found in IBAN", "Financial Institution Name")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("BIC Found in IBAN (4 Characters)", "BIC Found in IBAN")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("Full BIC (8 Characters)", "Full BIC")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("Financial Institution Name")
	if err != nil {
		return nil, err
	}

	out := make([]bankdata.Bank, 0, len(table.rows))
	seen := make(map[string]bool, len(table.rows))
	for _, row := range table.rows {
		code := strings.ToUpper(field(row, codeAt))
		name := field(row, nameAt)
		if !isBankLetters(code) || name == "" {
			continue
		}
		// An institution with two BICs is listed once for each under the same
		// four letters. The IBAN cannot tell them apart, so the first stands.
		if seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "MT",
			BankCode: code,
			Name:     name,
			BIC:      strings.ToUpper(strings.Join(strings.Fields(field(row, bicAt)), "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}

// isBankLetters reports whether s is the four letters a Maltese IBAN holds.
func isBankLetters(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
