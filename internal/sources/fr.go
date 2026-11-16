package sources

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(ACPR{}) }

// acprURL is the REGAFI export of every entity the French supervisor
// registers. The dataset name is stable; the export is built on request.
const acprURL = "https://www.regafi.fr/api/explore/v2.1/catalog/datasets/prd-banque-entites/exports/csv?delimiter=%3B"

// ACPR reads the French register of financial agents (REGAFI).
//
// The register is far wider than the banks: most of its rows are agents and
// intermediaries, many of them natural persons. Only an institution that was
// given an interbank code (CIB) can appear in an IBAN, so the CIB is both the
// bank code and the filter.
type ACPR struct{}

func (ACPR) Country() string { return "FR" }
func (ACPR) Name() string    { return "ACPR (REGAFI)" }

func (ACPR) Terms() Terms {
	return Terms{
		Licence:     "No licence stated (ACPR open data portal)",
		URL:         "https://www.regafi.fr/terms/terms-and-conditions/",
		Attribution: "Source: ACPR, REGAFI",
		Note:        "The portal defines no terms of reuse, so no right beyond consulting the register is granted in writing.",
	}
}

func (a ACPR) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, acprURL)
	return data, acprURL, err
}

func (a ACPR) Parse(data []byte) ([]bankdata.Bank, error) {
	// The event column holds free text with line breaks inside quotes, so the
	// file has to be read as CSV and never line by line.
	table, err := parseCSV(DecodeAuto(data), ';', "cib", "denomination")
	if err != nil {
		return nil, err
	}
	cibAt, err := table.mustIndex("cib")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("denomination")
	if err != nil {
		return nil, err
	}
	leiAt, hasLEI := table.index("lei")
	zipAt, hasZip := table.index("code_postal")
	cityAt, hasCity := table.index("ville")

	var out []bankdata.Bank
	seen := map[string]bool{}
	for _, row := range table.rows {
		// Rows without a code are skipped before anything else is read from
		// them: they include private individuals acting as agents, who must
		// never end up in the output.
		codes := acprCodes(field(row, cibAt))
		name := field(row, nameAt)
		if len(codes) == 0 || name == "" {
			continue
		}

		var lei string
		if hasLEI {
			if lei = strings.ToUpper(field(row, leiAt)); len(lei) != 20 {
				lei = ""
			}
		}
		bank := bankdata.Bank{
			Country: "FR",
			Name:    name,
			LEI:     lei,
			Source:  a.Name(),
		}
		if hasZip {
			bank.Zip = field(row, zipAt)
		}
		if hasCity {
			bank.City = field(row, cityAt)
		}

		// An institution keeps the codes of the banks it absorbed, and each
		// of them still appears in IBANs, so every code gets its own record.
		for _, code := range codes {
			if seen[code] {
				continue
			}
			seen[code] = true
			bank.BankCode = code
			out = append(out, bank)
		}
	}
	return out, nil
}

// acprCodes reads the CIB cell, a JSON list such as
// [{"code": "12345", "date": "2001-01-01"}], and returns the codes in it. An
// empty list, which is what most rows carry, yields nothing.
func acprCodes(cell string) []string {
	if cell == "" || cell == "[]" {
		return nil
	}
	var entries []struct {
		Code json.RawMessage `json:"code"`
	}
	if err := json.Unmarshal([]byte(cell), &entries); err != nil {
		return nil
	}
	var codes []string
	for _, e := range entries {
		// The code is published as text, but a number would lose its leading
		// zeros, so both forms are read and padded back to five digits.
		code := strings.Trim(strings.TrimSpace(string(e.Code)), `"`)
		if code == "" || len(code) > 5 || strings.Trim(code, "0123456789") != "" {
			continue
		}
		codes = append(codes, strings.Repeat("0", 5-len(code))+code)
	}
	return codes
}
