# Build stage.
FROM golang:1.27-alpine AS build

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
        -o /iban-pizza \
        ./cmd/iban-pizza

# The runtime image has no shell to create a directory with, so the data
# directory is made here and copied across with its ownership.
RUN mkdir /data

# Runtime stage.
#
# scratch holds nothing but the binary: no shell, no package manager, no libc,
# so there is nothing to exploit and nothing to patch.
#
# The image contains no bank data. Each registry has its own terms of use, and
# most let a user download and use the data but not a project hand it on.
# "iban-pizza update" downloads it into /data on the machine that runs it.
FROM scratch

# Certificate roots, needed by "iban-pizza update" to reach the registries.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /iban-pizza /iban-pizza
# The MIT licence requires the notices to accompany copies of the software;
# the image is a copy.
COPY LICENSE NOTICE /

# Where "iban-pizza update" writes and "iban-pizza serve" reads. Mount a
# volume here so the data outlives the container. The directory is owned by
# the unprivileged user below, and a fresh named volume inherits that.
COPY --from=build --chown=65534:65534 /data /data
ENV IBAN_PIZZA_DATA_FILE=/data/bankdata.jsonl.gz \
    IBAN_PIZZA_SCHEME_FILE=/data/schemes
VOLUME /data

# An unprivileged, non existent user. scratch has no /etc/passwd, so the
# numeric form is the only one that works.
USER 65534:65534

EXPOSE 8080

# No shell exists here, so this is the exec form by necessity as well as by
# preference.
ENTRYPOINT ["/iban-pizza"]
CMD ["serve", "-addr", ":8080"]
