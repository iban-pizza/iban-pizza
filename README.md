<br>

<p align="center">
  <a href="https://iban.pizza">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset=".github/brand/wordmark-dark.svg">
      <source media="(prefers-color-scheme: light)" srcset=".github/brand/wordmark-light.svg">
      <img alt="iban.pizza" src=".github/brand/wordmark-light.svg" height="72">
    </picture>
  </a>
</p>

<h3 align="center">IBAN validation, bank lookup and SEPA schemes as one binary</h3>

<p align="center">
  No database required. Bank data comes straight from the official registries, downloaded by you with one command. Self-host with Docker or a single static binary.
</p>

<p align="center">
  <a href="https://iban.pizza"><strong>Website</strong></a> &middot;
  <a href="docs/deployment.md"><strong>Deploy</strong></a> &middot;
  <a href="internal/api/openapi.yaml"><strong>API</strong></a> &middot;
  <a href="https://github.com/iban-pizza/iban-pizza/releases"><strong>Releases</strong></a>
</p>

<br>

A ground up rewrite of the [goiban](https://github.com/apilayer/goiban) family
of projects, whose last release was in 2019.

```
git clone https://github.com/iban-pizza/iban-pizza && cd iban-pizza
docker compose up
curl http://localhost:8080/v2/iban/DE89370400440532013000
```

The first start downloads the bank registries from their official publishers
into a volume, then serves from them. Nothing else is needed: no database, no
configuration.

**No bank data ships with this project.** Every registry has its own terms of
use, and most let you download and use the data but do not let a project hand
it on. So the repository, the binaries and the image contain parsers only, and
`iban-pizza update` fetches the data onto the machine that uses it. The terms
of each publisher apply to what you download; `iban-pizza sources` lists them.
Until the data is there, the service validates the structure and check digits
of an IBAN and resolves no bank.

While this repository is private, its image is private too and `docker pull`
needs a login with the `read:packages` scope; the released binaries do not. See
[docs/deployment.md](docs/deployment.md) for both.

## Getting started

**[docs/deployment.md](docs/deployment.md)** covers every way to run it:

- **Binary**, downloaded from the
  [releases page](https://github.com/iban-pizza/iban-pizza/releases),
  verified against `SHA256SUMS`, run with one command. Includes a systemd unit.
- **Docker and Compose**, standalone or with PostgreSQL.
- **Kubernetes**, from the manifests in `deploy/kubernetes/`.
- **Downloading the data and keeping it current.**

Releases are built by GitHub Actions from a tag: static binaries for Linux,
macOS and Windows, a multi architecture container image on GHCR, and a
checksum file, all attached to the release.

## What it does

```
GET  /v2/iban/{iban}                        Validate and describe
POST /v2/iban:batch                         Up to 100 at once
GET  /v2/banks?country=&bic=&name=&limit=   Search banks
GET  /v2/banks/{country}/{bankCode}         One bank
GET  /v2/banks/{country}/{bankCode}/logo.svg
GET  /v2/countries                          The IBAN registry
GET  /v2/data                               What is loaded, from where, how old
GET  /healthz /readyz /openapi.yaml
```

The v1 routes of openiban.com are served unchanged alongside them, so an
existing client only changes its base URL:

```
GET /validate/{iban}?validateBankCode=true&getBIC=true
GET /countries
GET /calculate/{countryCode}/{bankCode}/{accountNumber}
GET /v2/calculate/{countryCode}/{bankCode}/{accountNumber}
```

A v2 answer for a real account:

```json
{
  "valid": false,
  "iban": { "formatted": "DE49 5001 0517 9144 3556 68", "bankCode": "50010517" },
  "checks": {
    "length":        { "ok": true,  "message": "Correct length for DE (22 characters)" },
    "bankCode":      { "ok": true,  "message": "Bank code 50010517 is valid" },
    "accountNumber": { "ok": false, "method": "C1",
                       "message": "Account number 9144355668 has a wrong check digit" },
    "ibanChecksum":  { "ok": true,  "message": "The IBAN checksum is correct" }
  },
  "bank": { "name": "ING-DiBa", "bic": "INGDDEFFXXX", "city": "Frankfurt am Main" },
  "schemes": { "level": "institution", "schemes": { "sctInst": { "status": "participant" } } },
  "dataAsOf": { "DE": "2026-09-02", "schemes": "2026-09-02" }
}
```

## Two things to know before integrating

**`ok` can be null.** A check that could not be performed is not a failure. The
account number of a bank whose check digit method is not implemented reports
`"ok": null`, and the overall `valid` stays true. Rendering null as a failure
would reject correct accounts.

**Scheme membership is per institution, not per account.** The EPC register
records that an institution joined a scheme. ING-DiBa is listed for SEPA B2B
direct debit and still does not offer it to retail customers. A status of
`unknown` means the BIC is absent from the register entirely, which is common
for savings and cooperative banks reachable through a central institution, so
it must not be read as "not supported".

## Why a rewrite

The upstream is four repositories from before Go modules existed, untouched
since August 2019. The rewrite addresses what that age produced.

**Its bundled data froze on 2019-07-25.** Two of the seven download URLs in its
`sources.txt` are dead today, and Germany's institution count has fallen from
about 17,000 to 13,806 since. Here, `iban-pizza update` resolves the official
endpoints at run time, `/healthz` reports the age of every dataset, and a
scheduled job opens a pull request every quarter so a missed refresh is visible
instead of silent.

**`fmt.Fprintf(w, value)` received user controlled content**, so a percent sign
in the IBAN parameter reached the format verb parser. All responses now go
through `json.Encoder`, and a test feeds format verbs at the routes.

**The response cache was keyed on the raw request parameter with no length
check**, so arbitrary requests grew memory without bound. Input is now rejected
on length before anything is allocated.

**Account check digits were never verified.** The upstream parsed the
Bundesbank method identifier, stored it, and implemented no method. Eleven
methods are implemented here, each verified against the test account numbers in
the Bundesbank specification.

**BBAN validation only compared total length**, accepting any alphanumeric
content of the right size. Validation is now driven by the IBAN registry
structure per country, so `DE89AB0400440532013000` is rejected rather than
accepted.

**CORS was an unconditional wildcard.** It is now configured, and closed by
default.

## Data

Bank data comes from official national registries.

- [docs/country-sources.md](docs/country-sources.md) lists every registry the
  program downloads, with its terms, and the euro area countries it cannot
  cover and why.
- [docs/data-sources.md](docs/data-sources.md) has working notes on the
  pan-European sources and the layout traps of the first registries.

`iban-pizza update` downloads a registry for these countries:

Austria, Belgium, Bulgaria, Croatia, Cyprus, the Czech Republic, Finland,
France, Germany, Greece, Italy, Luxembourg, Malta, Portugal, Slovakia, Slovenia
and Spain: 16 of the 21 euro area countries, each from its central bank,
supervisor or banking association,
plus the EPC Register of Participants for all six SEPA schemes, which is what
answers whether an institution takes part in SEPA Credit Transfer, Instant
Credit Transfer and Direct Debit (core and B2B). Run `iban-pizza sources` for
the publisher, terms of use and required source statement of each.

Some registries name a bank and its LEI but no BIC. For those the BIC is joined
from the BIC-to-LEI mapping published by SWIFT and GLEIF, and the answer says
so in `bicSource`. A BIC is only filled in when the mapping identifies exactly
one institution code in the country; an ambiguous one is left empty rather
than guessed.

Not downloaded automatically: the Netherlands and Estonia, whose publishers
restrict the list to personal or non commercial use, Ireland and Lithuania,
whose files sit behind a captcha or a bot check, and Latvia, which publishes a
legacy binary spreadsheet. If you hold the data under your own agreement, load
it with `iban-pizza import`.

Resolving the bank code inside an IBAN needs a national registry. Pan-European
sources such as the EPC register, the ECB MFI list and GLEIF are keyed by BIC
or LEI and enrich the answer, but carry no national bank codes and cannot
replace those registries. Countries without a loaded registry still get full
structural IBAN validation.

### There is no separate loader

The upstream split the work across four repositories, so a deployment meant
running `goiban-service`, populating a MySQL database with `goiban-data-loader`
as a second program, and keeping both in step. All four collapsed into this one
repository, and the loader became a subcommand of the same binary.

| Upstream repository | Here |
|---|---|
| `goiban` | `iban/`, `internal/checkdigit/` |
| `goiban-data` | `bankdata/`, rewritten because the original carries no licence |
| `goiban-data-loader` | `internal/sources/` plus `iban-pizza update` |
| `goiban-service` | `internal/api/` plus `iban-pizza serve` |

### How data gets in

**1. Download into a file.** This is the default:

```sh
iban-pizza sources   # who publishes what, under which terms
iban-pizza update    # downloads into data/bankdata.jsonl.gz
iban-pizza serve
```

`-data-file`, or `IBAN_PIZZA_DATA_FILE`, moves the file; the container image
sets it to `/data/bankdata.jsonl.gz`. `-countries DE,AT` downloads a subset,
and `-dry-run` downloads and parses without storing, which is the safe way to
check whether a registry has changed its format. Run `update` again whenever
the data should be newer, then restart the service to load it.

Without the file the service still starts. It validates structure and check
digits, reports the bank code as unchecked, and answers `/readyz` with 503
until data is loaded.

**2. Copy one download to other machines**, when only one host may reach the
publishers:

```sh
iban-pizza update -data-file bankdata.jsonl.gz          # where egress is allowed
iban-pizza import -snapshot bankdata.jsonl.gz -database-url postgres://...
```

**3. Refresh into PostgreSQL**, for central maintenance across instances:

```sh
iban-pizza update -database-url postgres://user:pass@host/iban
iban-pizza serve  -database-url postgres://user:pass@host/iban
```

There is no separate step to enable PostgreSQL. Passing `-database-url`, or
setting `IBAN_PIZZA_DATABASE_URL`, is the whole switch. The schema is created on
connect, so an empty database is enough to start, and `update` has to run once
before the service can resolve a bank.

With Compose that is one command:

```sh
docker compose -f compose.yaml -f compose.postgres.yaml up
```

which starts PostgreSQL, downloads the registries into it once, and then
starts the service against it. Refresh later without touching the service:

```sh
docker compose -f compose.yaml -f compose.postgres.yaml run --rm data
```

In every case `iban-pizza update` resolves the download links from the
publishers' pages at run time, refuses to replace existing data when a download
or parse fails, and swaps one country at a time so a broken registry cannot
take the others down with it.

**4. Import a file you already have**, per country, with no network:

```sh
iban-pizza import -country DE -file blz-aktuell.txt -database-url postgres://...
```

Built in parsers cover the countries listed above; every other country
accepts a generic CSV with a `bankCode,name,...` header. Details in the
[deployment guide](docs/deployment.md#importing-a-file-you-already-have).

The scheduled `check-sources` workflow downloads and parses every registry each
week and stores nothing, so a publisher changing its format shows up as a
failed run in this repository before it reaches a user.

## Running it

### Container

```sh
docker compose up
```

or, without Compose, with a volume that holds the downloaded data:

```sh
docker volume create bankdata
docker run --rm -v bankdata:/data ghcr.io/iban-pizza/iban-pizza:latest update
docker run -p 8080:8080 -v bankdata:/data ghcr.io/iban-pizza/iban-pizza:latest
```

The image is built `FROM scratch`, runs as uid 65534, and contains the binary
and certificate roots and no bank data.

### Binary

```sh
iban-pizza update                      # download the registries once
iban-pizza serve                       # serves data/bankdata.jsonl.gz
iban-pizza serve -data-file data.gz    # a data file somewhere else
iban-pizza serve -database-url postgres://...
```

Flags, each with an `IBAN_PIZZA_`-prefixed environment variable equivalent:

| Flag | Default | Purpose |
|---|---|---|
| `-addr` | `:8080` | listen address |
| `-data-file` | `data/bankdata.jsonl.gz` | data file written by `iban-pizza update` |
| `-database-url` | | PostgreSQL connection string |
| `-scheme-file` | | directory of EPC exports, enables the schemes block |
| `-cors-origins` | none | comma separated origins, or `*` |
| `-rate-limit` | 600 | requests per minute per client address, 0 disables |
| `-stale-after` | 120 days | age at which data is reported stale |
| `-base-url` | | public URL, for absolute logo links |
| `-log-format` | `json` | `json` or `text` |

### Refreshing the data

```sh
iban-pizza update -scheme-dir data/schemes
```

Download links are resolved at run time rather than hard coded, because the
Bundesbank path carries a content hash that changes with every quarterly
release. `--dry-run` downloads and parses without storing, and `--countries DE`
refreshes one registry.

## Client libraries

Two official clients live in this repository and are tested in CI against a
service built from the same commit, so an API change that breaks a client
fails here rather than at a user.

| Language | Package | Where |
|---|---|---|
| JavaScript / TypeScript | `npm install iban-pizza` | [`clients/js`](clients/js) |
| Python | `pip install iban-pizza` | [`clients/python`](clients/python) |

Both are thin: no runtime dependencies, the platform's own HTTP, and one
method per endpoint. The TypeScript types are generated from
`internal/api/openapi.yaml` at build time, so they cannot drift from the API.

```ts
import { IbanPizza } from "iban-pizza";
const api = new IbanPizza({ baseUrl: "https://iban.pizza" });
const r = await api.validate("DE89 3704 0044 0532 0130 00");
```

```python
from iban_pizza import IbanPizza
r = IbanPizza("https://iban.pizza").validate("DE89 3704 0044 0532 0130 00")
```

Anything written against openiban.com keeps working too: the v1 routes are
served unchanged, so an existing client only changes its base URL.

## Development

```sh
make test     # go test ./... -race
make cover    # coverage summary
make lint     # gofmt and go vet
make vuln     # govulncheck
make build    # static binary into bin/
make update   # download the registries into ./data (not checked in)
make docker   # build the image
```

Releases are cut by pushing a tag. `v1.2.3` builds static binaries for Linux,
macOS and Windows on amd64 and arm64 with a `SHA256SUMS` file, publishes a
multi architecture image to GHCR with an SBOM and provenance attestation, signs
it with cosign, and creates the GitHub release.

## Layout

```
iban/                 IBAN parsing, mod-97, registry driven BBAN structure
bankdata/             Bank data model and repository interface
  postgres/           PostgreSQL backend
internal/sources/     One parser per national registry
internal/sepa/        EPC Register of Participants
internal/checkdigit/  German account check digit methods
internal/logo/        Brand mapping and monogram generation
internal/api/         v1 compatibility surface, v2, middleware, interface
cmd/iban-pizza/       serve, update, sources, import, snapshot, version
```

`iban` and `bankdata` are public packages, so the core stays importable as a
library the way the upstream `goiban` package was.

## Licence

MIT. Derived from Chris Grieger's MIT licensed `goiban`, `goiban-service` and
`goiban-data-loader`. It contains no code from `goiban-data`, which carries no
licence at all. Not affiliated with openiban.com. See [NOTICE](NOTICE).

Bank names and logos are trademarks of their owners. No third party artwork is
served by default; logos are generated monograms unless an operator configures
a resolver.
