package sources

import (
	"html"
	"regexp"
	"strings"
)

// Two registries publish their list only as a table on a web page. The
// standard library has no HTML parser, and these tables need none: rows and
// cells are found by their tags and everything else is dropped.

var (
	htmlTag     = regexp.MustCompile(`(?is)<(/?)(tr|td|th)\b[^>]*>`)
	htmlAnyTag  = regexp.MustCompile(`(?s)<[^>]*>`)
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->|<script\b.*?</script>|<style\b.*?</style>`)
	htmlSpace   = regexp.MustCompile(`[\s\x{00a0}]+`)
)

// htmlRows returns every table row on a page as its cell texts, in document
// order. Tables are not distinguished: callers pick their rows by content,
// which survives a page being restyled where positions would not.
//
// A row that opens while another is still open, as happens with nested
// tables, closes the outer one, so the inner rows come out intact.
func htmlRows(page string) [][]string {
	page = htmlComment.ReplaceAllString(page, "")

	var (
		rows    [][]string
		row     []string
		inRow   bool
		inCell  bool
		cellAt  int
		flushTo = func(end int) {
			if inCell {
				row = append(row, htmlText(page[cellAt:end]))
				inCell = false
			}
		}
		endRow = func(end int) {
			flushTo(end)
			// A row whose only content was a nested table has nothing left
			// once that table's rows are taken out.
			if inRow && strings.Join(row, "") != "" {
				rows = append(rows, row)
			}
			row, inRow = nil, false
		}
	)

	for _, m := range htmlTag.FindAllStringSubmatchIndex(page, -1) {
		closing := page[m[2]:m[3]] == "/"
		name := strings.ToLower(page[m[4]:m[5]])
		switch {
		case name == "tr" && !closing:
			endRow(m[0])
			inRow = true
		case name == "tr" && closing:
			endRow(m[0])
		case !closing: // td or th
			flushTo(m[0])
			if inRow {
				inCell, cellAt = true, m[1]
			}
		default:
			flushTo(m[0])
		}
	}
	endRow(len(page))
	return rows
}

// htmlText reduces a fragment to its visible text.
func htmlText(fragment string) string {
	s := htmlAnyTag.ReplaceAllString(fragment, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(htmlSpace.ReplaceAllString(s, " "))
}
