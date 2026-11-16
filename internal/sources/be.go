package sources

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(NBB{}) }

// nbbURL is the Belgian list of bank identification codes. The "_current"
// name is stable across revisions.
const nbbURL = "https://www.nbb.be/doc/be/be/protocol/full_list_current.xlsx"

// NBB reads the Belgian bank code list.
//
// The sheet has one row for every code from 000 to 999, most of them
// unassigned, and names each institution in up to four languages.
type NBB struct{}

func (NBB) Country() string { return "BE" }
func (NBB) Name() string    { return "National Bank of Belgium" }

func (NBB) Terms() Terms {
	return Terms{
		Licence:     "NBB copyright and use of information",
		URL:         "https://www.nbb.be/en/disclaimer-and-legal-information/copyright-and-use-information",
		Attribution: "Source: National Bank of Belgium",
		Note: "May be distributed or reproduced with integrity and accuracy maintained and the NBB named; " +
			"anyone passing it on for a fee must say it is free on nbb.be.",
	}
}

func (b NBB) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, nbbURL)
	return data, nbbURL, err
}

// nbbPlaceholders are the values the BIC column holds where no BIC applies:
// a free code, or an institution without one.
var nbbPlaceholders = map[string]bool{"VRIJ": true, "N/A": true, "NAV": true, "NYA": true, "-": true}

// nbbNotABank are the names the list gives to codes no institution holds.
var nbbNotABank = map[string]bool{
	"VRIJ": true, "LIBRE": true, "FREI": true, "FREE": true,
	"ONBESCHIKBAAR": true, "INDISPONIBLE": true, "NICHT VERFÜGBAR": true, "UNAVAILABLE": true,
}

func (b NBB) Parse(data []byte) ([]bankdata.Bank, error) {
	rows, err := readXLSX(data, "")
	if err != nil {
		return nil, err
	}
	table, err := tableFromRows(rows, "T_Identification_Number", "Biccode")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("T_Identification_Number")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("Biccode")
	if err != nil {
		return nil, err
	}
	// The name is not always given in every language, so take the first one
	// present, preferring English for an international audience.
	var nameAt []int
	for _, col := range []string{"T_Institutions_English", "T_Institutions_Dutch", "T_Institutions_French", "T_Institutions_German"} {
		if i, ok := table.index(col); ok {
			nameAt = append(nameAt, i)
		}
	}

	out := make([]bankdata.Bank, 0, len(table.rows))
	for _, row := range table.rows {
		code := field(row, codeAt)
		if len(code) > 3 || code == "" {
			continue
		}
		// A numeric cell loses its leading zeros.
		code = strings.Repeat("0", 3-len(code)) + code

		bic := strings.ToUpper(strings.ReplaceAll(field(row, bicAt), " ", ""))
		if nbbPlaceholders[bic] {
			bic = ""
		}
		var name string
		for _, i := range nameAt {
			if name = field(row, i); name != "" {
				break
			}
		}
		// Free codes carry the word for "free" as their name, and codes held
		// back from assignment the word for "unavailable".
		if name == "" || nbbNotABank[strings.ToUpper(name)] {
			continue
		}

		out = append(out, bankdata.Bank{
			Country:  "BE",
			BankCode: code,
			Name:     name,
			BIC:      bic,
			Source:   b.Name(),
		})
	}
	return out, nil
}

var nbbVersion = regexp.MustCompile(`Version\s+(\d{2}/\d{2}/\d{4})`)

// PublishedAt reads the "Version dd/mm/yyyy" line above the header.
func (NBB) PublishedAt(data []byte) time.Time {
	rows, err := readXLSX(data, "")
	if err != nil {
		return time.Time{}
	}
	for _, row := range rows[:min(len(rows), 5)] {
		if m := nbbVersion.FindStringSubmatch(strings.Join(row, " ")); m != nil {
			if t, err := time.Parse("02/01/2006", m[1]); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}
