# nett

**N**ative **E**numeration & **T**argeting **T**oolkit

A native Go reconnaissance framework. The **entire core reconnaissance stack is implemented in Go** — it does **not** shell out to `subfinder`, `amass`, `httpx`, `dnsx`, `naabu`, `massdns`, `nuclei`, `ffuf`, `gobuster`, `katana`, `hakrawler`, `assetfinder`, `findomain`, `waybackurls`, `gau`, or any other external reconnaissance program.

After `go build ./...`, the resulting binary performs its work without requiring any other security tools to be installed.

> **Repository status:** nett is an in-progress project delivered in milestones (see [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)).
>
> **Milestones 1–5, 9, and part of 7** — CLI/configuration/logging, the SQLite asset-graph store, the scope engine, the native DNS engine, Certificate Transparency + subdomain discovery, DNS brute force/permutation, and the native TCP port scanner — are fully implemented and tested. `nett scan` is live with `-d`, `-brute`, `-tcp`, and `-p`.
>
> All remaining milestones are documented in `docs/` and marked **planned** in the module catalog. The CLI only advertises capabilities that are actually implemented. Run `nett capabilities` to see the current capabilities.

## Quick start

👉 See [`docs/QUICKSTART.md`](docs/QUICKSTART.md) for the complete walkthrough covering installation, building, configuration, and usage.

## Install

Install directly with Go:

```bash
go install github.com/offsecfergali/nett/cmd/nett@latest
```

Or build from a checkout and install the binary onto your `PATH`:

```bash
go build -o nett ./cmd/nett
sudo install -m 0755 nett /usr/local/bin/nett
```

Once installed, `nett` can be run from any directory:

```bash
nett --help
nett --version
nett modules
nett capabilities
nett config
```

## Build & run

Build the complete project:

```bash
go build ./...
```

Build the CLI binary:

```bash
go build -o nett ./cmd/nett
```

Run it directly:

```bash
./nett --help
./nett version
./nett capabilities
./nett modules
./nett modules dns
./nett config
```

### Makefile

If you have `make` available:

```bash
make build
make install
make check
make bench
```

Commands:

* `make build` — builds nett with version metadata.
* `make install` — builds and installs nett to `/usr/local/bin`.
* `make check` — runs `go vet`, tests, and the race detector.
* `make bench` — runs benchmarks.

## Scanning

`nett scan <target> [flags]` runs reconnaissance against a domain or IP, gated
by the scope engine and persisted to the project's SQLite store:

```bash
nett scan example.com -d                    # passive subdomain discovery (CT + DNS validation)
nett scan example.com -brute words.txt      # DNS brute force with common-affix permutations
nett scan example.com -tcp                  # TCP connect scan of common ports
nett scan example.com -tcp 1-1000           # TCP connect scan of a port range
nett scan example.com -tcp 22,80,443        # TCP connect scan of a port list
nett scan example.com -p                    # TCP connect scan of every port, 1-65535
nett scan example.com -d -brute words.txt -p  # combine actions in one run
```

`-udp`, `-f` (fingerprinting), `-en` (port/service enumeration), and `-dir`
(content discovery) are part of the design (see `docs/ARCHITECTURE.md`) but
not implemented yet; passing them fails with an explicit error naming the
milestone that will add them, rather than silently doing nothing.

## Configuration

Configuration is assembled using strict precedence:

```text
built-in defaults
        ↓
JSON config file
        ↓
NETT_* environment variables
        ↓
CLI flags
```

For example:

```bash
nett --config nett.json --project acme --log-format json config
```

Set individual configuration values through environment variables:

```bash
NETT_LOG_LEVEL=debug nett config
```

Configuration files use JSON and require no additional configuration dependencies.

Run:

```bash
nett config
```

to inspect the fully resolved effective configuration.

See [`docs/DATA_MODEL.md`](docs/DATA_MODEL.md) for details about persisted configuration and the asset-graph data model.

## Modules & capabilities

View the complete module catalog:

```bash
nett modules
```

Inspect a specific module:

```bash
nett modules dns
```

View only capabilities that are currently implemented:

```bash
nett capabilities
```

This distinction is intentional: planned modules are documented for development purposes but are not presented by the CLI as available functionality.

## Architecture

The project is designed around a native Go reconnaissance pipeline with persistent state and controlled target scope.

### Design documents

* [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — repository layout, layering, concurrency model, event/pivot engine, persistence, resume/diff behavior, and milestone ledger.
* [`docs/DATA_MODEL.md`](docs/DATA_MODEL.md) — SQLite asset-graph schema, nodes, edges, provenance, events, module runs, classifications, scores, and Go domain types.
* [`docs/MODULES.md`](docs/MODULES.md) — per-module engineering notes covering purpose, inputs, algorithms, concurrency, output, data model, failure behavior, security, and tests.
* [`docs/QUICKSTART.md`](docs/QUICKSTART.md) — installation, configuration, and first-use walkthrough.

## Safety & scope

nett is a **reconnaissance framework**.

It does **not** provide:

* exploitation
* brute forcing
* credential stuffing
* evasion
* payload delivery

Every network-active module consults the scope engine before contacting a target.

Aggressive active-scanning capabilities — such as port scanning, range reverse-DNS, and content discovery — require explicit activation and must respect configured rate, timeout, and concurrency limits.

Only scan systems and infrastructure that you are authorized to assess.

## Runtime dependencies

The core reconnaissance functionality is implemented directly in Go.

nett does not require external reconnaissance tools such as:

```text
subfinder
amass
httpx
dnsx
naabu
massdns
nuclei
ffuf
gobuster
katana
hakrawler
assetfinder
findomain
waybackurls
gau
```

The project uses Go dependencies where appropriate, but it does not depend on these external security programs being installed on the host.

## Container

Build the minimal container image:

```bash
docker build -t nett .
```

Run nett:

```bash
docker run --rm nett capabilities
```

The container image does not install the external reconnaissance tools listed above.

## Development

Each milestone must pass all four engineering gates before work proceeds to the next milestone:

```bash
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

You can run the complete milestone gate with:

```bash
make check
```

## Project status

nett is actively being developed in milestones.

Implemented milestones are exposed through the CLI and covered by tests. Future functionality is specified in the architecture and module documentation before implementation begins.

Check the current state with:

```bash
nett capabilities
nett modules
```

## License

See the repository license file for the project's licensing terms.
