package sources

import (
	"fmt"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

// GenericName is the source name recorded for records loaded through the
// generic format when the caller supplies none.
const GenericName = "generic import"

// GenericColumns documents the header the generic format expects. Column order
// does not matter and names are matched case insensitively with spaces,
// hyphens and underscores ignored, so "Bank Code", "bank_code" and "bankCode"
// all resolve to the same column.
var GenericColumns = []string{
	"bankCode", "name", "bic", "zip", "city", "shortName", "checkAlgo", "country",
}

// ParseGeneric reads a delimited file with a header row naming the columns.
//
// It exists for the countries that have no built in parser: a registry a
// customer licensed and may not redistribute, a source that publishes only a
// PDF the customer has extracted, or an internal list. Only bankCode and name
// are required. A country column overrides the country argument per row,
// which lets one file carry several countries.
func ParseGeneric(country, source string, data []byte) ([]bankdata.Bank, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if source == "" {
		source = GenericName
	}

	text := DecodeAuto(data)
	sep := detectSeparator(text)

	table, err := parseCSV(text, sep, "bankcode", "name")
	if err != nil {
		return nil, fmt.Errorf("generic import: %w (expected a header row with at least bankCode and name)", err)
	}

	codeAt, err := table.mustIndex("bankCode")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("name")
	if err != nil {
		return nil, err
	}
	bicAt, _ := table.index("bic", "swift", "swiftCode")
	zipAt, _ := table.index("zip", "postcode", "plz")
	cityAt, _ := table.index("city", "ort", "town")
	shortAt, _ := table.index("shortName")
	algoAt, _ := table.index("checkAlgo", "checkDigitMethod")
	countryAt, hasCountry := table.index("country")

	out := make([]bankdata.Bank, 0, len(table.rows))
	seen := make(map[bankdata.Key]bool, len(table.rows))
	for n, row := range table.rows {
		code := field(row, codeAt)
		name := field(row, nameAt)
		if code == "" || name == "" {
			continue
		}

		cc := country
		if hasCountry {
			if v := strings.ToUpper(field(row, countryAt)); v != "" {
				cc = v
			}
		}
		if len(cc) != 2 {
			return nil, fmt.Errorf("generic import: row %d has no usable country code (%q)", n+2, cc)
		}

		b := bankdata.Bank{
			Country:   cc,
			BankCode:  code,
			Name:      name,
			ShortName: field(row, shortAt),
			Zip:       field(row, zipAt),
			City:      field(row, cityAt),
			BIC:       strings.ToUpper(strings.ReplaceAll(field(row, bicAt), " ", "")),
			CheckAlgo: strings.ToUpper(field(row, algoAt)),
			Source:    source,
		}
		key := b.Key().Normalize()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, b)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("generic import: no rows with both bankCode and name")
	}
	return out, nil
}

// detectSeparator picks the delimiter from the header line. Registries use
// both, and asking the caller to know which is one question too many.
func detectSeparator(text string) rune {
	header := text
	if i := strings.IndexAny(text, "\r\n"); i >= 0 {
		header = text[:i]
	}
	if strings.Count(header, ";") > strings.Count(header, ",") {
		return ';'
	}
	return ','
}

// ParseFile parses a local file for a country, using the built in parser when
// one is registered and the generic format otherwise. The boolean reports
// which path was taken, so a caller can say so.
func ParseFile(country, source string, data []byte) ([]bankdata.Bank, string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if src, ok := Get(country); ok {
		banks, err := src.Parse(data)
		if err != nil {
			return nil, "", err
		}
		// The name returned here is what the caller records as the source.
		// It must match what is written on the records, otherwise a later
		// Replace by that name fails to find them.
		if source != "" {
			for i := range banks {
				banks[i].Source = source
			}
			return banks, source, nil
		}
		return banks, src.Name(), nil
	}
	banks, err := ParseGeneric(country, source, data)
	if source == "" {
		source = GenericName
	}
	return banks, source, err
}
