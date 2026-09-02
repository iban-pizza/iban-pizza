package iban

import (
	"errors"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	for _, in := range []string{
		"DE89370400440532013000",
		"de89370400440532013000",      // lower case
		"DE89 3704 0044 0532 0130 00", // grouped
		"DE89-3704-0044-0532-0130-00", // hyphenated
		"NL91ABNA0417164300",
		"CH9300762011623852957",
	} {
		got, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q) returned %v", in, err)
			continue
		}
		if err := got.Validate(); err != nil {
			t.Errorf("Validate(%q) returned %v", in, err)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		in   string
		want error
	}{
		{"", ErrTooShort},
		{"DE", ErrTooShort},
		{strings.Repeat("D", 35), ErrTooLong},
		{"D189370400440532013000", ErrCountryCode},
		{"DEX9370400440532013000", ErrCheckDigits},
		{"ZZ89370400440532013000", ErrUnknownCountry},
		{"DE8937040044053201300", ErrLength},   // one short
		{"DE893704004405320130000", ErrLength}, // one long
		// Correct length for DE, but letters where the structure demands digits.
		// Upstream accepted this because it only compared lengths.
		{"DE89AB0400440532013000", ErrStructure},
		// NL wants four letters as the bank code; digits must be refused.
		{"NL9112340417164300", ErrStructure},
	}
	for _, tc := range cases {
		_, err := Parse(tc.in)
		if !errors.Is(err, tc.want) {
			t.Errorf("Parse(%q) returned %v, want %v", tc.in, err, tc.want)
		}
	}
}

func TestValidateChecksum(t *testing.T) {
	// Structurally perfect, checksum wrong: Parse must succeed so the caller can
	// tell "malformed" from "mistyped", and Validate must reject.
	i, err := Parse("DE88370400440532013000")
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	if err := i.Validate(); !errors.Is(err, ErrChecksum) {
		t.Errorf("Validate returned %v, want ErrChecksum", err)
	}
}

// TestParseFormatVerbs feeds the format verbs that broke the upstream service,
// which passed request data straight into fmt.Fprintf. Nothing here may panic.
func TestParseFormatVerbs(t *testing.T) {
	for _, in := range []string{"%s", "%d", "%!v(PANIC=", "%%%%", "DE89%s0400440532013000"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) should have failed", in)
		}
	}
}

func TestComponents(t *testing.T) {
	i, err := Parse("DE49500105179144355668")
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	for _, tc := range []struct{ name, got, want string }{
		{"CountryCode", i.CountryCode(), "DE"},
		{"CheckDigits", i.CheckDigits(), "49"},
		{"BankCode", i.BankCode(), "50010517"},
		{"AccountNumber", i.AccountNumber(), "9144355668"},
		{"Formatted", i.Formatted(), "DE49 5001 0517 9144 3556 68"},
		{"String", i.String(), "DE49500105179144355668"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	// Germany has no branch code; the accessor must say so rather than guess.
	if got := i.BranchCode(); got != "" {
		t.Errorf("BranchCode = %q, want empty for DE", got)
	}
}

func BenchmarkParseAndValidate(b *testing.B) {
	for b.Loop() {
		i, err := Parse("DE89370400440532013000")
		if err != nil {
			b.Fatal(err)
		}
		_ = i.Validate()
	}
}

func TestCalculate(t *testing.T) {
	// Bank code and account number of a known IBAN must reassemble into it.
	got, err := Calculate("DE", "37040044", "0532013000")
	if err != nil {
		t.Fatalf("Calculate returned %v", err)
	}
	if got.String() != "DE89370400440532013000" {
		t.Errorf("Calculate = %q, want DE89370400440532013000", got.String())
	}
	if err := got.Validate(); err != nil {
		t.Errorf("calculated IBAN fails validation: %v", err)
	}

	// Short forms must be padded rather than rejected.
	padded, err := Calculate("DE", "37040044", "532013000")
	if err != nil {
		t.Fatalf("Calculate with a short account returned %v", err)
	}
	if padded.String() != got.String() {
		t.Errorf("short form produced %q, want %q", padded.String(), got.String())
	}
}

func TestCalculateRejects(t *testing.T) {
	cases := []struct{ cc, bank, account string }{
		{"ZZ", "12345678", "1234567890"},  // unknown country
		{"DE", "", "1234567890"},          // missing bank code
		{"DE", "37040044", ""},            // missing account
		{"DE", "370400441", "1234"},       // bank code too long
		{"DE", "37040044", "12345678901"}, // account too long
	}
	for _, tc := range cases {
		if _, err := Calculate(tc.cc, tc.bank, tc.account); err == nil {
			t.Errorf("Calculate(%q, %q, %q) should have failed", tc.cc, tc.bank, tc.account)
		}
	}
}

// TestCalculateRoundTripsEveryRegistryExample proves the check digit
// computation against every country in the registry rather than one of them.
func TestCalculateRoundTripsEveryRegistryExample(t *testing.T) {
	for code, c := range registry {
		if c.Example == "" || !c.BankCode.known() || !c.Account.known() {
			continue
		}
		parsed, err := Parse(c.Example)
		if err != nil {
			t.Errorf("%s: example does not parse: %v", code, err)
			continue
		}
		// Only countries whose bank code and account number together make up
		// the whole BBAN can be reassembled from those two parts.
		if c.BankCode.End != c.Account.Start || c.Account.End != c.BBANLength() {
			continue
		}
		got, err := Calculate(code, parsed.BankCode(), parsed.AccountNumber())
		if err != nil {
			t.Errorf("%s: Calculate returned %v", code, err)
			continue
		}
		if got.String() != parsed.String() {
			t.Errorf("%s: round trip produced %q, want %q", code, got.String(), parsed.String())
		}
	}
}
