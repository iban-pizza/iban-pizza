package sepa

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// BaseURL is where the EPC publishes the register exports.
const BaseURL = "https://www.europeanpaymentscouncil.eu/sites/default/files/participants_export"

// URL returns the CSV export URL for a scheme.
func URL(s Scheme) string {
	return fmt.Sprintf("%s/%s/%s.csv", BaseURL, s.file(), s.file())
}

// ParseCSV reads one EPC scheme export.
//
// Columns are resolved by name rather than by position, because the schemes do
// not share a layout: the Verification of Payee export inserts Status and Role
// as columns six and seven, so a parser that counted positions would read the
// wrong fields for that one scheme.
func ParseCSV(data []byte) ([]Participant, error) {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	r.Comma = ','
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("export is empty")
		}
		return nil, fmt.Errorf("read header: %w", err)
	}

	col := make(map[string]int, len(header))
	for i, name := range header {
		col[normalise(name)] = i
	}

	bicAt, ok := col["bic"]
	if !ok {
		return nil, fmt.Errorf("no BIC column, header is %v", header)
	}
	nameAt := col["participantname"]
	countryAt := col["country"]
	cityAt := col["city"]
	readyAt, hasReady := col["readinessdate"]
	leaveAt, hasLeave := col["schemeleavingdate"]
	roleAt, hasRole := col["role"]

	var out []Participant
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		bic := NormalizeBIC(at(row, bicAt))
		if bic == "" {
			continue
		}
		p := Participant{
			BIC:     bic,
			Name:    at(row, nameAt),
			Country: at(row, countryAt),
			City:    at(row, cityAt),
		}
		if hasReady {
			p.ReadinessDate = at(row, readyAt)
		}
		if hasLeave {
			p.LeavingDate = at(row, leaveAt)
		}
		if hasRole {
			p.Role = at(row, roleAt)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no participants found")
	}
	return out, nil
}

func normalise(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s)
}

func at(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
