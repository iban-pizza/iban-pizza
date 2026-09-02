# iban.pizza

IBAN validation, bank lookup and SEPA scheme membership, served as a single
static binary with no runtime dependencies.

A ground up rewrite of the [goiban](https://github.com/apilayer/goiban) family
of projects, whose last release was in 2019.

```
docker run -p 8080:8080 ghcr.io/netzfabrikcom/iban-pizza:0.1.0
curl http://localhost:8080/v2/iban/DE89370400440532013000
```

No database, no data directory, no configuration. The bank data is compiled
into the binary, so nothing has to be downloaded before the first request.

While this repository is private, its image is private too and `docker pull`
needs a login with the `read:packages` scope; the released binaries do not. See
[docs/deployment.md](docs/deployment.md) for both.

## Getting started

**[docs/deployment.md](docs/deployment.md)** covers every way to run it:

- **Binary**, downloaded from the
  [releases page](https://github.com/NETZFABRIKCOM/iban-pizza/releases),
  verified against `SHA256SUMS`, run with one command. Includes a systemd unit.
- **Docker and Compose**, standalone or with PostgreSQL.
- **Kubernetes**, from the manifests in `deploy/kubernetes/`.
- **Keeping the data current**, and what is and is not compiled in.

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
about 17,000 to 13,806 since. Here, `openiban update` resolves the official
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

- [docs/country-sources.md](docs/country-sources.md) has the status of all 36
  SEPA countries: which publish a free machine readable bank code registry,
  which publish only a PDF, which sell it, and which are still unresearched.
- [docs/data-sources.md](docs/data-sources.md) has the working detail for the
  sources in use, including the encoding and layout traps.

Currently loaded: Germany (Bundesbank), Austria (OeNB), Czech Republic (CNB),
plus the EPC Register of Participants for all six SEPA schemes. Six more
countries have a verified free source waiting on a spreadsheet reader:
Switzerland and Liechtenstein, Belgium, the Netherlands, Norway, Hungary and
Latvia.

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
| `goiban-data-loader` | `internal/sources/` plus `openiban update` |
| `goiban-service` | `internal/api/` plus `openiban serve` |

### Three ways data gets in

**1. Do nothing.** A snapshot of every registry is compiled into the binary,
so `openiban serve` answers immediately. The whole dataset is about 110 KB
compressed for 4,423 institutions. This is the default and needs no network, no
file and no database.

**2. Refresh into a file**, when data should be newer than the binary:

```sh
# The file does not have to exist. update creates it, and the directory too.
openiban update --write-snapshot /var/lib/openiban/data.gz
openiban serve  -data-file /var/lib/openiban/data.gz
```

No rebuild is involved. `--countries DE,AT` refreshes a subset, and `--dry-run`
downloads and parses without storing, which is the safe way to check whether a
registry has changed its format.

**3. Refresh into PostgreSQL**, for central maintenance across instances:

```sh
openiban update -database-url postgres://user:pass@host/iban
openiban serve  -database-url postgres://user:pass@host/iban
```

There is no separate step to enable PostgreSQL. Passing `-database-url`, or
setting `OPENIBAN_DATABASE_URL`, is the whole switch. The schema is created on
connect, so an empty database is enough to start, and the loader has to run
once before the service can answer anything.

With Compose that is one command:

```sh
docker compose -f compose.yaml -f compose.postgres.yaml up
```

which starts PostgreSQL, runs the loader once, and then starts the service
against it. Refresh later without touching the service:

```sh
docker compose -f compose.yaml -f compose.postgres.yaml run --rm loader
```

In every case `openiban update` resolves the download links from the
publishers' pages at run time, refuses to replace existing data when a download
or parse fails, and swaps one country at a time so a broken registry cannot
take the others down with it.

**4. Import a file you already have**, per country, with no network:

```sh
openiban import -country DE -file blz-aktuell.txt -database-url postgres://...
```

Built in parsers cover the countries in the snapshot; every other country
accepts a generic CSV with a `bankCode,name,...` header. Details in the
[deployment guide](docs/deployment.md#importing-a-file-you-already-have).

To refresh the snapshot that ships inside the binary, write it back to its
source location and rebuild:

```sh
make update    # writes internal/embedded/snapshot.jsonl.gz
make build
```

The scheduled `data-refresh` workflow does exactly this every quarter and opens
a pull request with the result.

## Running it

### Container

```sh
docker run -p 8080:8080 ghcr.io/netzfabrikcom/iban-pizza:latest
```

The image is built `FROM scratch`, runs as uid 65534, and contains the binary
and certificate roots and nothing else.

### Binary

```sh
openiban serve                       # embedded snapshot, no configuration
openiban serve -data-file data.gz    # a snapshot refreshed without a rebuild
openiban serve -database-url postgres://...
```

Flags, each with an `OPENIBAN_`-prefixed environment variable equivalent:

| Flag | Default | Purpose |
|---|---|---|
| `-addr` | `:8080` | listen address |
| `-data-file` | | snapshot file, overrides the embedded one |
| `-database-url` | | PostgreSQL connection string |
| `-scheme-file` | | directory of EPC exports, enables the schemes block |
| `-cors-origins` | none | comma separated origins, or `*` |
| `-rate-limit` | 600 | requests per minute per client address, 0 disables |
| `-stale-after` | 120 days | age at which data is reported stale |
| `-base-url` | | public URL, for absolute logo links |
| `-log-format` | `json` | `json` or `text` |

### Refreshing the data

```sh
openiban update --write-snapshot internal/embedded/snapshot.jsonl.gz \
                --scheme-dir data/schemes
```

Download links are resolved at run time rather than hard coded, because the
Bundesbank path carries a content hash that changes with every quarterly
release. `--dry-run` downloads and parses without storing, and `--countries DE`
refreshes one registry.

## Development

```sh
make test     # go test ./... -race
make cover    # coverage summary
make lint     # gofmt and go vet
make vuln     # govulncheck
make build    # static binary into bin/
make update   # refresh the data and the embedded snapshot
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
internal/embedded/    The bank data snapshot compiled into the binary
cmd/openiban/         serve, update, snapshot, version
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
