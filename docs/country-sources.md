# Bank data sources per SEPA country

Status of the national bank code registry for all 36 SEPA countries. Every URL
marked verified was fetched successfully on **2026-09-02** and returned the
stated content type and size.

The question this table answers is narrow: **is there a free, machine readable
list that maps the bank code inside an IBAN to a bank and its BIC?** That is a
different question from whether a country publishes a list of licensed banks.
Most do; far fewer publish the bank code mapping in a form software can read.

## Summary

| Tier | Countries | Meaning |
|---|---|---|
| A, machine readable and verified | DE, AT, CH, LI, BE, NL, CZ, NO, HU, LV | Free, parseable, live |
| B, published but PDF only | SE, FI, ES | Free but needs PDF extraction |
| C, subscription or commercial | FR, GB, IT | No free machine readable route found |
| D, not located | the remaining 20 | No free source found yet, see notes |

Tier A covers the countries where a bank lookup can be offered today. Countries
outside it still get full structural IBAN validation, plus BIC level enrichment
from the pan-European sources at the end of this document.

## Tier A: machine readable, verified live

| Country | Endpoint | Format | Size | Cadence |
|---|---|---|---|---|
| DE | `bundesbank.de/resource/blob/<id>/<hash>/.../blz-aktuell-txt-data.txt` | Fixed width, windows-1252, CRLF | 282 KB | quarterly |
| AT | `oenb.at/docroot/downloads_observ/sepa-zv-vz_gesamt.csv` | CSV `;`, ISO-8859-1 | 207 KB | daily |
| CH, LI | `six-group.com/dam/download/banking-services/interbank-clearing/bc-bank-master/bcbankenstamm_d.xls` | XLS | 1.5 MB | continuous |
| BE | `nbb.be/doc/be/be/protocol/full_list_current.xlsx` | XLSX | 50 KB | continuous |
| NL | `betaalvereniging.nl/wp-content/uploads/BIC-lijst-NL.xlsx` | XLSX | 47 KB | continuous |
| CZ | `cnb.cz/cs/platebni-styk/.galleries/ucty_kody_bank/download/kody_bank_CR.csv` | CSV `;`, UTF-8 | 2 KB | continuous |
| NO | `bits.no/document/iban/` | XLSX, columns `Bank identifier, BIC, Bank` | 56 KB | continuous |
| HU | `mnb.hu/letoltes/sht.xlsx` | XLSX, columns `Branch office code, BIC code, Name, Address` | 230 KB | continuous |
| LV | `bank.lv/images/stories/pielikumi/makssist/BIC_IBAN/bic_saraksts_2026_ENG1.xls` | XLS, English edition | 49 KB | yearly filename |

Implemented so far: DE, AT, CZ. The rest are pending a spreadsheet reader; see
"Why the spreadsheet countries are not implemented yet" below.

### Traps in these files

- **DE** is fixed width in *characters* and encoded windows-1252. Decoding it as
  UTF-8, or slicing bytes rather than runes, shifts every column after an umlaut
  and moves BICs onto the wrong institution.
- **AT** is ISO-8859-1 with five lines of legal notice and a query date above the
  header row. Locate the header by content, not by counting lines.
- **CZ** is served with `Content-Type: text/html` although the body is CSV.
  Never gate parsing on the content type.
- **DE** download URLs carry a content hash that changes with every quarterly
  release, so the link must be resolved from the landing page at run time.
- **LV** puts the year in the filename, so that link needs resolving too.

## Tier B: published, but PDF only

| Country | Source | Note |
|---|---|---|
| SE | `bankinfrastruktur.se/media/kelmctkm/1906_clearingnummer-institut-221212_-nummerordning.pdf` | 123 KB PDF of clearing numbers by institution. Verified live. |
| FI | `finanssiala.fi/wp-content/uploads/2021/03/Suomalaiset_rahalaitostunnukset_ja_BIC-koodit.pdf` | 30 KB PDF of Finnish institution codes and BICs. Verified live. |
| ES | Banco de España "Registros de Entidades", published as PDF | The NRBE code is the first four digits of the Spanish bank code. Only PDF registers were found free of charge; structured data is sold commercially. |

These are usable with `pdftotext -layout` plus a parser, the same route used to
extract the check digit vectors in `docs/check-digits.md`. A PDF layout can
change without warning, so any such parser needs a row count assertion to catch
a silent format change.

