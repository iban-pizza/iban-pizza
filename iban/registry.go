package iban

import (
	"fmt"
	"strings"
)

// charClass mirrors the character classes used by the SWIFT IBAN Registry
// structure notation.
//
//	n  digits (0-9)
//	a  upper case letters (A-Z)
//	c  alphanumeric (0-9, A-Z)
type charClass byte

const (
	classDigit charClass = 'n'
	classUpper charClass = 'a'
	classAlnum charClass = 'c'
)

func (c charClass) accepts(ch byte) bool {
	switch c {
	case classDigit:
		return ch >= '0' && ch <= '9'
	case classUpper:
		return ch >= 'A' && ch <= 'Z'
	case classAlnum:
		return (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z')
	}
	return false
}

func (c charClass) describe() string {
	switch c {
	case classDigit:
		return "a digit"
	case classUpper:
		return "an upper case letter"
	case classAlnum:
		return "an alphanumeric character"
	}
	return "an unknown character"
}

// Field marks a half-open range [Start,End) inside the BBAN.
type Field struct{ Start, End int }

func (f Field) known() bool { return f.End > f.Start }

// extract returns the substring of bban covered by f, or "" if f is unset or
// reaches past the end of bban.
func (f Field) extract(bban string) string {
	if !f.known() || f.End > len(bban) {
		return ""
	}
	return bban[f.Start:f.End]
}

// Country describes one entry of the IBAN Registry.
//
// Length is the total IBAN length including country code and check digits.
// BBAN holds the structure of the part after the check digits in SWIFT
// notation; an empty BBAN means only the length is known, in which case
// validation degrades to a length and alphabet check rather than reporting a
// structure it cannot verify.
type Country struct {
	Code, Name string
	Length     int
	BBAN       string
	BankCode   Field
	BranchCode Field
	Account    Field

	// Example is a real IBAN for this country, used by
	// TestRegistryConsistency to prove the row describes reality.
	Example string
}

// BBANLength is the expected length of the BBAN for this country.
func (c Country) BBANLength() int { return c.Length - 4 }

// expandStructure turns SWIFT notation such as "8!n10!n" into one charClass per
// BBAN character. It returns an error for malformed notation so that a typo in
// the table below fails loudly in tests rather than silently accepting input.
func expandStructure(pattern string) ([]charClass, error) {
	var out []charClass
	for i := 0; i < len(pattern); {
		start := i
		for i < len(pattern) && pattern[i] >= '0' && pattern[i] <= '9' {
			i++
		}
		if start == i {
			return nil, fmt.Errorf("expected a length at offset %d of %q", i, pattern)
		}
		n := 0
		for _, d := range pattern[start:i] {
			n = n*10 + int(d-'0')
		}
		// "!" marks a fixed length. Every entry in this table is fixed length;
		// the marker is accepted and ignored so the notation can be copied from
		// the registry verbatim.
		if i < len(pattern) && pattern[i] == '!' {
			i++
		}
		if i >= len(pattern) {
			return nil, fmt.Errorf("missing character class at end of %q", pattern)
		}
		cl := charClass(pattern[i])
		switch cl {
		case classDigit, classUpper, classAlnum:
		default:
			return nil, fmt.Errorf("unknown character class %q in %q", pattern[i], pattern)
		}
		i++
		for range n {
			out = append(out, cl)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty structure %q", pattern)
	}
	return out, nil
}

// Lookup returns the registry entry for a two letter country code.
func Lookup(code string) (Country, bool) {
	c, ok := registry[strings.ToUpper(code)]
	return c, ok
}

// Countries lists every country code known to the registry.
func Countries() []string {
	out := make([]string, 0, len(registry))
	for code := range registry {
		out = append(out, code)
	}
	return out
}
