# Data sources

Working notes on the pan-European sources and on the traps in the first three
national registries. The current list of every registry the program downloads,
with its terms, is in [country-sources.md](country-sources.md); where the two
disagree, that file is right.

The endpoints in this document were first verified on **2026-09-02**.

Two things are easy to conflate, and the distinction drives the whole design:

- **A national bank code registry** maps the bank code *inside an IBAN*
  (German BLZ, Italian ABI, UK sort code) to a bank. Only a national source has this.
- **A pan-European BIC directory** maps a BIC to a name, address and scheme
  membership. These cover all of SEPA but contain no national bank codes.

Resolving `DE89 3704 0044 ...` to "Commerzbank" needs the first kind. The second kind
enriches the answer once you have it. No pan-European source replaces the national ones.

## Tier 1: national bank code registries (verified live)

| Country | Endpoint | Format | Update | Notes |
|---|---|---|---|---|
| DE | `bundesbank.de/resource/blob/<id>/<hash>/.../blz-aktuell-{txt,csv,xml}-data.*` | Fixed width / CSV / XML, **ISO-8859-1**, CRLF | quarterly | URL carries a content hash, so it must be resolved from the landing page at run time. 13,806 rows. |
| AT | `oenb.at/docroot/downloads_observ/sepa-zv-vz_gesamt.csv` | CSV `;`, **ISO-8859-1**, 3 preamble lines | **daily** | Replaces the dead `kiverzeichnis` URL. Carries an explicit copyright and liability reservation in its header, see Licensing. |
| CH + LI | `six-group.com/dam/download/banking-services/interbank-clearing/bc-bank-master/bcbankenstamm_d.xls` | XLS, ~1.5 MB | continuous | Path moved from `/dam/downloads/`. **Also contains Liechtenstein institutions**, which is why LI needs no separate source. |
| BE | `nbb.be/doc/be/be/protocol/full_list_current.xlsx` | XLSX | continuous | Stable `_current` filename, no hash to resolve. |
| NL | `betaalvereniging.nl/wp-content/uploads/BIC-lijst-NL.xlsx` | XLSX | continuous | Redirects to a dated path; follow redirects. |
| CZ | `cnb.cz/cs/platebni-styk/.galleries/ucty_kody_bank/download/kody_bank_CR.csv` | CSV `;`, UTF-8 | continuous | Columns: payment code, provider, BIC, CERTIS flag. **Served with `Content-Type: text/html` despite being CSV**, do not gate parsing on the content type. |

### Further countries
Belgium and thirteen more countries have been added since; see
[country-sources.md](country-sources.md) for all of them and for the euro area
countries that cannot be downloaded automatically.

## Tier 2: pan-European, BIC keyed (verified live)

| Source | Endpoint | Gives | Licence |
|---|---|---|---|
| EPC Register of Participants | `europeanpaymentscouncil.eu/sites/default/files/participants_export/{scheme}/{scheme}.csv` for `sct`, `sct_inst`, `sdd_core`, `sdd_b2b`, `oct_inst`, `vop` | BIC, name, address, city, country, readiness date, leaving date, scheme options | EPC terms |
| ECB MFI list | daily release linked from `mid.ecb.europa.eu/rss/mid.xml`, files named `mfi_csv_<yymmdd>.csv.gz` | RIAD code, **LEI**, country, name, address, postal, city, category, head office | ECB terms |
| GLEIF BIC-to-LEI | `mapping.gleif.org/api/v2/bic-lei/<uuid>/download` (UUIDs listed on the GLEIF download page) | `LEI,BIC` pairs | BIC/LEI Mapping Table License Agreement (SWIFT notice required), **not CC0** |

Notes that cost time if discovered late:

- **The ECB MFI file has no BIC column.** It carries LEI. Reaching a BIC means joining
  MFI to LEI to GLEIF to BIC. It also has no national bank codes.
- **The ECB MFI file is UTF-16 and tab separated**, not UTF-8 CSV.
- Coverage: 5,370 institutions across all 27 EU countries, daily.
- **The EPC exports do not share a column layout.** `sct` and `sdd_*` have 8 columns,
  `oct_inst` has 9 (an extra `Role`), and `vop` has 10 (`Status` and `Role` inserted at
  positions 6 and 7). Resolve columns by header name; reading them positionally puts role
  text into the leaving date field for two of the six schemes.
- EPC BICs are 11 characters; join on the full BIC, not an 8 character prefix.

## Scheme membership is institution level

Joining bank data to the EPC register answers "did this institution adhere to the
scheme", not "can this account receive such a payment". ING-DiBa (`INGDDEFFXXX`) is
listed for `sdd_b2b` since 2021-10-10, yet does not offer B2B direct debit to retail
customers. Both statements are true at different levels. The API reports the register
level and says so; see `internal/sepa`.

## Licensing

Code and data are separate questions. The code is MIT. The data is not part of
the project at all: nothing is bundled, `iban-pizza update` downloads each
registry from its publisher, and that publisher's terms apply to whoever ran
the download.

- **OeNB (AT)**: the OeNB's Impressum und Haftung, section 2.2, grants reuse of
  OeNB website data under **CC BY 4.0**, commercial use included. Re-verified
  2026-11-16.
- **Bundesbank (DE)** and **ČNB (CZ)**: storing, passing on and reproducing is
  allowed with the source named, and both say the information may not be
  altered. A converted extract inside a public image is hard to square with
  that, which is the reason the bundled snapshot was removed.
- **GLEIF BIC-to-LEI mapping**: GLEIF's LEI data is CC0, the mapping table is
  not. It carries its own licence from SWIFT and GLEIF, which permits any use
  provided the SWIFT notice accompanies copies. See `NOTICE`.
- **ECB**: free use with the source cited and modifications stated. The
  separate policy for ESCB statistics adds that statistics are not to be
  modified; whether the MFI list counts as statistics is not settled.
- **EPC**: terms not reviewed; the site blocks automated requests for them.

The terms of every other registry are summarised in
[country-sources.md](country-sources.md) and printed by `iban-pizza sources`.
