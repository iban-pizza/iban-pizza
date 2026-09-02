// Package checkdigit verifies German account number check digits.
//
// The Bundesbank publishes a method identifier for every bank code and
// specifies about ninety different algorithms. The upstream project read that
// identifier, stored it, and never checked anything with it.
//
// The safety rule here matters more than coverage: a method that is not
// implemented returns ResultUnchecked, never ResultInvalid. Reporting a valid
// customer account as invalid is far worse than reporting that it was not
// checked, so unimplemented methods stay silent.
package checkdigit

import (
	"fmt"
	"strings"
)

// Result is the outcome of a check.
type Result int

const (
	// ResultUnchecked means no statement was made: the method is not
	// implemented, or the method itself prescribes no calculation.
	ResultUnchecked Result = iota

	// ResultValid means the check digit matches.
	ResultValid

	// ResultInvalid means the check digit does not match.
	ResultInvalid
)

func (r Result) String() string {
	switch r {
	case ResultValid:
		return "valid"
	case ResultInvalid:
		return "invalid"
	default:
		return "unchecked"
	}
}

// accountLength is the length every German account number is padded to before
// a method is applied.
const accountLength = 10

// method computes the expected check digit for a padded account number.
//
// The boolean reports whether a check digit could be derived at all. Some
// methods declare particular remainders or account ranges unusable, and those
// must not be turned into a failed check.
type method func(d []int) (expected int, checkPos int, ok bool)

// methods maps the Bundesbank identifier to its implementation. Identifiers
// absent from this map are reported as unchecked.
var methods = map[string]method{
	"00": m00, "01": m01, "02": m02, "06": m06, "09": m09,
	"10": m10, "13": m13, "28": m28, "32": m32, "34": m34,
	"88": m88, "99": m99, "17": m17, "C1": mC1,
}

// Implemented reports whether a method identifier has an implementation.
func Implemented(id string) bool {
	_, ok := methods[normaliseID(id)]
	return ok
}

// ImplementedMethods lists the identifiers this package can check.
func ImplementedMethods() []string {
	out := make([]string, 0, len(methods))
	for id := range methods {
		out = append(out, id)
	}
	return out
}

func normaliseID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

// Validate checks account against the Bundesbank method identified by id.
//
// It returns ResultUnchecked when the method is unknown or prescribes no
// calculation. An account number that is not numeric, or longer than ten
// digits, is reported as invalid because no German account number looks like
// that.
func Validate(id, account string) (Result, error) {
	id = normaliseID(id)

	fn, ok := methods[id]
	if !ok || !verified[id] {
		return ResultUnchecked, nil
	}

	digits, err := toDigits(account)
	if err != nil {
		return ResultInvalid, err
	}

	// Method 13 needs a second attempt with an assumed sub account number, so
	// it compares rather than only deriving a check digit.
	if id == "13" {
		return validate13(digits, strings.TrimSpace(account)), nil
	}

	expected, pos, usable := fn(digits)
	if !usable {
		return ResultUnchecked, nil
	}
	if digits[pos] == expected {
		return ResultValid, nil
	}
	return ResultInvalid, nil
}

// toDigits parses an account number into ten digits, left padded with zeros.
func toDigits(account string) ([]int, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, fmt.Errorf("checkdigit: empty account number")
	}
	if len(account) > accountLength {
		return nil, fmt.Errorf("checkdigit: account number %q is longer than %d digits",
			account, accountLength)
	}

	d := make([]int, accountLength)
	offset := accountLength - len(account)
	for i := range len(account) {
		c := account[i]
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("checkdigit: account number %q contains a non-digit", account)
		}
		d[offset+i] = int(c - '0')
	}
	return d, nil
}

// weightedSum multiplies digits[from..to] by weights, applied right to left,
// and adds the products.
//
// When crossSum is set, a two digit product is replaced by its digit sum, which
// for the weights in use is the same as subtracting nine.
func weightedSum(d []int, from, to int, weights []int, crossSum bool) int {
	sum, w := 0, 0
	for i := to; i >= from; i-- {
		p := d[i] * weights[w%len(weights)]
		if crossSum && p > 9 {
			p -= 9
		}
		sum += p
		w++
	}
	return sum
}

// mod10 derives the check digit used by the modulus 10 methods.
func mod10(sum int) int { return (10 - sum%10) % 10 }

// mod11Standard derives the check digit used by method 06 and the many methods
// that refer to it. A remainder of 0 or 1 yields check digit 0.
func mod11Standard(sum int) (int, bool) {
	switch rest := sum % 11; rest {
	case 0, 1:
		return 0, true
	default:
		return 11 - rest, true
	}
}

// mod11Strict is the method 02 variant, where a remainder of 1 makes the
// account number unusable rather than yielding a check digit.
func mod11Strict(sum int) (int, bool) {
	switch rest := sum % 11; rest {
	case 0:
		return 0, true
	case 1:
		return 0, false
	default:
		return 11 - rest, true
	}
}

// verified lists the methods whose implementation is confirmed against the
// test account numbers printed in the Bundesbank specification.
//
// Methods 01, 02 and 13 are implemented from the specification text but the
// document publishes no test numbers for them, so nothing proves the
// implementation is right. Until a vector exists they report unchecked rather
// than risking a false rejection of a real account. Method 13 alone covers
// about 7.5 percent of German institutions, which is exactly the scale at
// which an unverified guess would do damage.
var verified = map[string]bool{
	"00": true, "06": true, "09": true, "10": true,
	"28": true, "32": true, "34": true, "88": true, "99": true,
	"17": true, "C1": true,
}

// Verified reports whether a method's implementation is backed by official
// test vectors.
func Verified(id string) bool { return verified[normaliseID(id)] }
