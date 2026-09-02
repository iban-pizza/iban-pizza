# Running iban.pizza

Three ways to run it, from simplest to most involved. All three answer
immediately, because the bank data is compiled into the binary and the image.
Nothing has to be downloaded first.

## In production, in one paragraph

Pull the image, start it, done. There is no import step, no first run
initialisation and no data download. The bank data is inside the image, and
the service answers correctly from the first request even with no network
access at all. To get newer data later, pull a newer image tag; releases are
cut each quarter with the current registries. That is the whole operating
model for the standalone deployment, and it is the one to choose unless you
have a specific reason for the PostgreSQL variant below.

The only case with a loader is PostgreSQL, and it is opt in: several instances
sharing one centrally maintained dataset, refreshed without redeploying the
service. Standalone never runs one.

## What is shipped and what is not

The binary and the image contain a snapshot of the bank registries: Germany,
Austria and the Czech Republic today, about 110 KB compressed, plus the IBAN
structure registry for 70 countries. The snapshot's retrieval date is reported
by `/v2/data` and stamped into every v2 answer as `dataAsOf`.

The EPC scheme register is **not** compiled in. Without it the v2 response
simply omits the `schemes` block; nothing breaks. Section "Adding the scheme
register" below shows how to add it.

## 1. The binary

Download the release for your platform from the
[releases page](https://github.com/NETZFABRIKCOM/iban-pizza/releases), verify
it, and run it:

```sh
# Linux, amd64. Substitute the version and your platform.
V=0.1.0
curl -LO https://github.com/NETZFABRIKCOM/iban-pizza/releases/download/v$V/iban-pizza_v${V}_linux_amd64
curl -LO https://github.com/NETZFABRIKCOM/iban-pizza/releases/download/v$V/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS

chmod +x iban-pizza_v${V}_linux_amd64
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
costs nothing:

```ini
# /etc/systemd/system/iban-pizza.service
[Unit]
Description=iban.pizza
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/iban-pizza serve -addr 127.0.0.1:8080
Restart=on-failure
DynamicUser=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
```

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
| `-data-file` | `IBAN_PIZZA_DATA_FILE` | | snapshot file, overrides the embedded one |
| `-database-url` | `IBAN_PIZZA_DATABASE_URL` | | PostgreSQL connection string |
| `-scheme-file` | `IBAN_PIZZA_SCHEME_FILE` | | directory of EPC exports |
| `-stale-after` | `IBAN_PIZZA_STALE_AFTER` | 120 days | age at which data is reported stale |
| `-base-url` | `IBAN_PIZZA_BASE_URL` | | public URL, for absolute logo links |
| `-log-format` | `IBAN_PIZZA_LOG_FORMAT` | `json` | `json` or `text` |
| `-log-level` | `IBAN_PIZZA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Cross origin access is closed unless you open it. A browser frontend on
another origin needs `IBAN_PIZZA_CORS_ORIGINS` set to that origin.

## 2. Docker and Compose

```sh
docker run -p 8080:8080 ghcr.io/netzfabrikcom/iban-pizza:0.1.0
```

The image is built `FROM scratch`: the binary, the certificate roots for
`iban-pizza update`, and nothing else. No shell, no package manager, no libc. It
runs as uid 65534.

### While the repository is private

A private repository publishes a private image. Pulling it needs a GitHub
token with the `read:packages` scope, and a `docker login`:

```sh
gh auth refresh -h github.com -s read:packages
gh auth token | docker login ghcr.io -u YOUR_GITHUB_USER --password-stdin
docker pull ghcr.io/netzfabrikcom/iban-pizza:0.1.0
```

The `repo` scope alone is not enough, even for a member of the organisation;
the pull is refused with `denied` until `read:packages` is present. The
released binaries have no such restriction for anyone who can see the
repository, which makes the binary the simpler start while the repository is
private. Once it is public, the image pulls without any login.

With Compose, the repository carries two files:

```sh
# The service alone. Publishes port 8080; change it with IBAN_PIZZA_PORT.
docker compose up

# The service backed by PostgreSQL. The loader copies the snapshot from the
# image into the database and exits; it never contacts a publisher.
docker compose -f compose.yaml -f compose.postgres.yaml up
docker compose -f compose.yaml -f compose.postgres.yaml run --rm loader
```

## 3. Kubernetes

Two manifests in `deploy/kubernetes/`, applied with plain `kubectl` and no
extra tooling:

```sh
# Standalone: a Deployment with two replicas, readiness and liveness probes,
# a restricted security context, and a Service.
kubectl apply -f deploy/kubernetes/deployment.yaml

# Backed by PostgreSQL: a Secret for the connection string, a CronJob that
# copies the image's snapshot into the database, and the same Deployment
# reading from it. Bring your own PostgreSQL; the manifest expects it as
# "postgres".
kubectl apply -f deploy/kubernetes/postgres.yaml
```

Change the connection string in the Secret before applying it. To load data
right away, and after every image rollout:

```sh
kubectl create job --from=cronjob/iban-pizza-loader sync-now
```

The probes are chosen so that old data does not take a pod out of rotation:
`/readyz` fails only when no bank data is loaded at all, and `/healthz` stays
200 while the service can answer, reporting staleness in the body instead.

## Knowing what data you are answering from

`/v2/data` reports the provenance of everything loaded: which registry for
which country, the URL it was fetched from, when, how many records, and
whether it counts as stale. It also reports whether the EPC scheme register is
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

- **`serve` never downloads.** It reads the embedded snapshot, a file, or a
  database. A production host can deny all outbound traffic and the service
  does not notice. Do that where you can.
- **Only `update` contacts a publisher**, and it belongs in a pipeline, not
  on a production host. The `data-refresh` workflow runs it in CI, runs the
  test suite against what came back, and opens a pull request that shows the
  record count per country. A person reviews that before it becomes part of a
  release. Production then receives data as a reviewed artifact.
- **`import` covers the exceptions** without any network: a licensed file,
  an air gapped host, a registry site that is down.

What `update` does defend against, for the pipeline where it runs: TLS with
certificate verification, a hard size cap per download, parse before store,
and refusal to replace existing data when a download or parse fails. What it
cannot defend against is a publisher shipping wrong data, because none of the
registries sign their files. That is why the pull request review exists.

For the PostgreSQL variant this means the loader copies the snapshot out of
the image rather than fetching:

```sh
iban-pizza import -snapshot embedded -database-url postgres://...
```

That is what the Compose file and the Kubernetes CronJob do. Run it after
every image rollout and the database matches the image; nothing in
production ever talks to a registry. Fetching in cluster with `update` is
possible and documented in both manifests, and it is a deliberate trade of
egress for freshness rather than the default.

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

The snapshot in a release is as old as that release. Three ways to get newer
data, in order of effort:

**Upgrade the release.** A scheduled workflow refreshes the snapshot every
quarter and opens a pull request; each release carries the data current at
its date.

**Refresh into a file** on the host, without touching the binary:

```sh
# The file does not have to exist; update creates it and its directory.
iban-pizza update --write-snapshot /var/lib/iban-pizza/data.gz
iban-pizza serve  -data-file /var/lib/iban-pizza/data.gz
```

`iban-pizza update` fetches from the official registries directly, resolving the
current download links from the publishers' pages at run time. `--dry-run`
downloads and parses without storing, `--countries DE,AT` limits the run.

**Refresh into PostgreSQL**, for several instances sharing one dataset:

```sh
iban-pizza update -database-url postgres://...
```

`update` always fetches from the publishers. `--countries DE,AT` limits which
registries it fetches, but it never reads a local file. For that there is
`import`.

## Importing a file you already have

`iban-pizza import` loads one registry file per country into any of the three
stores, without contacting a publisher. Use it when the host has no outbound
network, when a registry's site is down, or for a registry you licensed and
may not redistribute, such as UK sort codes or the French FIB.

```sh
# A file in a registry's own format, parsed by the built in parser.
iban-pizza import -country DE -file blz-aktuell.txt -database-url postgres://...

# Into a snapshot file instead of a database.
iban-pizza import -country AT -file sepa-zv-vz_gesamt.csv --write-snapshot /var/lib/iban-pizza/data.gz

# Check what a file would produce before touching anything.
iban-pizza import -country CZ -file kody_bank_CR.csv --dry-run
```

`-snapshot` copies every source of a snapshot instead of one file: a path, or
`embedded` for the snapshot built into the binary. Each import replaces that
source's records and leaves every other country
untouched, so countries can be loaded one at a time and in any order. The
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

Where the data comes from, per country and with the licence position of each
source, is in [country-sources.md](country-sources.md).

## Adding the scheme register

The v2 response reports which EPC schemes an institution has joined, from the
EPC Register of Participants. That register is not compiled in. Fetch it once
and point the service at the directory:

```sh
iban-pizza update --scheme-dir /var/lib/iban-pizza/schemes
iban-pizza serve  -scheme-file /var/lib/iban-pizza/schemes
```

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
git tag v0.2.0
git push origin v0.2.0
```
