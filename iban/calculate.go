package iban

import (
	"fmt"
	"strings"
)

// Calculate builds an IBAN from a country code, a national bank code and an
// account number.
//
// Bank code and account number are left padded with zeros to the lengths the
// registry records for the country, which is what lets a caller pass the short
// forms people actually write down.
func Calculate(countryCode, bankCode, account string) (*IBAN, error) {
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	bankCode = Normalize(bankCode)
	account = Normalize(account)

	country, ok := Lookup(countryCode)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownCountry, countryCode)
	}
	if bankCode == "" || account == "" {
		return nil, fmt.Errorf("iban: bank code and account number are required")
	}

	bankCode, err := padTo(bankCode, country.BankCode)
	if err != nil {
		return nil, fmt.Errorf("iban: bank code: %w", err)
	}

	// The account number takes whatever room the BBAN has left.
	accountLen := country.BBANLength() - len(bankCode)
	if country.Account.known() {
		accountLen = country.Account.End - country.Account.Start
	}
	if accountLen < 1 {
		return nil, fmt.Errorf("iban: no room for an account number in a %s IBAN", countryCode)
	}
	if len(account) > accountLen {
		return nil, fmt.Errorf("iban: account number is %d characters, %s allows %d",
			len(account), countryCode, accountLen)
	}
	account = strings.Repeat("0", accountLen-len(account)) + account

	bban := bankCode + account
	if len(bban) != country.BBANLength() {
		return nil, fmt.Errorf("%w: assembled %d characters, %s needs %d",
			ErrLength, len(bban), countryCode, country.BBANLength())
	}

	// Check digits are the value that makes the whole number congruent to 1
	// modulo 97, computed over the number with "00" in their place.
	check := 98 - mod97(countryCode+"00"+bban)
	candidate := fmt.Sprintf("%s%02d%s", countryCode, check, bban)

	return Parse(candidate)
}

// padTo left pads s with zeros to the width of f, if that width is known.
func padTo(s string, f Field) (string, error) {
	if !f.known() {
		return s, nil
	}
	width := f.End - f.Start
	if len(s) > width {
		return "", fmt.Errorf("%q is %d characters, expected at most %d", s, len(s), width)
	}
	return strings.Repeat("0", width-len(s)) + s, nil
}
