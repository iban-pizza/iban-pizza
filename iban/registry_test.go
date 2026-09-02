package iban

import "testing"

// TestRegistryConsistency checks every registry row against itself. It is the
// safety net for a hand maintained table: a wrong length, a malformed
// structure string, a field offset reaching past the BBAN or a bad example
// IBAN all fail here rather than in production.
func TestRegistryConsistency(t *testing.T) {
	for code, c := range registry {
		t.Run(code, func(t *testing.T) {
			if c.Code != code {
				t.Errorf("keyed as %q but Code is %q", code, c.Code)
			}
			if c.Length < MinLength || c.Length > MaxLength {
				t.Errorf("length %d outside the legal range %d..%d", c.Length, MinLength, MaxLength)
			}

			if c.BBAN != "" {
				classes, err := expandStructure(c.BBAN)
				if err != nil {
					t.Fatalf("structure %q does not parse: %v", c.BBAN, err)
				}
				if len(classes) != c.BBANLength() {
					t.Errorf("structure %q expands to %d characters but length %d implies a %d character BBAN",
						c.BBAN, len(classes), c.Length, c.BBANLength())
				}
			}

			for name, f := range map[string]Field{
				"BankCode": c.BankCode, "BranchCode": c.BranchCode, "Account": c.Account,
			} {
				if !f.known() {
					continue
				}
				if f.Start < 0 || f.End > c.BBANLength() {
					t.Errorf("%s field %v reaches outside the %d character BBAN", name, f, c.BBANLength())
				}
			}

			if c.Example == "" {
				t.Skip("no example IBAN recorded")
			}
			parsed, err := Parse(c.Example)
			if err != nil {
				t.Fatalf("example %q does not parse: %v", c.Example, err)
			}
			if err := parsed.Validate(); err != nil {
				t.Errorf("example %q fails the checksum: %v", c.Example, err)
			}
		})
	}
}

// TestExpandStructure covers the notation parser directly, including the
// malformed input it must reject.
func TestExpandStructure(t *testing.T) {
	ok := []struct {
		in   string
		want int
	}{
		{"8!n10!n", 18},
		{"4!a10!n", 14},
		{"1!a5!n5!n12!c", 23},
		{"3n", 3},
	}
	for _, tc := range ok {
		got, err := expandStructure(tc.in)
		if err != nil {
			t.Errorf("expandStructure(%q) returned %v", tc.in, err)
			continue
		}
		if len(got) != tc.want {
			t.Errorf("expandStructure(%q) expanded to %d, want %d", tc.in, len(got), tc.want)
		}
	}

	for _, bad := range []string{"", "!n", "8", "8!", "8!x", "n8"} {
		if _, err := expandStructure(bad); err == nil {
			t.Errorf("expandStructure(%q) should have failed", bad)
		}
	}
}
