package sources

import (
	"encoding/csv"
	"fmt"
	"strings"
)

// csvTable is a parsed delimited file with named columns.
type csvTable struct {
	columns map[string]int
	rows    [][]string
}

// parseCSV reads delimited text whose header row is located by looking for a
// line containing every name in mustContain.
//
// Two of the registries put legal notices and a query timestamp above the
// header, and those preamble lines change. Searching for the header by content
// survives that, where counting lines to skip would not.
func parseCSV(text string, sep rune, mustContain ...string) (*csvTable, error) {
	lines := SplitLines(text)

	headerAt := -1
	for i, line := range lines {
		lower := strings.ToLower(line)
		ok := true
		for _, want := range mustContain {
			if !strings.Contains(lower, strings.ToLower(want)) {
				ok = false
				break
			}
		}
		if ok {
			headerAt = i
			break
		}
	}
	if headerAt < 0 {
		return nil, fmt.Errorf("no header row containing %v", mustContain)
	}

	r := csv.NewReader(strings.NewReader(strings.Join(lines[headerAt:], "\n")))
	r.Comma = sep
	r.LazyQuotes = true
	// Registries are not always rectangular; a short trailing row must not
	// abort the whole import.
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("no data rows below the header")
	}

	columns := make(map[string]int, len(records[0]))
	for i, name := range records[0] {
		columns[normaliseHeader(name)] = i
	}
	return &csvTable{columns: columns, rows: records[1:]}, nil
}

// normaliseHeader lowercases and strips spaces so that "SWIFT-Code" and
// "swift code" resolve to the same column.
func normaliseHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "").Replace(s)
	return s
}

// index returns the position of the first header that matches any of names.
func (t *csvTable) index(names ...string) (int, bool) {
	for _, n := range names {
		if i, ok := t.columns[normaliseHeader(n)]; ok {
			return i, true
		}
	}
	return 0, false
}

// mustIndex is index with an error naming the columns that were available,
// which turns a silent column rename upstream into an actionable message.
func (t *csvTable) mustIndex(names ...string) (int, error) {
	if i, ok := t.index(names...); ok {
		return i, nil
	}
	have := make([]string, 0, len(t.columns))
	for name := range t.columns {
		have = append(have, name)
	}
	return 0, fmt.Errorf("none of the columns %v found, file has %v", names, have)
}

// field returns the trimmed value at column i, or "" when the row is short.
func field(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