## Tier C: subscription or commercial only

| Country | Situation |
|---|---|
| FR | The Banque de France Fichier des implantations bancaires (FIB) holds the CIB codes and is distributed to subscribers over a dedicated portal. No free bulk download exists. |
| GB | Sort codes come from the industry sort code directory, licensed commercially through Vocalink and Pay.UK. |
| IT | Banca d'Italia publishes a PDF list of reporting banks; the full ABI and CAB database is distributed commercially. |

For these three, an IBAN still validates structurally, and the BIC level
enrichment below still applies. Only the bank code lookup is unavailable.

## Tier D: not located yet

BG, HR, CY, DK, EE, GR, IE, LT, LU, MT, PL, PT, RO, SK, SI, IS, MC, SM, AD, VA.

What is known about them:

- **PL** publishes the EWIB register of settlement numbers at `ewib.nbp.pl`, but
  the site is a JavaScript application and no direct file endpoint was found.
- **DK** points at `registreringsnumre.dk`, operated by Mastercard Payment
  Services. The page responds but exposes no data file link.
- **BG, RO, LT** have central bank pages that respond, but none carried a
  downloadable file. **LT** returns 403 to non browser clients.
- **SK, SI, HR** central bank pages moved; the previously documented paths 404.
- **MC** is covered by the French FIB and therefore inherits the French
  situation. **SM, AD, VA** are small enough that no national register was
  expected.
- **IE, PT, GR, CY, MT, EE, LU, IS** were checked at their obvious sources
  without a machine readable result. Several return 403 to scripted requests,
  so some of these are worth a second look through a browser.

Tier D is a research gap, not a statement that no source exists. Each entry
needs one focused look at the national central bank or payment association.

## Pan-European sources

These cover every SEPA country but are keyed by BIC or LEI, so they enrich an
answer rather than replacing a national registry. None of them carries a
national bank code.

| Source | Endpoint | Gives | Licence |
|---|---|---|---|
| EPC Register of Participants | `europeanpaymentscouncil.eu/sites/default/files/participants_export/{scheme}/{scheme}.csv` | BIC, name, address, city, country, scheme adherence | EPC terms |
| ECB MFI list | daily release from `mid.ecb.europa.eu/rss/mid.xml`, files `mfi_csv_<yymmdd>.csv.gz` | RIAD code, LEI, country, name, address, category | ECB terms |
| GLEIF BIC to LEI | `mapping.gleif.org/api/v2/bic-lei/<uuid>/download` | `LEI,BIC` pairs | CC0 |

Notes worth knowing before relying on them:

- The **ECB MFI file has no BIC column**, only LEI. Reaching a BIC means joining
  MFI to LEI to GLEIF. It is also UTF-16 and tab separated, not UTF-8 CSV.
  Coverage is 5,370 institutions across all 27 EU countries, updated daily.
- The **EPC exports do not share a column layout**: 8 columns for `sct` and
  `sdd_*`, 9 for `oct_inst`, 10 for `vop`. Resolve columns by header name.
- EPC BICs are 11 characters. Join on the full BIC, not an 8 character prefix.

## Why the spreadsheet countries are not implemented yet

Six of the nine Tier A countries publish XLSX or XLS. Reading XLSX needs a
library such as `excelize`, and the Swiss file is the older binary XLS format
which `excelize` does not read at all.

The service currently has one dependency, a PostgreSQL driver. Adding a
spreadsheet library for five countries, and a second one for Switzerland, is a
real trade against the goal of a small auditable binary. The `sources.Source`
interface makes each country a self contained plug in, so the decision can be
taken per country rather than all at once.

A cheaper route worth trying first: several of these publishers also expose CSV
or XML variants that are not linked from the obvious page, exactly as the
Bundesbank does. Germany was moved off fixed width parsing that way.

## Licensing

Terms differ per source and matter for redistributing a populated snapshot.

- **GLEIF** is CC0 and unrestricted.
- **OeNB (AT)** states in the CSV header that the data is provided solely under
  its own disclaimer and copyright terms.
- **Bundesbank, SIX, NBB, Betaalvereniging, CNB, MNB, Bits, Latvijas Banka, EPC,
  ECB** each publish their own terms of use.

Review these before shipping a container that embeds the data commercially. See
`NOTICE` for the code side, which is a separate question.
