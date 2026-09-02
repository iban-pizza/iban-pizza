// Package iban parses and validates International Bank Account Numbers.
//
// Validation is driven by the country registry in registry_data.go, so a
// country's BBAN structure is checked rather than only its total length.
package iban

import (
	"errors"
	"fmt"
	"strings"
)

// MaxLength is the longest IBAN permitted by ISO 13616. Input longer than this
// is rejected before any allocation, which keeps untrusted request parameters
// from turning into work.
const MaxLength = 34

// MinLength is the shortest IBAN in the registry (Norway, 15).
const MinLength = 15

var (
	ErrTooShort       = errors.New("iban: too short")
	ErrTooLong        = errors.New("iban: too long")
	ErrCountryCode    = errors.New("iban: invalid country code")
	ErrCheckDigits    = errors.New("iban: invalid check digits")
	ErrUnknownCountry = errors.New("iban: unknown country")
	ErrLength         = errors.New("iban: wrong length for country")
	ErrStructure      = errors.New("iban: BBAN does not match country structure")
	ErrChecksum       = errors.New("iban: checksum failed")
)

// IBAN is a syntactically well formed IBAN. Obtaining one via Parse guarantees
// the country is known, the length matches the registry and the BBAN structure
// fits; it does NOT guarantee the checksum, which Validate reports separately
// so callers can tell "malformed" from "mistyped".
type IBAN struct {
	raw     string // normalised: upper case, no separators
	country Country
}

// Normalize strips ASCII spaces and hyphens and upper-cases the input. It is
// the only place input formatting is tolerated.
func Normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '-':
			continue
		case c >= 'a' && c <= 'z':
			b.WriteByte(c - 'a' + 'A')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Parse normalises and structurally validates an IBAN. The checksum is not
// verified here; call Validate for that.
func Parse(s string) (*IBAN, error) {
	s = Normalize(s)

	if len(s) < 4 {
		return nil, ErrTooShort
	}
	if len(s) > MaxLength {
		return nil, ErrTooLong
	}

	cc := s[0:2]
	if !isUpper(cc[0]) || !isUpper(cc[1]) {
		return nil, ErrCountryCode
	}
	if !isDigit(s[2]) || !isDigit(s[3]) {
		return nil, ErrCheckDigits
	}

	country, ok := Lookup(cc)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCountry, cc)
	}
	if len(s) != country.Length {
		return nil, fmt.Errorf("%w: %s expects %d characters, got %d",
			ErrLength, cc, country.Length, len(s))
	}
	if err := checkStructure(country, s[4:]); err != nil {
		return nil, err
	}

	return &IBAN{raw: s, country: country}, nil
}

// checkStructure verifies the BBAN against the country's registry structure.
// For length-only rows it falls back to an alphanumeric check so that the
// result never claims more than the data supports.
func checkStructure(c Country, bban string) error {
	if c.BBAN == "" {
		for i := range len(bban) {
			if !classAlnum.accepts(bban[i]) {
				return fmt.Errorf("%w: position %d is not alphanumeric", ErrStructure, i+5)
			}
		}
		return nil
	}

	classes, err := expandStructure(c.BBAN)
	if err != nil {
		// A malformed table entry is a programming error, not user input.
		return fmt.Errorf("iban: registry entry for %s is invalid: %w", c.Code, err)
	}
	if len(classes) != len(bban) {
		return fmt.Errorf("%w: %s expects a %d character BBAN, got %d",
			ErrStructure, c.Code, len(classes), len(bban))
	}
	for i := range len(bban) {
		if !classes[i].accepts(bban[i]) {
			return fmt.Errorf("%w: position %d of %s must be %s",
				ErrStructure, i+5, c.Code, classes[i].describe())
		}
	}
	return nil
}

// Validate reports whether the mod-97 checksum holds.
func (i *IBAN) Validate() error {
	if mod97(i.raw) != 1 {
		return ErrChecksum
	}
	return nil
}

// String returns the compact machine form, e.g. "DE89370400440532013000".
func (i *IBAN) String() string { return i.raw }

// Formatted returns the human readable form in groups of four,
// e.g. "DE89 3704 0044 0532 0130 00".
func (i *IBAN) Formatted() string {
	var b strings.Builder
	b.Grow(len(i.raw) + len(i.raw)/4)
	for n := 0; n < len(i.raw); n += 4 {
		if n > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(i.raw[n:min(n+4, len(i.raw))])
	}
	return b.String()
}

func (i *IBAN) CountryCode() string { return i.raw[0:2] }
func (i *IBAN) CheckDigits() string { return i.raw[2:4] }
func (i *IBAN) BBAN() string        { return i.raw[4:] }
func (i *IBAN) Country() Country    { return i.country }

// BankCode returns the national bank identifier, or "" when the registry does
// not record where it sits for this country.
func (i *IBAN) BankCode() string { return i.country.BankCode.extract(i.BBAN()) }

// BranchCode returns the branch identifier, or "" when the country has none.
func (i *IBAN) BranchCode() string { return i.country.BranchCode.extract(i.BBAN()) }

// AccountNumber returns the account number portion, or "" when unknown.
func (i *IBAN) AccountNumber() string { return i.country.Account.extract(i.BBAN()) }

// mod97 computes the ISO 7064 MOD-97-10 remainder of an IBAN.
//
// The first four characters are rotated to the end, then each character is
// expanded to its numeric value (A=10 ... Z=35) and the remainder is
// accumulated digit by digit. Doing it incrementally avoids the big.Int
// allocation the original implementation needed for a value that never has to
// exist in full.
func mod97(s string) uint32 {
	var r uint32
	for n := range len(s) {
		c := s[(n+4)%len(s)]
		switch {
		case c >= '0' && c <= '9':
			r = r*10 + uint32(c-'0')
		case c >= 'A' && c <= 'Z':
			r = r*100 + uint32(c-'A') + 10
		default:
			return 0
		}
		r %= 97
	}
	return r
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
