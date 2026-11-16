VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test lint vuln run update snapshot docker clean

all: test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/iban-pizza ./cmd/iban-pizza

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
	./bin/iban-pizza serve -scheme-file data/schemes -log-format text

# Download the bank data and the scheme register from the official publishers
# into ./data, which is not checked in. Each publisher's terms apply; list them
# with "./bin/iban-pizza sources".
update: build
	./bin/iban-pizza update -scheme-dir data/schemes

docker:
	docker build --build-arg VERSION=$(VERSION) -t iban-pizza:$(VERSION) .

clean:
	rm -rf bin coverage.out
