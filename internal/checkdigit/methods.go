package checkdigit

// The digit slice passed to every method is ten entries long, index 0 being
// position 1 of the account number as the specification counts them. So
// "position 10 is the check digit" means index 9.

// m00 is modulus 10 with weights 2, 1 repeating over positions 1 to 9, taking
// the digit sum of each two digit product.
func m00(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 8, []int{2, 1}, true)
	return mod10(sum), 9, true
}

// m01 is modulus 10 with weights 3, 7, 1 repeating over positions 1 to 9.
// Unlike method 00 it does not reduce the products.
func m01(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 8, []int{3, 7, 1}, false)
	return mod10(sum), 9, true
}

// m02 is modulus 11 with weights 2 through 9 then 2, over positions 1 to 9.
// A remainder of 1 makes the account number unusable.
func m02(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 8, []int{2, 3, 4, 5, 6, 7, 8, 9, 2}, false)
	check, ok := mod11Strict(sum)
	return check, 9, ok
}

// m06 is modulus 11 with weights 2 to 7 repeating over positions 1 to 9.
func m06(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 8, []int{2, 3, 4, 5, 6, 7}, false)
	check, ok := mod11Standard(sum)
	return check, 9, ok
}

// m09 prescribes no calculation at all, so nothing is ever checked.
func m09(_ []int) (int, int, bool) { return 0, 9, false }

// m10 is method 06 with weights 2 through 10.
func m10(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 8, []int{2, 3, 4, 5, 6, 7, 8, 9, 10}, false)
	check, ok := mod11Standard(sum)
	return check, 9, ok
}

// m13 is method 00 applied to positions 2 to 7, with the check digit at
// position 8. Positions 9 and 10 hold a sub account number that must stay out
// of the calculation.
//
// The specification also allows for the sub account number being omitted, in
// which case the number is shifted left by two and "00" is assumed. That
// second attempt is handled in validate13 because it needs to compare, not
// just derive, a check digit.
func m13(d []int) (int, int, bool) {
	sum := weightedSum(d, 1, 6, []int{2, 1}, true)
	return mod10(sum), 7, true
}

// m28 is modulus 11 with weights 2 to 8 over positions 1 to 7, check digit at
// position 8. Positions 9 and 10 are a sub account number.
func m28(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 6, []int{2, 3, 4, 5, 6, 7, 8}, false)
	check, ok := mod11Standard(sum)
	return check, 7, ok
}

// m32 is modulus 11 with weights 2 to 7 over positions 4 to 9.
func m32(d []int) (int, int, bool) {
	sum := weightedSum(d, 3, 8, []int{2, 3, 4, 5, 6, 7}, false)
	check, ok := mod11Standard(sum)
	return check, 9, ok
}

// m34 is method 28 with the weights 2, 4, 8, 5, 10, 9, 7.
func m34(d []int) (int, int, bool) {
	sum := weightedSum(d, 0, 6, []int{2, 4, 8, 5, 10, 9, 7}, false)
	check, ok := mod11Standard(sum)
	return check, 7, ok
}

// m88 is modulus 11 over positions 4 to 9, with an exception: when position 3
// is a 9, positions 3 to 9 are used with an extra weight.
func m88(d []int) (int, int, bool) {
	var sum int
	if d[2] == 9 {
		sum = weightedSum(d, 2, 8, []int{2, 3, 4, 5, 6, 7, 8}, false)
	} else {
		sum = weightedSum(d, 3, 8, []int{2, 3, 4, 5, 6, 7}, false)
	}
	check, ok := mod11Standard(sum)
	return check, 9, ok
}

// m99 is method 06 with weights 2 to 7 then 2, 3, 4. Account numbers from
// 0396000000 to 0499999999 are exempt and count as correct, which is expressed
// here as "no check digit could be derived" so that the caller reports them as
// unchecked rather than as a failure.
func m99(d []int) (int, int, bool) {
	if n := value(d); n >= 396000000 && n <= 499999999 {
		return 0, 9, false
	}
	sum := weightedSum(d, 0, 8, []int{2, 3, 4, 5, 6, 7, 2, 3, 4}, false)
	check, ok := mod11Standard(sum)
	return check, 9, ok
}

// value returns the account number as an integer.
func value(d []int) int {
	n := 0
	for _, digit := range d {
		n = n*10 + digit
	}
	return n
}

// validate13 implements the retry that method 13 prescribes. The sub account
// number is sometimes omitted, in which case the digits sit two places to the
// left of where the method expects them.
func validate13(d []int, account string) Result {
	if expected, pos, ok := m13(d); ok && d[pos] == expected {
		return ResultValid
	}

	// Second attempt: append the assumed sub account number "00", which shifts
	// the existing digits two positions to the left.
	if len(account) > accountLength-2 {
		return ResultInvalid
	}
	shifted, err := toDigits(account + "00")
	if err != nil {
		return ResultInvalid
	}
	if expected, pos, ok := m13(shifted); ok && shifted[pos] == expected {
		return ResultValid
	}
	return ResultInvalid
}

// weightedSumLTR multiplies digits[from..to] by weights applied left to right,
// taking the digit sum of any two digit product whose weight is greater than
// one. Methods 17 and C1 are specified this way round, unlike the modulus 10
// and 11 families above which run right to left.
func weightedSumLTR(d []int, from, to int, weights []int) int {
	sum, w := 0, 0
	for i := from; i <= to; i++ {
		weight := weights[w%len(weights)]
		p := d[i] * weight
		if weight > 1 && p > 9 {
			p -= 9
		}
		sum += p
		w++
	}
	return sum
}

// mod11MinusOne is the check digit rule shared by methods 17 and C1: subtract
// one from the sum, divide by eleven, and subtract the remainder from ten. No
// remainder yields check digit zero.
func mod11MinusOne(sum int) int {
	rest := (sum - 1) % 11
	if rest < 0 {
		rest += 11
	}
	if rest == 0 {
		return 0
	}
	return 10 - rest
}

// m17 is modulus 11 over the six digit customer number in positions 2 to 7,
// with the check digit at position 8. Positions 1 and 9 to 10 hold an account
// type digit and a sub account number.
func m17(d []int) (int, int, bool) {
	sum := weightedSumLTR(d, 1, 6, []int{1, 2, 1, 2, 1, 2})
	return mod11MinusOne(sum), 7, true
}

// mC1v2 is the C1 variant used when the account starts with a five: positions
// 1 to 9 with the check digit at position 10.
func mC1v2(d []int) (int, int, bool) {
	sum := weightedSumLTR(d, 0, 8, []int{1, 2, 1, 2, 1, 2, 1, 2, 1})
	return mod11MinusOne(sum), 9, true
}

// mC1 selects the C1 variant by the first digit of the ten digit account
// number, as the specification prescribes.
func mC1(d []int) (int, int, bool) {
	if d[0] == 5 {
		return mC1v2(d)
	}
	return m17(d)
}
