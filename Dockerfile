# Build stage.
FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies are resolved before the sources are copied so that a code change
# does not invalidate the module cache layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
# CGO is off so the result is a static binary that runs on an empty base image.
# The symbol table and DWARF data are stripped because nothing debugs inside
# the container, and trimpath keeps build paths out of the binary.
RUN CGO_ENABLED=0 go build \
        -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /openiban \
        ./cmd/openiban

# Runtime stage.
#
# scratch holds nothing but the binary: no shell, no package manager, no libc,
# so there is nothing to exploit and nothing to patch. The bank data is
# compiled into the binary, which is what makes this possible.
FROM scratch

# Certificate roots, needed by "openiban update" to reach the registries.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /openiban /openiban

# An unprivileged, non existent user. scratch has no /etc/passwd, so the
# numeric form is the only one that works.
USER 65534:65534

EXPOSE 8080

# No shell exists here, so this is the exec form by necessity as well as by
# preference.
ENTRYPOINT ["/openiban"]
CMD ["serve", "-addr", ":8080"]
