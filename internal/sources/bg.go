package sources

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(BNB{}) }

// bnbURL is the Bulgarian register of BAE codes. It exists only as tables on
// this page.
const bnbURL = "https://www.bnb.bg/RegistersAndServices/RSBAEAndBIC/index.htm"

// BNB reads the Bulgarian register of BAE codes.
//
// A BAE code is eight characters: the four letter bank code that a Bulgarian
// IBAN carries, then four characters for the branch. The register lists every
// branch, so one bank appears on many rows, and only the head office row
// carries the BIC.
type BNB struct{}

func (BNB) Country() string { return "BG" }
func (BNB) Name() string    { return "Bulgarian National Bank" }

func (BNB) Terms() Terms {
	return Terms{
		Licence:     "BNB terms of use",
		URL:         "https://www.bnb.bg/AboutUs/PressOffice/PORightsUsing/index.htm",
		Attribution: "Source: Българска народна банка (Bulgarian National Bank)",
		Note:        "Data may be distributed and reproduced with the source cited and the material not altered or distorted.",
	}
}

func (b BNB) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, bnbURL)
	return data, bnbURL, err
}

var (
	bnbBAE = regexp.MustCompile(`^[A-Z]{4}[A-Z0-9]{4}$`)
	bnbBIC = regexp.MustCompile(`^[A-Z0-9]{8}([A-Z0-9]{3})?$`)
)

func (b BNB) Parse(data []byte) ([]bankdata.Bank, error) {
	// The page is a run of small tables, one per bank and one per branch
	// list, with the header in a table of its own. htmlRows flattens them, so
	// the first header names the columns for everything below it.
	table, err := tableFromRows(htmlRows(DecodeAuto(data)), "БАЕ код", "BIC код")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("БАЕ код")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("BIC код (ЦУ)", "BIC код")
	if err != nil {
		return nil, err
	}
	// The name column's heading differs between the sub-tables, and it is the
	// first column in all of them.
	nameAt, ok := table.index("Наименование на ДПУ")
	if !ok {
		nameAt = 0
	}

	// One record per four letter code. The position of each code in out is
	// remembered so that a head office row arriving after its branches can
	// still replace the branch name taken first.
	at := make(map[string]int)
	var out []bankdata.Bank
	for _, row := range table.rows {
		bae := strings.ToUpper(strings.ReplaceAll(field(row, codeAt), " ", ""))
		// Repeated header rows and anything else that is not a register row
		// fail this test.
		if !bnbBAE.MatchString(bae) {
			continue
		}
		code := bae[:4]
		name := field(row, nameAt)
		bic := strings.ToUpper(strings.ReplaceAll(field(row, bicAt), " ", ""))
		if !bnbBIC.MatchString(bic) {
			bic = ""
		}

		i, known := at[code]
		switch {
		case !known:
			at[code] = len(out)
			out = append(out, bankdata.Bank{
				Country:  "BG",
				BankCode: code,
				Name:     name,
				BIC:      bic,
				Source:   b.Name(),
			})
		case out[i].BIC == "" && bic != "":
			// The row with the BIC is the head office: its name is the
			// bank's, where the one held so far is a branch's.
			out[i].Name, out[i].BIC = name, bic
		}
	}
	return out, nil
}

// bnbDated matches the date in the register's heading, "... от 13.11.2026".
var bnbDated = regexp.MustCompile(`Регистър на БАЕ кодовете[^<]*?\sот\s+(\d{1,2}\.\d{1,2}\.\d{4})`)

// PublishedAt reads the date in the heading above the register.
func (BNB) PublishedAt(data []byte) time.Time {
	// An older register with its own, older date is left on the page inside
	// an HTML comment. Dropping comments first keeps it from being read.
	page := htmlComment.ReplaceAllString(DecodeAuto(data), "")
	m := bnbDated.FindStringSubmatch(page)
	if m == nil {
		return time.Time{}
	}
	t, err := time.Parse("2.1.2006", m[1])
	if err != nil {
		return time.Time{}
	}
	return t
}
