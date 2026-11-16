package sources

import (
	"context"
	"regexp"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(BSI{}) }

// bsiURL is the Slovenian list of payment service provider identifiers. It is
// served as text/plain although the layout is a semicolon separated table.
const bsiURL = "https://www.bsi.si/en/d/list-of-identifiers-of-psp"

// BSI reads the Slovenian list of payment service provider identifiers.
//
// The file is semicolon separated and Windows-1250 encoded. Each line is one
// five digit identifier, the two digit bank number followed by a three digit
// branch number, which together are the bank code field of a Slovenian IBAN.
type BSI struct{}

func (BSI) Country() string { return "SI" }
func (BSI) Name() string    { return "Banka Slovenije" }

func (BSI) Terms() Terms {
	return Terms{
		Licence:     "CC BY 4.0",
		URL:         "https://www.bsi.si/en/disclaimer-and-copyright",
		Attribution: "Source: Banka Slovenije, CC BY 4.0",
	}
}

func (b BSI) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, bsiURL)
	return data, bsiURL, err
}

// bsiCode is the identifier as a Slovenian IBAN carries it.
var bsiCode = regexp.MustCompile(`^\d{5}$`)

func (b BSI) Parse(data []byte) ([]bankdata.Bank, error) {
	// Latin-1 cannot stand in for Windows-1250 here: the carons in Slovenian
	// names sit on bytes that Latin-1 maps to other characters.
	table, err := parseCSV(DecodeCharmap(data, charmap.Windows1250), ';', "national id", "bic")
	if err != nil {
		return nil, err
	}

	codeAt, err := table.mustIndex("NATIONAL ID")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("NAME")
	if err != nil {
		return nil, err
	}
	bicAt, _ := table.index("BIC11", "BIC")
	zipAt, _ := table.index("ZIP CODE", "ZIP")
	cityAt, _ := table.index("CITY")

	seen := make(map[string]bool)
	out := make([]bankdata.Bank, 0, len(table.rows))
	for _, row := range table.rows {
		code := field(row, codeAt)
		// The identifier is the IBAN field as it stands. Anything that is not
		// five digits is not one, and padding it would invent a code.
		if !bsiCode.MatchString(code) || seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "SI",
			BankCode: code,
			Name:     field(row, nameAt),
			Zip:      field(row, zipAt),
			City:     field(row, cityAt),
			BIC:      strings.ToUpper(strings.ReplaceAll(field(row, bicAt), " ", "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}
