package logo

import (
	"fmt"
	"hash/fnv"
	"strings"
	"unicode"
)

// palette holds background colours chosen to keep white text readable on all
// of them, so the generated mark works in a light and a dark interface alike.
var palette = []string{
	"#1f4e79", "#2e6b4f", "#7a3b2e", "#4a3b73", "#8a5a1a",
	"#1f5f6b", "#6b2f4a", "#3d5a1f", "#54407a", "#7a2f2f",
	"#25566b", "#5a4a1f",
}

// Monogram is a generated placeholder mark.
type Monogram struct {
	Initials   string
	Background string
	Foreground string
}

// NewMonogram derives a monogram from a bank name and BIC.
//
// The colour comes from the BIC so that the same institution always renders
// identically, including across restarts and across instances.
func NewMonogram(name, bic string) Monogram {
	return Monogram{
		Initials:   initials(name),
		Background: pickColour(bic + name),
		Foreground: "#ffffff",
	}
}

// initials takes up to two letters representing the name.
func initials(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	// Skip legal form suffixes, which carry no identity and would produce
	// monograms that all read the same.
	skip := map[string]bool{
		"ag": true, "eg": true, "gmbh": true, "kg": true, "kgaa": true,
		"mbh": true, "co": true, "se": true, "sa": true, "nv": true,
		"as": true, "plc": true, "ltd": true, "und": true, "der": true,
		"die": true, "das": true, "van": true, "de": true,
	}

	var picked []rune
	for _, f := range fields {
		if skip[strings.ToLower(f)] {
			continue
		}
		r := []rune(f)
		picked = append(picked, unicode.ToUpper(r[0]))
		if len(picked) == 2 {
			break
		}
	}

	switch len(picked) {
	case 0:
		return "?"
	case 1:
		// A single word name gives its first two letters more character than
		// one letter alone.
		for _, f := range fields {
			r := []rune(f)
			if len(r) >= 2 && !skip[strings.ToLower(f)] {
				return string([]rune{unicode.ToUpper(r[0]), unicode.ToLower(r[1])})
			}
		}
		return string(picked)
	default:
		return string(picked)
	}
}

// pickColour maps a seed onto the palette deterministically.
func pickColour(seed string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToUpper(seed)))
	return palette[int(h.Sum32())%len(palette)]
}

// SVG renders the monogram.
//
// The initials are escaped rather than interpolated raw. A bank name is
// external data, and an unescaped "&" or "<" would produce a broken or
// injectable document.
func (m Monogram) SVG(size int) string {
	if size <= 0 {
		size = 128
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128" width="%d" height="%d" role="img" aria-label="%s">`+
		`<rect width="128" height="128" rx="24" fill="%s"/>`+
		`<text x="64" y="64" fill="%s" font-family="system-ui,-apple-system,Segoe UI,Roboto,sans-serif" `+
		`font-size="52" font-weight="600" text-anchor="middle" dominant-baseline="central">%s</text>`+
		`</svg>`,
		size, size, escapeXML(m.Initials), m.Background, m.Foreground, escapeXML(m.Initials))
}

// escapeXML escapes the five XML predefined entities.
func escapeXML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	).Replace(s)
}
