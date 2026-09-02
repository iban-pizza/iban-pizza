package checkdigit

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOfficialVectors runs every account number the Bundesbank publishes as a
// test case for the implemented methods. These are the specification's own
// examples, so a failure here means the implementation is wrong rather than
// the expectation being wrong.
func TestOfficialVectors(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "vectors.tsv"))
	if err != nil {
		t.Fatalf("open vectors: %v", err)
	}
	defer f.Close()

	counted := map[string]int{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 3 {
			t.Fatalf("line %d is malformed: %q", line, text)
		}
		id, expectation, account := parts[0], parts[1], parts[2]

		got, err := Validate(id, account)
		if err != nil {
			t.Errorf("method %s account %s returned an error: %v", id, account, err)
			continue
		}
		counted[id]++

		switch expectation {
		case "valid":
			// Unchecked is acceptable only where the method itself declines to
			// calculate, such as the exempt account range in method 99.
			if got == ResultInvalid {
				t.Errorf("method %s rejected %s, which the specification lists as correct",
					id, account)
			}
		case "invalid":
			if got == ResultValid {
				t.Errorf("method %s accepted %s, which the specification lists as incorrect",
					id, account)
			}
		default:
			t.Fatalf("line %d has unknown expectation %q", line, expectation)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read vectors: %v", err)
	}

	if len(counted) == 0 {
		t.Fatal("no vectors were exercised")
	}
	t.Logf("checked %d methods against official vectors", len(counted))
}

// TestUnimplementedMethodIsNeverInvalid is the safety rule of this package.
//
// About ninety methods exist and only some are implemented. Returning invalid
// for the rest would reject valid customer accounts, so unknown identifiers
// must stay silent no matter what account number they are handed.
func TestUnimplementedMethodIsNeverInvalid(t *testing.T) {
	for _, id := range []string{"A7", "D4", "63", "76", "zz", "", "999"} {
		for _, account := range []string{"9144355668", "0000000000", "1234567890"} {
			got, err := Validate(id, account)
			if err != nil {
				t.Errorf("method %q account %s returned an error: %v", id, account, err)
			}
			if got != ResultUnchecked {
				t.Errorf("method %q account %s returned %v, want unchecked", id, account, got)
			}
		}
	}
}

// TestMethod09MakesNoStatement covers the most common identifier in the German
// data, which prescribes no calculation at all.
func TestMethod09MakesNoStatement(t *testing.T) {
	for _, account := range []string{"1", "9999999999", "0000000000"} {
		got, err := Validate("09", account)
		if err != nil {
			t.Fatalf("Validate returned %v", err)
		}
		if got != ResultUnchecked {
			t.Errorf("method 09 for %s returned %v, want unchecked", account, got)
		}
	}
}

func TestMethodsDetectASingleWrongDigit(t *testing.T) {
	// Take a known good account number per method and corrupt one digit. A
	// method that accepts the corrupted number is not checking anything.
	cases := []struct{ id, good string }{
		{"00", "9290701"},
		{"06", "94012341"},
		{"10", "12345008"},
		{"28", "19999000"},
		{"32", "9141405"},
		{"88", "2525259"},
	}
	for _, tc := range cases {
		if got, err := Validate(tc.id, tc.good); err != nil || got != ResultValid {
			t.Errorf("method %s rejected its own good vector %s (%v, %v)", tc.id, tc.good, got, err)
			continue
		}

		corrupted := corrupt(tc.good)
		got, err := Validate(tc.id, corrupted)
		if err != nil {
			t.Errorf("method %s account %s returned an error: %v", tc.id, corrupted, err)
			continue
		}
		if got == ResultValid {
			t.Errorf("method %s accepted %s, a corrupted form of %s", tc.id, corrupted, tc.good)
		}
	}
}

// corrupt increments the first digit of s, wrapping 9 to 0.
func corrupt(s string) string {
	b := []byte(s)
	if b[0] == '9' {
		b[0] = '0'
	} else {
		b[0]++
	}
	return string(b)
}

func TestValidateRejectsMalformedAccounts(t *testing.T) {
	for _, account := range []string{"", "abcdefgh", "12345678901", "12 34"} {
		if _, err := Validate("00", account); err == nil {
			t.Errorf("Validate accepted malformed account %q", account)
		}
	}
}

