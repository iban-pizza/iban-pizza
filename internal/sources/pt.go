package sources

import (
	"context"
	"regexp"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(ECBPortugal{}) }

// ECBPortugal reads the Portuguese institutions out of the ECB list.
//
// The Banco de Portugal publishes its own register only as a legacy binary
// spreadsheet, which this program does not read. The ECB list carries the
// same institutions, and for those established in Portugal its RIAD code is
// "PT" followed by the bank code without leading zeros.
type ECBPortugal struct{}

func (ECBPortugal) Country() string { return "PT" }
func (ECBPortugal) Name() string    { return ecbName }

// Partial reports that an unknown bank code is not proof of a wrong one here.
func (ECBPortugal) Partial() string {
	return "The ECB list covers institutions registered in Portugal; branches of foreign institutions with a Portuguese bank code are missing."
}
func (ECBPortugal) Terms() Terms { return ecbTerms }

func (ECBPortugal) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	return ecbFetch(ctx, c)
}

// ecbPortugalCode matches the RIAD codes that embed a bank code. The others
// are built on something else and say nothing about the IBAN: branches of
// foreign banks carry "PT" and a nine digit tax number, money market funds
// "PTIF" and a fund number.
var ecbPortugalCode = regexp.MustCompile(`^PT(\d{1,4})$`)

// ecbPortugalExceptions maps the RIAD codes that do not embed the bank code
// to the code the institution uses in its IBANs.
//
// PT9000 is the Caixa Central de Crédito Agrícola Mútuo, the central
// institution of the agricultural credit group, whose accounts carry bank
// code 0045. Left alone it would be filed under 9000, where no IBAN would
// ever find it.
var ecbPortugalExceptions = map[string]string{
	"9000": "0045",
}

func (p ECBPortugal) Parse(data []byte) ([]bankdata.Bank, error) {
	institutions, err := ecbInstitutions(data, "PT")
	if err != nil {
		return nil, err
	}

	out := make([]bankdata.Bank, 0, len(institutions))
	seen := map[string]bool{}
	for _, in := range institutions {
		m := ecbPortugalCode.FindStringSubmatch(in.RIAD)
		if m == nil {
			continue
		}
		// The RIAD code drops the leading zeros that the IBAN keeps.
		code := strings.Repeat("0", 4-len(m[1])) + m[1]
		if actual, ok := ecbPortugalExceptions[code]; ok {
			code = actual
		}
		if seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "PT",
			BankCode: code,
			Name:     in.Name,
			Zip:      in.Zip,
			City:     in.City,
			LEI:      in.LEI,
			Source:   p.Name(),
		})
	}
	return out, nil
}
