# Bank data sources per country

Where `iban-pizza update` downloads each national bank registry from, under
which terms, and which euro area countries it does not cover and why. Every
source below was downloaded and parsed successfully on **2026-11-16**.

None of this data ships with the project. The repository, the binaries and the
image contain parsers only; `update` fetches each registry from its publisher
onto the machine that runs it, and that publisher's terms apply to whoever
downloaded it. `iban-pizza sources` prints the same terms from the program
itself, and they are stored with the data and reported at `/v2/data`.

The question a source has to answer is narrow: **which bank does the bank code
inside an IBAN belong to, and what is its BIC?** That is a different question
from whether a country publishes a list of licensed banks. Most do; fewer
publish the bank code in a form software can read.

## Downloaded by `iban-pizza update`

Record counts are from 2026-11-16 and move with every revision.

| Country | Publisher | Format | Records | BIC | Terms |
|---|---|---|---|---|---|
| AT | Oesterreichische Nationalbank | CSV | 875 | in the file | CC BY 4.0 |
| BE | National Bank of Belgium | XLSX | 790 | in the file | Publisher's terms: reproduction with integrity, accuracy and source |
| BG | Bulgarian National Bank | HTML table | 38 | in the file | Publisher's terms: source cited, not altered |
| CY | Central Bank of Cyprus | XLSX | 42 | in the file | Publisher's terms: accurate, source cited, changes stated |
| CZ | Česká národní banka | CSV | 47 | in the file | Publisher's terms: source named, not altered |
| DE | Deutsche Bundesbank | fixed width text | 3,507 | in the file | Publisher's terms: source named, not altered |
| ES | Banco de España | XLSX | 296 | joined by LEI | Publisher's terms: faithful reproduction, source cited |
| FI | Finance Finland | XLSX | 644 | in the file | No terms published |
| FR | ACPR, REGAFI register | CSV | 844 | joined by LEI | No terms defined by the portal |
| GR | Hellenic Bank Association | CSV | 36 | none | Publisher's terms: reproduction with source |
| HR | Croatian National Bank | XLSX | 22 | in the file | Publisher's terms: source named, not modified |
| IT | Banca d'Italia | CSV in a zip | 463 | joined by LEI | CC BY 4.0 |
| LU | ABBL | XLSX | 136 | in the file | Publisher's terms: reference use, unmodified, source indicated |
| MT | Central Bank of Malta | XLSX | 63 | in the file | Publisher's terms: source stated, not altered |
| PT | European Central Bank, MFI list | UTF-16 text, gzip | 122 | joined by LEI | ECB reuse policy: source named, changes stated |
| SI | Banka Slovenije | text | 821 | in the file | CC BY 4.0 |
| SK | Národná banka Slovenska | HTML table | 38 | in the file | Publisher's terms: source acknowledged, not modified |

That is 16 of the 21 euro area countries, plus the Czech Republic.

Only three of these are under a named open licence. The rest are the
publisher's own website terms. They allow downloading and using the list, and
several say the data may not be passed on altered. That is why nothing is
bundled: a converted copy inside a public image would be exactly that. Anyone
who wants to hand downloaded data on, as opposed to running the service on it,
has to read the terms of each registry for that purpose.

### Registries with known gaps

A bank code that a complete registry does not list is wrong, and the service
says so. Four registries are known not to list every institution that issues
IBANs in their country: **IT**, **ES**, **PT** and **GR**, for the reasons in
the notes below. They are marked `partial`, and a bank code missing from one of
them is reported as unchecked, not as invalid, so that a valid IBAN of an
institution the registry leaves out is not rejected.

### Notes per source

- **FR** is the register of the banking supervisor, not the Banque de France
  directory, which is subscription only. It covers every institution with a
  bank code, including branches of foreign banks and payment institutions.
  One institution can hold several codes and each becomes a record.
  Institutions in Monaco and the overseas territories are in the same file and
  are stored under FR; a Monaco IBAN, which starts with MC, is not resolved.
- **IT** is a register with history across many kinds of supervised firms.
  Only current entries from the registers of banks, payment institutions and
  electronic money institutions are kept. BancoPosta (07601) is not in it.
- **ES** covers banks, savings banks, credit cooperatives, the official credit
  institute, branches of foreign credit institutions, and payment and
  electronic money institutions. Representative offices, appraisal companies,
  guarantee societies and similar entries are dropped. An entity type the
  publisher adds or renames is dropped until the parser's list is extended.