func TestAccountsArePaddedLeft(t *testing.T) {
	// A short account number must be treated as if left padded with zeros,
	// otherwise the weights land on the wrong digits.
	short, err := Validate("00", "9290701")
	if err != nil {
		t.Fatalf("Validate returned %v", err)
	}
	padded, err := Validate("00", "0009290701")
	if err != nil {
		t.Fatalf("Validate returned %v", err)
	}
	if short != padded {
		t.Errorf("padded and unpadded forms disagree: %v and %v", short, padded)
	}
}

func TestImplementedReporting(t *testing.T) {
	if !Implemented("00") || !Implemented(" 06 ") {
		t.Error("Implemented does not recognise a known method")
	}
	if Implemented("A7") {
		t.Error("Implemented claims an unimplemented method")
	}
	if len(ImplementedMethods()) == 0 {
		t.Error("ImplementedMethods is empty")
	}
}

func TestResultString(t *testing.T) {
	for _, tc := range []struct {
		r    Result
		want string
	}{
		{ResultValid, "valid"},
		{ResultInvalid, "invalid"},
		{ResultUnchecked, "unchecked"},
	} {
		if got := tc.r.String(); got != tc.want {
			t.Errorf("Result(%d).String() = %q, want %q", tc.r, got, tc.want)
		}
	}
}

// TestUnverifiedMethodsStaySilent covers the gate on methods that have an
// implementation but no official test vectors. They must never produce a
// verdict, because nothing proves the implementation right.
func TestUnverifiedMethodsStaySilent(t *testing.T) {
	for _, id := range []string{"01", "02", "13"} {
		if !Implemented(id) {
			t.Errorf("method %s should have an implementation", id)
		}
		if Verified(id) {
			t.Errorf("method %s is marked verified but has no official vectors", id)
		}
		for _, account := range []string{"1234567890", "0000000001", "9999999999"} {
			got, err := Validate(id, account)
			if err != nil {
				t.Errorf("method %s account %s returned an error: %v", id, account, err)
			}
			if got != ResultUnchecked {
				t.Errorf("method %s account %s returned %v, want unchecked", id, account, got)
			}
		}
	}
}

// TestVerifiedMethodsHaveVectors keeps the gate honest in the other direction:
// nothing may be marked verified unless the vector file actually exercises it.
func TestVerifiedMethodsHaveVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors.tsv"))
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	withVectors := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		withVectors[strings.SplitN(line, "\t", 2)[0]] = true
	}

	for id := range verified {
		// Method 09 prescribes no calculation, so it needs no vectors.
		if id == "09" {
			continue
		}
		if !withVectors[id] {
			t.Errorf("method %s is marked verified but no vector exercises it", id)
		}
	}
}

// TestC1MatchesIndependentImplementation checks the ING-DiBa account number
// from a third party IBAN checker. Method C1 is ING's, and an independent tool
// reports this account as having a wrong check digit.
func TestC1MatchesIndependentImplementation(t *testing.T) {
	got, err := Validate("C1", "9144355668")
	if err != nil {
		t.Fatalf("Validate returned %v", err)
	}
	if got != ResultInvalid {
		t.Errorf("C1 for 9144355668 returned %v, want invalid", got)
	}

	// The same account with a corrected check digit must pass, which proves the
	// method discriminates rather than rejecting everything.
	//
	// This account starts with a 9, so C1 selects variant 1, which is method 17
	// and reads the check digit at position 8. Varying the last digit would
	// change nothing, because positions 9 and 10 are the sub account number.
	var accepted int
	for d := range 10 {
		candidate := []byte("9144355668")
		candidate[7] = byte('0' + d)
		if r, _ := Validate("C1", string(candidate)); r == ResultValid {
			accepted++
		}
	}
	if accepted != 1 {
		t.Errorf("%d of 10 check digits were accepted at position 8, want exactly 1", accepted)
	}
}

// TestC1SelectsVariantByFirstDigit covers the branch in C1, which uses one
// algorithm for accounts starting with a five and another for the rest.
func TestC1SelectsVariantByFirstDigit(t *testing.T) {
	// Variant 2 vectors start with a five, variant 1 vectors do not. Each must
	// be accepted, which only happens if the selection is right.
	for _, tc := range []struct{ account, variant string }{
		{"5432112349", "2"},
		{"0446786040", "1"},
	} {
		got, err := Validate("C1", tc.account)
		if err != nil {
			t.Errorf("account %s returned an error: %v", tc.account, err)
			continue
		}
		if got != ResultValid {
			t.Errorf("variant %s vector %s returned %v, want valid", tc.variant, tc.account, got)
		}
	}
}
