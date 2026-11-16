# Running iban.pizza

Three ways to run it, from simplest to most involved. In all three the bank
data is downloaded once from the official registries before the service can
resolve a bank, and each of them does that for you or in one command.

## In production, in one paragraph

Start it with the data step that belongs to your way of running it: `docker
compose up` runs a one shot download before the service, the Kubernetes
manifest does the same in an init container, and with the bare binary you run
`iban-pizza update` yourself. After that the service reads a local file, or a
database, and never contacts a publisher. To get newer data, run the download
again and restart the service.

## What is shipped and what is not

The binary and the image contain the IBAN structure registry for 70 countries,
the check digit methods and one parser per national bank registry. They
contain **no bank data**.

That is deliberate. Every registry is published under its own terms of use.
Most allow anyone to download and use the list, with the source named, and
several forbid passing it on altered, or passing it on at all. A project that
bundled the registries into a public image would be redistributing them; a
program that downloads them onto your machine at your request is not. So
`iban-pizza update` fetches each registry from its publisher, and the terms of
that publisher apply to you as the one who downloaded it. Read them before the
first download:

```sh
iban-pizza sources
```

The same information is stored with the data and reported by `/v2/data` for
every loaded source: publisher, URL, retrieval date, the date the publisher
put on the file, the terms and the source statement it asks for.

Until data is downloaded the service starts and works for what needs none: it
validates the structure and check digits of an IBAN and reports the bank code
as unchecked. `/readyz` answers 503 until at least one registry is loaded.

The EPC scheme register, which answers whether an institution takes part in
SEPA Credit Transfer, Instant Credit Transfer and Direct Debit, is downloaded
the same way. See "Adding the scheme register" below.

## 1. The binary

