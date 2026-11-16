package sources

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

// Several registries publish only a spreadsheet. An XLSX file is a zip archive
// of XML documents, and reading cell text out of one needs nothing beyond the
// standard library, so this is a small reader rather than a dependency. It
// reads text and numbers as they are stored and ignores styles, formulas and
// everything else a registry list does not use.

// maxSheetBytes bounds what is decompressed from one archive member, so that a
// crafted file cannot expand into unbounded memory.
const maxSheetBytes = 128 << 20

// readXLSX returns the rows of one worksheet as text. An empty sheet name
// selects the first sheet. Rows are padded so that a cell's index is its
// column, which keeps columns aligned when a row has gaps.
func readXLSX(data []byte, sheet string) ([][]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[strings.TrimPrefix(f.Name, "/")] = f
	}

	sheetPath, err := xlsxSheetPath(files, sheet)
	if err != nil {
		return nil, err
	}
	shared, err := xlsxSharedStrings(files)
	if err != nil {
		return nil, err
	}

	raw, err := xlsxRead(files, sheetPath)
	if err != nil {
		return nil, err
	}

	var doc struct {
		Rows []struct {
			Cells []struct {
				Ref    string `xml:"r,attr"`
				Type   string `xml:"t,attr"`
				Value  string `xml:"v"`
				Inline struct {
					Text string    `xml:"t"`
					Runs []xlsxRun `xml:"r"`
				} `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", sheetPath, err)
	}

	rows := make([][]string, 0, len(doc.Rows))
	for _, r := range doc.Rows {
		var row []string
		for i, c := range r.Cells {
			col := xlsxColumn(c.Ref)
			if col < 0 {
				col = i
			}
			var text string
			switch c.Type {
			case "s":
				var n int
				if _, err := fmt.Sscanf(c.Value, "%d", &n); err == nil && n >= 0 && n < len(shared) {
					text = shared[n]
				}
			case "inlineStr":
				text = c.Inline.Text
				for _, run := range c.Inline.Runs {
					text += run.Text
				}
			default:
				text = c.Value
			}
			for len(row) <= col {
				row = append(row, "")
			}
			row[col] = strings.TrimSpace(text)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

type xlsxRun struct {
	Text string `xml:"t"`
}

// xlsxSheetPath resolves a sheet name to its archive member through the
// workbook and its relationships, since sheet files are not reliably numbered
// in display order.
func xlsxSheetPath(files map[string]*zip.File, name string) (string, error) {
	raw, err := xlsxRead(files, "xl/workbook.xml")
	if err != nil {
		return "", err
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err := xml.Unmarshal(raw, &wb); err != nil {
		return "", fmt.Errorf("parse workbook: %w", err)
	}
	if len(wb.Sheets) == 0 {
		return "", fmt.Errorf("workbook has no sheets")
	}

	rid := wb.Sheets[0].RID
	if name != "" {
		rid = ""
		var have []string
		for _, s := range wb.Sheets {
			have = append(have, s.Name)
			if strings.EqualFold(strings.TrimSpace(s.Name), strings.TrimSpace(name)) {
				rid = s.RID
			}
		}
		if rid == "" {
			return "", fmt.Errorf("no sheet named %q, workbook has %q", name, have)
		}
	}

	raw, err = xlsxRead(files, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return "", err
	}
	var rels struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(raw, &rels); err != nil {
		return "", fmt.Errorf("parse workbook relationships: %w", err)
	}
	for _, r := range rels.Rel {
		if r.ID == rid {
			if strings.HasPrefix(r.Target, "/") {
				return strings.TrimPrefix(r.Target, "/"), nil
			}
			return path.Join("xl", r.Target), nil
		}
	}
	return "", fmt.Errorf("sheet relationship %q not found", rid)
}

// xlsxSharedStrings reads the string table. Text cells store an index into it
// rather than their own text. A workbook without text has no such table.
func xlsxSharedStrings(files map[string]*zip.File) ([]string, error) {
	if _, ok := files["xl/sharedStrings.xml"]; !ok {
		return nil, nil
	}
	raw, err := xlsxRead(files, "xl/sharedStrings.xml")
	if err != nil {
		return nil, err
	}
	var sst struct {
		Items []struct {
			Text string    `xml:"t"`
			Runs []xlsxRun `xml:"r"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(raw, &sst); err != nil {
		return nil, fmt.Errorf("parse shared strings: %w", err)
	}
	out := make([]string, len(sst.Items))
	for i, it := range sst.Items {
		// A cell with mixed formatting is stored as several runs; the text is
		// their concatenation.
		text := it.Text
		for _, run := range it.Runs {
			text += run.Text
		}
		out[i] = text
	}
	return out, nil
}

func xlsxRead(files map[string]*zip.File, name string) ([]byte, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("xlsx has no %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(io.LimitReader(rc, maxSheetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	if len(raw) > maxSheetBytes {
		return nil, fmt.Errorf("%s exceeds the %d byte limit", name, maxSheetBytes)
	}
	return raw, nil
}

// xlsxColumn converts a cell reference such as "C7" or "AB12" to a zero based
// column index, or -1 when the reference is missing.
func xlsxColumn(ref string) int {
	col := 0
	seen := false
	for _, r := range ref {
		if r < 'A' || r > 'Z' {
			break
		}
		col = col*26 + int(r-'A') + 1
		seen = true
	}
	if !seen {
		return -1
	}
	return col - 1
}
