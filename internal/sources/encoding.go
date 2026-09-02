package sources

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// DecodeLatin1 converts ISO-8859-1 bytes to UTF-8.
//
// Several official registries publish Latin-1 and say so nowhere. Reading
// those bytes as if they were UTF-8 corrupts every umlaut, and in a fixed
// width file it also shifts every column after one, which silently moves BICs
// and check digit methods onto the wrong institution. The conversion is
// direct: in Latin-1 each byte is its own Unicode code point.
func DecodeLatin1(b []byte) string {
	// Fast path: pure ASCII needs no conversion.
	ascii := true
	for _, c := range b {
		if c >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return string(b)
	}

	var sb strings.Builder
	sb.Grow(len(b) * 2)
	for _, c := range b {
		sb.WriteRune(rune(c))
	}
	return sb.String()
}

// DecodeAuto returns the text of b, converting from Latin-1 when b is not
// already valid UTF-8. Sources have changed encoding before, so guessing wrong
// in either direction has to be impossible rather than unlikely.
func DecodeAuto(b []byte) string {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if utf8.Valid(b) {
		return string(b)
	}
	return DecodeLatin1(b)
}

// SplitLines splits on LF and tolerates CRLF, which the German and Austrian
// files both use.
func SplitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	// Drop a single trailing empty line produced by a final newline.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// runeSlice cuts a fixed width field by rune position rather than byte
// position. The German file is fixed width in characters, so slicing raw bytes
// after a multi byte character would take the wrong columns.
func runeSlice(r []rune, start, end int) string {
	if start >= len(r) {
		return ""
	}
	end = min(end, len(r))
	return strings.TrimSpace(string(r[start:end]))
}
