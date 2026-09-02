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
curl -LO https://github.com/NETZFABRIKCOM/iban-pizza/releases/download/v$V/openiban_v${V}_linux_amd64
curl -LO https://github.com/NETZFABRIKCOM/iban-pizza/releases/download/v$V/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS

chmod +x openiban_v${V}_linux_amd64
./openiban_v${V}_linux_amd64 serve
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
# /etc/systemd/system/openiban.service
[Unit]
Description=iban.pizza
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/openiban serve -addr 127.0.0.1:8080
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
| `-addr` | `OPENIBAN_ADDR` | `:8080` | listen address |
| `-cors-origins` | `OPENIBAN_CORS_ORIGINS` | none | comma separated origins, or `*` |
| `-rate-limit` | `OPENIBAN_RATE_LIMIT` | 600 | requests per minute per client, 0 disables |
| `-data-file` | `OPENIBAN_DATA_FILE` | | snapshot file, overrides the embedded one |
| `-database-url` | `OPENIBAN_DATABASE_URL` | | PostgreSQL connection string |
| `-scheme-file` | `OPENIBAN_SCHEME_FILE` | | directory of EPC exports |
| `-stale-after` | `OPENIBAN_STALE_AFTER` | 120 days | age at which data is reported stale |
| `-base-url` | `OPENIBAN_BASE_URL` | | public URL, for absolute logo links |
| `-log-format` | `OPENIBAN_LOG_FORMAT` | `json` | `json` or `text` |
| `-log-level` | `OPENIBAN_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

Cross origin access is closed unless you open it. A browser frontend on
another origin needs `OPENIBAN_CORS_ORIGINS` set to that origin.

## 2. Docker and Compose

```sh
docker run -p 8080:8080 ghcr.io/netzfabrikcom/iban-pizza:0.1.0
```

The image is built `FROM scratch`: the binary, the certificate roots for
`openiban update`, and nothing else. No shell, no package manager, no libc. It
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
# The service alone. Publishes port 8080; change it with OPENIBAN_PORT.
docker compose up

# The service backed by PostgreSQL, with a loader that fills the database
# once and exits. Data can then be refreshed without restarting the service.
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
# refreshes the data each quarter, and the same Deployment reading from the
# database. Bring your own PostgreSQL; the manifest expects it as "postgres".
kubectl apply -f deploy/kubernetes/postgres.yaml
```

Change the connection string in the Secret before applying it. To load data
right away instead of waiting for the schedule:

```sh
kubectl create job --from=cronjob/iban-pizza-loader refresh-now
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

## Keeping the data current

The snapshot in a release is as old as that release. Three ways to get newer
data, in order of effort:

**Upgrade the release.** A scheduled workflow refreshes the snapshot every
quarter and opens a pull request; each release carries the data current at
its date.

**Refresh into a file** on the host, without touching the binary:

```sh
# The file does not have to exist; update creates it and its directory.
openiban update --write-snapshot /var/lib/openiban/data.gz
openiban serve  -data-file /var/lib/openiban/data.gz
```

`openiban update` fetches from the official registries directly, resolving the
current download links from the publishers' pages at run time. `--dry-run`
downloads and parses without storing, `--countries DE,AT` limits the run.

**Refresh into PostgreSQL**, for several instances sharing one dataset:

```sh
openiban update -database-url postgres://...
```

Where the data comes from, per country and with the licence position of each
source, is in [country-sources.md](country-sources.md).

## Adding the scheme register

The v2 response reports which EPC schemes an institution has joined, from the
EPC Register of Participants. That register is not compiled in. Fetch it once
and point the service at the directory:

```sh
openiban update --scheme-dir /var/lib/openiban/schemes
openiban serve  -scheme-file /var/lib/openiban/schemes
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
