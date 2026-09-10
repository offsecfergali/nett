# nett — build and quality targets.
# The whole toolchain is Go-only; no external recon binaries are required.

BINARY := nett
PKG    := ./cmd/nett
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/alieddine/nett/internal/version.Version=$(VERSION) \
           -X github.com/alieddine/nett/internal/version.Commit=$(COMMIT) \
           -X github.com/alieddine/nett/internal/version.Date=$(DATE)

PREFIX ?= /usr/local

.PHONY: all build install test race vet fmt check bench clean tidy

all: check build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install: build
	install -m 0755 $(BINARY) $(PREFIX)/bin/$(BINARY)

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

# check is the gate every milestone must pass before proceeding.
check: vet test race
	@echo "all checks passed"

bench:
	go test -bench . -benchmem ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