- **PT** comes from the ECB list because Banco de Portugal publishes its own
  only as a legacy binary spreadsheet. The ECB identifier for a Portuguese
  institution is its bank code. Branches of foreign institutions, which the
  ECB keys differently, are missing, and Caixa Central de Crédito Agrícola is
  listed under 9000 by the ECB and stored under 0045, the code its IBANs use.
- **GR** names the bank and carries neither BIC nor LEI, so Greek records have
  no BIC and get no scheme answer.
- **BG** and **MT** use the first four letters of the BIC as the bank code. The
  Bulgarian register lists branches too; one record per bank is kept.
- **BE** assigns every code from 000 to 999; unassigned ones are dropped.
- **SI** identifies bank and branch together in five digits.
- **LU** and **CY** have a download link that changes with every revision, as
  does **DE**; all three are resolved from the publisher's page at run time.

## BICs joined from a second source

The registries of FR, IT, ES and PT name the institution and its LEI but no
BIC. For those the BIC comes from the BIC-to-LEI mapping table that SWIFT and
GLEIF publish, and every such record says so in `bicSource`.

| Country | Records | With a joined BIC |
|---|---|---|
| FR | 844 | 483 |
| IT | 463 | 331 |
| ES | 296 | 123 |
| PT | 122 | 36 |

Three limits are worth knowing:

- A BIC is filled in only when the mapping gives exactly one institution code
  for that LEI in that country. An ambiguous one is left empty, because a
  wrong BIC is worse than none.
- The mapping holds what each institution registered, and that is not always
  the BIC it uses for payments. Société Générale, for example, appears with a
  single BIC that is not its payment BIC. The joined BIC is therefore right
  for identifying the institution and not reliable for routing a payment.
- Scheme membership is looked up by BIC, so it inherits both limits.

The mapping table is not under GLEIF's CC0 dedication. It has its own licence,
which permits any use provided the SWIFT notice accompanies copies; the notice
is reproduced in `NOTICE` and by `iban-pizza sources`.

## Euro area countries not downloaded

| Country | Publisher | Why not |
|---|---|---|
| NL | Betaalvereniging Nederland | The list is free to read but its terms allow only personal, non commercial use and forbid copying or distributing it without written consent. |
| IE | BPFI sort code database | The download sits behind a captcha, and the terms forbid distributing the data or building a directory from it. |
| EE | Finantsinspektsioon | Reproduction and distribution need prior permission; use is allowed for the press and for non profit purposes. Published as a web page without BICs. |
| LT | Lietuvos bankas | The page that names the current file answers automated requests with a bot check, which this program does not try to get around. Terms could not be read. |
| LV | Latvijas Banka | Published only as a legacy binary spreadsheet the program has no reader for. The terms on the site cover statistics, not this list. |

For NL, LV, IE, BG and MT the bank code is the first four letters of the BIC,
so those countries could in principle be served from any BIC list. No such
list with clear terms has been found; the EPC register would do, and its terms
could not be read either.

If you hold one of these lists under your own agreement, `iban-pizza import`
loads it, in the publisher's format where a parser exists and in the generic
CSV format otherwise.

## Outside the euro area

CZ is covered. Verified free sources exist for CH and LI (SIX, legacy binary
spreadsheet), NO (Bits, XLSX) and HU (MNB, XLSX); none has a parser yet. GB
sort codes are licensed commercially. SE is published as a PDF. PL, DK, RO, IS
and the microstates have no located machine readable source.

## Scheme membership

Whether an institution takes part in SEPA Credit Transfer, Instant Credit
Transfer, Direct Debit core and B2B, One-Leg Out Instant and Verification of
Payee comes from the EPC Register of Participants, one CSV export per scheme at
`europeanpaymentscouncil.eu/sites/default/files/participants_export/{scheme}/{scheme}.csv`.
`iban-pizza update -scheme-dir` downloads them.

The register is keyed by the full eleven character BIC and describes the
institution, not an account. The EPC is a private association and its site
answers automated requests for its terms page with an error, so its terms have
not been reviewed here. Read them in a browser before relying on the register
for anything beyond running the service.

## What every source needs before it is added

1. A free download that works without an account, a captcha or a bot check.
2. The bank code exactly as it appears in the IBAN.
3. Terms that at least allow downloading and using the list. Whether they
   allow passing it on does not matter for a parser, since nothing is passed
   on, and is recorded all the same.
4. A parser that finds columns by name, a test on invented rows in the real
   layout, and a `Terms` entry.

No publisher's file is checked in, as a fixture or otherwise.
