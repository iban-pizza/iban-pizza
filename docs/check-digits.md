# German account check digits

The Bundesbank assigns every German bank code a check digit method identifier
and publishes about ninety algorithms in
`pruefzifferberechnungsmethoden-data.pdf`. The identifier travels with the bank
record, so a lookup already knows which method applies to an account.

The upstream project parsed that identifier, stored it in its database, and
never implemented a single method. Account numbers were therefore never
checked.

## The rule that governs this package

**An unverified method reports "unchecked", never "invalid".**

Rejecting a valid customer account is far worse than admitting that nothing was
checked. The consequences follow from that:

- A method with no implementation returns `ResultUnchecked`.
- A method with an implementation but no official test vectors also returns
  `ResultUnchecked`, because nothing proves the implementation correct.
- The API renders unchecked as `"ok": null`, which does not count against the
  overall `valid`.

## Implemented and verified

Eleven methods are implemented and confirmed against the test account numbers
printed in the specification. `TestOfficialVectors` runs every published vector
on every build.

| Method | Rule | Share of German institutions |
|---|---|---|
| 09 | No calculation is prescribed, so nothing is ever claimed | 20.2% |
| 00 | Modulus 10, weights 2 and 1, digit sum of products | 9.4% |
| 88 | Modulus 11 over positions 4 to 9, with a rule for a leading 9 | 8.2% |
| 06 | Modulus 11, weights 2 to 7 | 7.8% |
| 10 | Modulus 11, weights 2 to 10 | 5.6% |
| 32 | Modulus 11 over positions 4 to 9 | 4.6% |
| 28 | Modulus 11 over positions 1 to 7 | 3.9% |
| 34 | Method 28 with weights 2, 4, 8, 5, 10, 9, 7 | 3.5% |
| 99 | Method 06 with an exempt account range | 2.9% |
| 17 | Modulus 11 over the customer number, left to right | |
| C1 | Two variants selected by the first digit, using method 17 | |

Method C1 is ING-DiBa's. It is included despite its low share of institutions
because the bank behind it is large, and it has twenty published vectors
covering both variants and both outcomes.

## Implemented but held back

Methods 01, 02 and 13 have an implementation derived from the specification
text, but the document publishes no test account numbers for them. They are
listed in `methods` and excluded from `verified`, so they report unchecked.

Method 13 alone covers about 7.5 percent of German institutions. That is
exactly the scale at which shipping an unverified guess would do real damage,
which is why it stays gated rather than being enabled on the strength of a
careful reading.

To enable one, find or derive authoritative vectors, add them to
`testdata/vectors.tsv`, and add the identifier to `verified`.
`TestVerifiedMethodsHaveVectors` enforces that direction too: nothing can be
marked verified unless a vector actually exercises it.

## Independent confirmation

Beyond the specification's own vectors, the C1 implementation was checked
against a third party IBAN checker for a real ING-DiBa account
(`DE49500105179144355668`). Both report the same four outcomes: correct length,
valid bank code, wrong account check digit, correct IBAN checksum.

## Extracting the vectors

The vectors in `testdata/vectors.tsv` were extracted from the specification PDF
with `pdftotext -layout`. One detail is worth recording, because it produced
wrong tests before it was caught: test numbers continue across page boundaries,
and a naive extractor attributes the numbers at the top of a page to the method
that ended on the previous one. Three account numbers were assigned to method
88 that way, and the resulting test failed against a correct implementation.
The extractor must stop collecting at page footers.
