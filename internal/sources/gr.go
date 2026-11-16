package sources

import (
	"context"
	"regexp"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(HBA{}) }

// hbaURL is the list of credit institutions in the HEBIC index. It is served
// as application/octet-stream although the body is delimited text.
const hbaURL = "https://www.hba.gr/En/Hebic/downloadbanks"

// HBA reads the Greek list of credit institutions from the HEBIC index of the
// Hellenic Bank Association.
//
// The file is semicolon separated and Windows-1253 encoded, with a title line
// above the header. It carries neither a BIC nor an LEI, so the records stay
// without BIC: there is nothing in this source to join one on.
type HBA struct{}

func (HBA) Country() string { return "GR" }
func (HBA) Name() string    { return "Hellenic Bank Association" }

// Partial reports that an unknown bank code is not proof of a wrong one here.
func (HBA) Partial() string {
	return "The index lists credit institutions; payment and electronic money institutions that issue Greek IBANs are missing."
}

func (HBA) Terms() Terms {
	return Terms{
		Licence:     "Hellenic Bank Association terms and conditions",
		URL:         "https://www.hba.gr/En/Hebic",
		Attribution: "Source: Hellenic Bank Association, HEBIC index",
		Note:        "Reproduction and republication are permitted with the source mentioned.",
	}
}

func (h HBA) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, hbaURL)
	return data, hbaURL, err
}

var hbaCode = regexp.MustCompile(`^\d{3}$`)

func (h HBA) Parse(data []byte) ([]bankdata.Bank, error) {
	table, err := parseCSV(DecodeCharmap(data, charmap.Windows1253), ';', "code number", "name of credit institution")
	if err != nil {
		return nil, err
	}

	// Only the first two columns are safe to address. The file has no
	// quoting, so a semicolon inside an address adds a field and moves every
	// column after it; code and name come before the address and cannot move.
	codeAt, err := table.mustIndex("Code Number")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("Name of Credit Institution")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	out := make([]bankdata.Bank, 0, len(table.rows))
	for _, row := range table.rows {
		// Codes are wrapped in apostrophes ('011') so that a spreadsheet
		// keeps their leading zeros.
		code := strings.TrimSpace(strings.Trim(field(row, codeAt), "'"))
		name := field(row, nameAt)
		if !hbaCode.MatchString(code) || name == "" || seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "GR",
			BankCode: code,
			Name:     name,
			Source:   h.Name(),
		})
	}
	return out, nil
}
