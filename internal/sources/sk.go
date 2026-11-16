package sources

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(NBS{}) }

// nbsURL is the Slovak directory of identification codes. It exists only as a
// table on this page.
const nbsURL = "https://nbs.sk/en/payments/general-information/directories-and-registers/directory-identification-codes-domestic-payment-system-in-sr/"

// NBS reads the Slovak directory of identification codes for the domestic
// payment system.
//
// Each row gives the BIC split over three cells (bank, country, location),
// then the four digit numeric code that a Slovak IBAN carries.
type NBS struct{}

func (NBS) Country() string { return "SK" }
func (NBS) Name() string    { return "Narodna banka Slovenska" }

func (NBS) Terms() Terms {
	return Terms{
		Licence:     "NBS disclaimer",
		URL:         "https://nbs.sk/en/disclaimer_en/",
		Attribution: "Source: Národná banka Slovenska",
		Note: "May be stored, reproduced and used with the source acknowledged; " +
			"the content and properties of the file may not be modified.",
	}
}

func (n NBS) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, nbsURL)
	return data, nbsURL, err
}

var (
	nbsCode     = regexp.MustCompile(`^\d{4}$`)
	nbsBank     = regexp.MustCompile(`^[A-Z0-9]{4}$`)
	nbsCountry  = regexp.MustCompile(`^[A-Z]{2}$`)
	nbsLocation = regexp.MustCompile(`^[A-Z0-9]{2}$`)
)

func (n NBS) Parse(data []byte) ([]bankdata.Bank, error) {
	seen := make(map[string]bool)
	var out []bankdata.Bank
	for _, row := range htmlRows(DecodeAuto(data)) {
		codeAt := nbsCodeCell(row)
		if codeAt < 0 {
			continue
		}
		code := field(row, codeAt)
		if seen[code] {
			continue
		}
		seen[code] = true

		// Rows are kept whatever country the BIC names. The Czech banks
		// listed further down the page, and the run-off codes of merged
		// banks, are all codes that a Slovak IBAN can carry.
		out = append(out, bankdata.Bank{
			Country:  "SK",
			BankCode: code,
			Name:     field(row, codeAt+3),
			BIC:      nbsCell(row, codeAt-3) + nbsCell(row, codeAt-2) + nbsCell(row, codeAt-1),
			Source:   n.Name(),
		})
	}
	return out, nil
}

// nbsCodeCell returns the position of the numeric code in a directory row, or
// -1 when the row is not one.
//
// A row is recognised by its shape rather than by where it sits: a four digit
// cell preceded by the three BIC cells and followed by the alphabetic code,
// the active party flag and the name. Counting columns from the left would
// tie the parser to the change marker column, and to the page keeping one
// table. A few providers have no BIC, and their three cells are empty.
func nbsCodeCell(row []string) int {
	for i := 3; i+3 < len(row); i++ {
		if !nbsCode.MatchString(field(row, i)) || field(row, i+3) == "" {
			continue
		}
		bank, country, location := nbsCell(row, i-3), nbsCell(row, i-2), nbsCell(row, i-1)
		if bank+country+location == "" {
			return i
		}
		if nbsBank.MatchString(bank) && nbsCountry.MatchString(country) && nbsLocation.MatchString(location) {
			return i
		}
	}
	return -1
}

// nbsCell returns a BIC cell in upper case without spaces.
func nbsCell(row []string, i int) string {
	return strings.ToUpper(strings.ReplaceAll(field(row, i), " ", ""))
}

var nbsEffective = regexp.MustCompile(`Effective from:\s*(\d{1,2})\.\s*(\d{1,2})\.\s*(\d{4})`)

// PublishedAt reads the "Effective from: d. m. yyyy" line above the table.
func (NBS) PublishedAt(data []byte) time.Time {
	// The date is split over tags and non-breaking spaces, so match it in
	// the visible text rather than in the markup.
	text := htmlText(htmlComment.ReplaceAllString(DecodeAuto(data), ""))
	m := nbsEffective.FindStringSubmatch(text)
	if m == nil {
		return time.Time{}
	}
	t, err := time.Parse("2.1.2006", m[1]+"."+m[2]+"."+m[3])
	if err != nil {
		return time.Time{}
	}
	return t
}
