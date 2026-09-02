VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test lint vuln run update snapshot docker clean

all: test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/openiban ./cmd/openiban

test:
	go test ./... -race -count=1

cover:
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

run: build
	./bin/openiban serve -scheme-file data/schemes -log-format text

# Refresh the bank data and the embedded snapshot from the official registries.
update: build
	./bin/openiban update --write-snapshot internal/embedded/snapshot.jsonl.gz --scheme-dir data/schemes

docker:
	docker build --build-arg VERSION=$(VERSION) -t iban-pizza:$(VERSION) .

clean:
	rm -rf bin coverage.out
