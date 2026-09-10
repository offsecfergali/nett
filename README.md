# nett

**N**ative **E**numeration & **T**argeting **T**oolkit

A native Go reconnaissance framework. The **entire core recon stack is
implemented in Go** — it does **not** shell out to `subfinder`, `amass`,
`httpx`, `dnsx`, `naabu`, `massdns`, `nuclei`, `ffuf`, `gobuster`, `katana`,
`hakrawler`, `assetfinder`, `findomain`, `waybackurls`, `gau`, or any other
external recon program. After `go build ./...` the binary performs its work with
no other security tools installed.

> **Scope of this repository right now.** This is an in-progress build delivered
> in milestones (see `docs/ARCHITECTURE.md`). **Milestones 1–4 (CLI/config/
> logging, the SQLite asset-graph store, the scope engine, and the native DNS
> engine) are fully implemented and tested.** Every other milestone is
> designed in `docs/` and marked *planned* in the module catalog. The CLI only
> advertises capabilities that actually exist — run `nett capabilities` to see
> them.

## Quick start

👉 **[Read QUICKSTART.md](docs/QUICKSTART.md)** for a complete walkthrough: install, build, configure, and use nett.

## Install

Install as a normal command so `nett` works from any directory:

```bash
go install github.com/alieddine/nett/cmd/nett@latest
```

Or build from a checkout and install it onto your `PATH`:

```bash
go build -o nett ./cmd/nett
sudo install -m 0755 nett /usr/local/bin/nett
```

Either way, once installed:

```bash
nett --help
nett --version
nett modules
nett capabilities
nett config
```

works from any directory, with no `./` prefix required.

## Build & run

```bash
go build ./...              # builds with zero third-party dependencies
go build -o nett ./cmd/nett
./nett --help
./nett version
./nett capabilities       # only implemented capabilities are listed
./nett modules            # full catalog with per-module status + milestone
./nett modules dns        # detail for one module
./nett config             # the fully-resolved effective configuration
```

Or use the Makefile:

```bash
make build     # build with version metadata baked in
make install   # build and install to /usr/local/bin
make check     # go vet + go test + go test -race  (the milestone gate)
make bench     # benchmarks
```

## Configuration

Configuration is assembled with strict precedence:

```
built-in defaults  →  JSON config file  →  NETT_* env vars  →  CLI flags
```

Example:

```bash
nett --config nett.json --project acme --log-format json config
NETT_LOG_LEVEL=debug nett config
```

A config file is JSON (dependency-free). See `nett config` for the full shape
and `docs/DATA_MODEL.md` for the persisted data model.

## Design documents

- `docs/ARCHITECTURE.md` — repository layout, layering, concurrency model, event/
  pivot engine, persistence/resume/diff, and the milestone ledger.
- `docs/DATA_MODEL.md` — the SQLite asset-graph schema (nodes, edges, provenance,
  events, module runs, classifications, scores) and the Go domain types.
- `docs/MODULES.md` — per-module engineering notes (purpose, input, algorithm,
  concurrency, output, data model, failure behavior, security, tests).

## Safety

nett is a **reconnaissance** framework. It contains no exploitation, brute
forcing, credential stuffing, evasion, or payload delivery. Every network-active
module consults the scope engine before contacting a target, and aggressive
active scanning (port scans, range reverse-DNS, content discovery) requires
explicit activation and respects configured rate/timeout/concurrency limits.

## Container

```bash
docker build -t nett .    # minimal image: only Go build + the nett binary
docker run --rm nett capabilities
```

The image installs none of the tools listed above.

## Development

Every milestone ends by passing all four gates; work does not proceed past a
milestone with a failing gate:

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
```