Download the release for your platform from the
[releases page](https://github.com/iban-pizza/iban-pizza/releases), verify
it, and run it:

```sh
# Linux, amd64. Substitute the version and your platform.
V=0.3.0
curl -LO https://github.com/iban-pizza/iban-pizza/releases/download/v$V/iban-pizza_v${V}_linux_amd64
curl -LO https://github.com/iban-pizza/iban-pizza/releases/download/v$V/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS

chmod +x iban-pizza_v${V}_linux_amd64
./iban-pizza_v${V}_linux_amd64 sources   # publishers and their terms
./iban-pizza_v${V}_linux_amd64 update    # downloads into data/bankdata.jsonl.gz
./iban-pizza_v${V}_linux_amd64 serve
```

That is the whole installation. The binary is static, so it runs on any Linux
without a runtime, and there are builds for macOS and Windows on the same page.

Check it:

```sh
curl http://localhost:8080/healthz
curl http://localhost:8080/v2/iban/DE89370400440532013000
```

### Running it as a service

A systemd unit that runs it as an unprivileged user with the hardening that
costs nothing. The data lives in the unit's state directory and is downloaded
before each start; the leading `-` lets the service start on the data it
already has when the publishers cannot be reached.

```ini
# /etc/systemd/system/iban-pizza.service
[Unit]
Description=iban.pizza
After=network-online.target
Wants=network-online.target

[Service]
StateDirectory=iban-pizza
Environment=IBAN_PIZZA_DATA_FILE=%S/iban-pizza/bankdata.jsonl.gz
ExecStartPre=-/usr/local/bin/iban-pizza update -allow-partial
ExecStart=/usr/local/bin/iban-pizza serve -addr 127.0.0.1:8080
Restart=on-failure
RestartSec=30
DynamicUser=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
```

Refresh the data with `systemctl restart iban-pizza`.

Put a reverse proxy in front for TLS. The service itself speaks plain HTTP and
does not terminate TLS by design; that job belongs to whatever already does it
on the host.

### Configuration

Everything is a flag with an environment variable of the same meaning, so a
unit file, a container and a shell all configure it the same way.

| Flag | Variable | Default | Purpose |
|---|---|---|---|
| `-addr` | `IBAN_PIZZA_ADDR` | `:8080` | listen address |
| `-cors-origins` | `IBAN_PIZZA_CORS_ORIGINS` | none | comma separated origins, or `*` |
| `-rate-limit` | `IBAN_PIZZA_RATE_LIMIT` | 600 | requests per minute per client, 0 disables |
| `-trusted-proxy-header` | `IBAN_PIZZA_TRUSTED_PROXY_HEADER` | none | header carrying the client address behind a proxy you control |
| `-data-file` | `IBAN_PIZZA_DATA_FILE` | `data/bankdata.jsonl.gz` | data file written by `iban-pizza update` |
| `-database-url` | `IBAN_PIZZA_DATABASE_URL` | | PostgreSQL connection string |
| `-scheme-file` | `IBAN_PIZZA_SCHEME_FILE` | | directory of EPC exports; `update` downloads into it |
| `-stale-after` | `IBAN_PIZZA_STALE_AFTER` | 120 days | age at which data is reported stale |
| `-base-url` | `IBAN_PIZZA_BASE_URL` | | public URL, for absolute logo links |
| `-log-format` | `IBAN_PIZZA_LOG_FORMAT` | `json` | `json` or `text` |
| `-log-level` | `IBAN_PIZZA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Cross origin access is closed unless you open it. A browser frontend on
another origin needs `IBAN_PIZZA_CORS_ORIGINS` set to that origin.

## 2. Docker and Compose

```sh
docker volume create bankdata
docker run --rm -v bankdata:/data ghcr.io/iban-pizza/iban-pizza:0.3.0 update
docker run -p 8080:8080 -v bankdata:/data ghcr.io/iban-pizza/iban-pizza:0.3.0
```

The first command downloads the registries into the volume, the second serves
from it. The image sets `IBAN_PIZZA_DATA_FILE` and `IBAN_PIZZA_SCHEME_FILE` to
paths under `/data`, so both commands agree without any flag.

The image is built `FROM scratch`: the binary, the certificate roots for
`iban-pizza update`, and nothing else. No shell, no package manager, no libc,
no bank data. It runs as uid 65534, which owns `/data`.

### While the repository is private

A private repository publishes a private image. Pulling it needs a GitHub
token with the `read:packages` scope, and a `docker login`:

```sh
gh auth refresh -h github.com -s read:packages
gh auth token | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
docker pull ghcr.io/iban-pizza/iban-pizza:0.3.0
```

The `repo` scope alone is not enough, even for a member of the organisation;
the pull is refused with `denied` until `read:packages` is present. The
released binaries have no such restriction for anyone who can see the
repository, which makes the binary the simpler start while the repository is
private. Once it is public, the image pulls without any login.

With Compose, the repository carries two files:

```sh
# The service alone. A one shot "data" service downloads the registries into
# a volume first. Publishes port 8080; change it with IBAN_PIZZA_PORT.
docker compose up

# The service backed by PostgreSQL. The same "data" service downloads into
# the database instead.
docker compose -f compose.yaml -f compose.postgres.yaml up
```

Refresh the data later by running the data service again and restarting the
service, which reads its data file at start:

```sh
docker compose run --rm data
docker compose restart iban
```

With PostgreSQL no restart is needed; the service reads the database per
request.

## 3. Kubernetes

Two manifests in `deploy/kubernetes/`, applied with plain `kubectl` and no
extra tooling:

```sh
# Standalone: a Deployment with two replicas, an init container that
# downloads the registries into the pod's volume, readiness and liveness
# probes, a restricted security context, and a Service.
kubectl apply -f deploy/kubernetes/deployment.yaml

# Backed by PostgreSQL: a Secret for the connection string, a CronJob that
# downloads the registries into the database, and the same Deployment reading
# from it. Bring your own PostgreSQL; the manifest expects it as "postgres".
kubectl apply -f deploy/kubernetes/postgres.yaml
```

Standalone, every pod downloads its own copy when it starts, so the namespace
has to allow that egress, and `kubectl rollout restart deployment/iban-pizza`
refreshes the data. With PostgreSQL only the CronJob needs egress. Change the
connection string in the Secret before applying it, and load data right away
rather than waiting for the schedule:

```sh
kubectl create job --from=cronjob/iban-pizza-loader sync-now
```

The probes are chosen so that old data does not take a pod out of rotation:
`/readyz` fails only when no bank data is loaded at all, and `/healthz` stays
200 while the service can answer, reporting staleness in the body instead.

## Knowing what data you are answering from

`/v2/data` reports the provenance of everything loaded: which registry for
which country, the URL it was fetched from, when, the date the publisher put
on the file where it states one, how many records, whether it counts as stale,
and the publisher's terms with the source statement it asks for. It also reports whether the EPC scheme register is
loaded and the participant count per scheme.

```sh
curl http://localhost:8080/v2/data
```

`/healthz` carries only the summary (`status`, `records`, `stale`), because
orchestrators poll it and it should stay small. Every v2 answer also stamps
the retrieval date of each source it used as `dataAsOf`, so a single response
can be dated without a second request.

## Security model for data

Three facts make the model simple:

- **`serve` never downloads.** It reads a file or a database. A host that runs
  only the service can deny all outbound traffic and the service does not
  notice. Do that where you can.
- **Only `update` contacts a publisher.** It can run on the same host before
  the service starts, which is what the Compose file and the standalone
  Kubernetes manifest do, or somewhere else entirely: download on a machine
  that may reach the publishers and carry the file across.

  ```sh
  iban-pizza update -data-file bankdata.jsonl.gz
  iban-pizza import -snapshot bankdata.jsonl.gz -database-url postgres://...
  ```

- **`import` covers the exceptions** without any network: a licensed file,
  an air gapped host, a registry site that is down.

What `update` defends against: TLS with certificate verification, a hard size
cap per download, parse before store, and refusal to replace existing data
when a download or parse fails. What it cannot defend against is a publisher
shipping wrong data, because none of the registries sign their files. If that
matters to you, download with `-dry-run` first, or into a file you inspect,
and promote it with `import -snapshot` once you are satisfied.

## Behind iban.pizza: the container on any host, no open port

iban.pizza itself is a Next.js site on Vercel; the domain stays at Cloudflare
for DNS and points at Vercel. The site rewrites the API paths (`/v2/*`,
`/validate/*`, `/calculate/*`, `/countries`, `/healthz`, `/readyz`,
`/openapi.yaml`) to wherever the service runs, so the page and the API share
one origin and the demo on the page needs no CORS.

The service runs on any host behind a Cloudflare Tunnel.
`deploy/cloudflare-tunnel/` holds a Compose file that runs the container next
to `cloudflared`, which opens an outbound connection to Cloudflare and needs
no inbound port on the host:

```sh
TUNNEL_TOKEN=... docker compose -f deploy/cloudflare-tunnel/compose.yaml up -d
```

Create the tunnel once in the Cloudflare dashboard, point its public hostname
(for example `api.iban.pizza`) at `http://iban-pizza:8080`, and set that
hostname as `API_ORIGIN` on the Vercel project. Behind a proxy every request
arrives from the proxy's address, so the service is told which header carries
the real client with `-trusted-proxy-header`; the Compose file sets it. Do not
set it when clients connect directly, because anyone can send the header
themselves.

## Keeping the data current

The data is as old as your last download. `/v2/data` and `/healthz` report its
age, and the service logs a warning at start once it passes `-stale-after`.
The registries change at different rates, from daily to quarterly; a weekly
download is a reasonable default.

**Into the data file**, then restart the service, which reads it at start:

```sh
iban-pizza update
```

`iban-pizza update` fetches from the official registries directly, resolving
the current download links from the publishers' pages at run time. `-dry-run`
downloads and parses without storing, `-countries DE,AT` limits the run. A
registry that fails to download or parse leaves its previous data in place and
makes the command exit with an error; `-allow-partial` turns that into a
warning as long as at least one registry succeeded, which is what an
unattended first start wants.

**Into PostgreSQL**, for several instances sharing one dataset. No restart is
needed:

```sh
iban-pizza update -database-url postgres://...
```

`update` always fetches from the publishers. It never reads a local file. For
that there is `import`.

Publishers change their files without notice. This repository runs a weekly
`check-sources` workflow that downloads and parses every registry and stores
nothing, so a broken parser shows up there, and a fix reaches you as a new
release of the program, not of the data.

## Importing a file you already have

`iban-pizza import` loads one registry file per country into any of the three
stores, without contacting a publisher. Use it when the host has no outbound
network, when a registry's site is down, or for a registry you licensed and
may not redistribute, such as UK sort codes, the Dutch BIC list or the Irish
sort code directory.

```sh
# A file in a registry's own format, parsed by the built in parser.
iban-pizza import -country DE -file blz-aktuell.txt -database-url postgres://...

# Into the data file instead of a database.
iban-pizza import -country AT -file sepa-zv-vz_gesamt.csv -data-file /var/lib/iban-pizza/bankdata.jsonl.gz

# Check what a file would produce before touching anything.
iban-pizza import -country CZ -file kody_bank_CR.csv --dry-run
```

`-snapshot` copies every source of a data file written by `update` instead of
one registry file. Each import replaces that source's records and leaves every
other country untouched, so countries can be loaded one at a time and in any order. The
file's modification time is recorded as the retrieval time, so an old file is
reported as old by `/v2/data` rather than looking fresh because it was
imported today.

### The generic format

Countries without a built in parser accept a delimited file with a header row:

```
bankCode,name,bic,zip,city,shortName,checkAlgo,country
```

Only `bankCode` and `name` are required. Column order does not matter, the
separator (`,` or `;`) is detected from the header, and names are matched
loosely: `Bank Code`, `bank_code` and `bankCode` are the same column, as are
`SWIFT` and `bic`, `PLZ` and `zip`, `Ort` and `city`. A `country` column
overrides `-country` per row, so one file may carry several countries.

```sh
iban-pizza import -country GB -file sortcodes.csv -source "Licensed sort code directory" \
                -database-url postgres://...
```

`-format generic` forces this layout for a country that does have a built in
parser, for example to load a hand maintained list in place of the registry.

Where the data comes from, per country and with the terms of each publisher,
is in [country-sources.md](country-sources.md).

## Adding the scheme register

The v2 response reports which EPC schemes an institution has joined, from the
EPC Register of Participants: SEPA Credit Transfer, Instant Credit Transfer,
Direct Debit core and B2B, One-Leg Out Instant and Verification of Payee. Like
the bank data, the register is downloaded and not shipped. Fetch it and point
the service at the directory:

```sh
iban-pizza update -scheme-dir /var/lib/iban-pizza/schemes
iban-pizza serve  -scheme-file /var/lib/iban-pizza/schemes
```

Setting `IBAN_PIZZA_SCHEME_FILE` covers both commands, and the container image
sets it already, so there the register is part of every `update`.

The register is keyed by BIC. An institution whose registry entry carries no
BIC therefore gets no scheme answer, and the statuses describe the institution,
not what a particular account can do.

Without it, v2 omits the `schemes` block and everything else works unchanged.

## Verifying a download

Every release ships a `SHA256SUMS` file covering all binaries. Check it before
running anything:

```sh
sha256sum --check --ignore-missing SHA256SUMS
```

Container images carry an SBOM and a provenance attestation. Signing with
cosign starts when the repository becomes public, because keyless signing
writes to a public transparency log that would name the repository.

## How releases are built

Pushing a tag builds everything. `v1.2.3` runs the test suite, builds static
binaries for Linux, macOS and Windows on amd64 and arm64, writes `SHA256SUMS`,
publishes a multi architecture image to GHCR, and creates the GitHub release
with those files attached. The workflow is `.github/workflows/release.yml`.

```sh
git tag v0.3.0
git push origin v0.3.0
```
