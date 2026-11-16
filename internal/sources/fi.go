package sources

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(Finanssiala{}) }

// finanssialaPage is the Finance Finland topic page on monetary institution
// codes. The spreadsheet it links has a date in its name and sits in a folder
// named after the month it was uploaded, so the link is read from the page.
const finanssialaPage = "https://www.finanssiala.fi/en/topics/finnish-monetary-financial-institution-identifiers-and-bics/"

// The page links the same list as a PDF as well; only the spreadsheet is
// wanted.
var finanssialaLink = regexp.MustCompile(`(?i)href="([^"]*/wp-content/uploads/[^"]*finnish_monetary_institution_codes[^"]*\.xlsx)"`)

// Finanssiala reads the Finnish list of monetary institution codes.
//
// The first three digits of a Finnish account number identify the bank. The
// sheet has one row for each such number, so a bank that holds a range of
// them appears once for every number in the range.
type Finanssiala struct{}

func (Finanssiala) Country() string { return "FI" }
func (Finanssiala) Name() string    { return "Finance Finland" }

func (Finanssiala) Terms() Terms {
	return Terms{
		Licence:     "No terms of use published",
		URL:         finanssialaPage,
		Attribution: "Source: Finance Finland (Finanssiala ry)",
		Note:        "The publisher states no reuse terms.",
	}
}

func (Finanssiala) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	url, err := c.ResolveLink(ctx, finanssialaPage, finanssialaLink)
	if err != nil {
		return nil, finanssialaPage, err
	}
	data, err := c.Get(ctx, url)
	return data, url, err
}

// finanssialaSheet is named because the workbook has two empty sheets after
// the list.
const finanssialaSheet = "Taul1"

func (b Finanssiala) Parse(data []byte) ([]bankdata.Bank, error) {
	rows, err := readXLSX(data, finanssialaSheet)
	if err != nil {
		return nil, err
	}
	table, err := tableFromRows(rows, "National ID", "BIC Code", "Financial Institution Name")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("National ID")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("BIC Code")
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
		code := field(row, codeAt)
		name := field(row, nameAt)
		if !isDigits(code, 3) || name == "" {
			continue
		}
		// The numbers are text today; a numeric cell would lose leading zeros.
		code = strings.Repeat("0", 3-len(code)) + code
		// Some numbers are listed twice, in places with a second BIC. A
		// lookup can return one bank only, so the first row stands.
		if seen[code] {
			continue
		}
		seen[code] = true

		out = append(out, bankdata.Bank{
			Country:  "FI",
			BankCode: code,
			Name:     name,
			BIC:      strings.ToUpper(strings.Join(strings.Fields(field(row, bicAt)), "")),
			Source:   b.Name(),
		})
	}
	return out, nil
}

var finanssialaDate = regexp.MustCompile(`^\d{1,2}\.\d{1,2}\.\d{4}$`)

// PublishedAt reads the date in the first cell, written the Finnish way as
// day.month.year without leading zeros.
func (Finanssiala) PublishedAt(data []byte) time.Time {
	rows, err := readXLSX(data, finanssialaSheet)
	if err != nil || len(rows) == 0 {
		return time.Time{}
	}
	cell := field(rows[0], 0)
	if !finanssialaDate.MatchString(cell) {
		return time.Time{}
	}
	t, err := time.Parse("2.1.2006", cell)
	if err != nil {
		return time.Time{}
	}
	return t
}
