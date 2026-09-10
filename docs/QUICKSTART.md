# nett — Quickstart: Install to First Scan

This guide walks you through installing nett and running your first reconnaissance. All you need is Go 1.24+. No other security tools required.

---

## 1. Prerequisites

- **Go 1.24+** — [download](https://golang.org/dl)
- **git** (optional, for cloning the repo)
- **A target domain** (e.g. `example.com`)

Verify Go is installed:

```bash
go version
```

---

## 2. Installation

### Option A: Clone and build locally

```bash
git clone https://github.com/alieddine/nett.git
cd nett
go build -o nett ./cmd/nett
```

The binary `./nett` is now ready to use.

### Option B: Install to `$GOPATH/bin`

```bash
go install github.com/alieddine/nett/cmd/nett@latest
```

Then use it as `nett` from anywhere (make sure `$GOPATH/bin` is in your `$PATH`).

### Option C: Docker

```bash
docker build -t nett .
docker run --rm nett --help
```

---

## 3. Verify the installation

```bash
./nett version
```

Output:

```
nett 0.1.0-dev
  commit:  <commit>
  built:   <timestamp>
  go:      go1.26.3
  platform: darwin/arm64
```

---

## 4. Explore what's implemented

The tool is honest about what actually works — only implemented capabilities are shown.

### See implemented capabilities

```bash
./nett capabilities
```

**Output (Milestone 1):**

```
Implemented capabilities:

CLI
 ├── capabilities
 ├── config
 ├── help
 ├── modules
 └── version

Config
 ├── defaults
 ├── env-overrides
 ├── flag-overrides
 ├── json-file
 └── validation

Logging
 ├── component-scoping
 ├── file-output
 ├── json
 ├── levels
 └── text
```

Notice: **DNS, HTTP, crawling, and API discovery are not here yet** (they are planned for later milestones). The tool never claims a capability it doesn't have.

### See the full module roadmap

```bash
./nett modules
```

**Output:**

```
NAME         MILE   STATUS       DESCRIPTION
core         M1     implemented  CLI, layered configuration, and structured logging
store        M2     planned      SQLite asset-graph store and data model
scope        M3     planned      Scope engine (domain/host/IP/CIDR/URL/path, allow/deny)
dns          M4     planned      Native DNS resolver with caching and wildcard detection
subdomain    M5     planned      Passive subdomain enumeration via provider interface
ct           M5     planned      Certificate Transparency discovery with native x509 parsing
...
```

### Inspect a specific module

```bash
./nett modules dns
```

**Output:**

```
Module:     dns
Summary:    Native DNS resolver with caching and wildcard detection
Milestone:  M4
Status:     planned
Active:     false (makes network requests; enforces scope before contacting targets)
```

---

## 5. Configuration

### View the effective configuration

```bash
./nett config
```

Outputs the full JSON:

```json
{
  "project": "default",
  "data_dir": "./data",
  "output_dir": "./output",
  "logging": {
    "level": "info",
    "format": "text",
    "file": ""
  },
  "concurrency": {
    "global": 50,
    "dns": 25,
    "http": 25,
    "port": 100
  },
  "http": {
    "timeout_seconds": 10,
    "retries": 2,
    "rate_limit_per_sec": 20,
    "user_agent": "nett/0.1 (+https://github.com/alieddine/nett)",
    "follow_redirects": true,
    "max_redirects": 10
  },
  "dns": {
    "resolvers": ["1.1.1.1:53", "8.8.8.8:53"],
    "timeout_seconds": 5,
    "retries": 2,
    "protocol": "udp",
    "rate_limit_per_sec": 50
  },
  "scope": {
    "include": null,
    "exclude": null,
    "wildcards": null
  },
  "modules": {}
}
```

### Override configuration

Configuration is assembled with strict precedence:

```
built-in defaults  →  JSON config file  →  NETT_* env  →  CLI flags
```

#### Via CLI flags

```bash
./nett --project acme --log-level debug config
```

#### Via environment variables

```bash
export NETT_PROJECT=acme
export NETT_LOG_LEVEL=debug
export NETT_LOG_FORMAT=json
./nett config
```

Common env vars:

```bash
NETT_PROJECT              # project name
NETT_LOG_LEVEL            # debug|info|warn|error
NETT_LOG_FORMAT           # text|json
NETT_LOG_FILE             # path to log file
NETT_DATA_DIR             # where to store the database
NETT_OUTPUT_DIR           # where to write exports
NETT_CONCURRENCY          # global worker pool size
NETT_HTTP_TIMEOUT         # seconds
NETT_HTTP_RETRIES         # count
NETT_HTTP_RATELIMIT       # requests/sec
NETT_DNS_RESOLVERS        # comma-separated (9.9.9.9:53,8.8.8.8:53)
NETT_DNS_TIMEOUT          # seconds
NETT_DNS_PROTOCOL         # udp|tcp|doh|dot
NETT_SCOPE_INCLUDE        # comma-separated domains/IPs
NETT_SCOPE_EXCLUDE        # comma-separated domains/IPs
```

#### Via JSON config file

Create `nett.json`:

```json
{
  "project": "acme",
  "data_dir": "./data/acme",
  "logging": {
    "level": "debug",
    "format": "json",
    "file": "./logs/nett.log"
  },
  "concurrency": {
    "global": 100,
    "dns": 50,
    "http": 50
  },
  "dns": {
    "protocol": "tcp",
    "resolvers": ["1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"]
  },
  "scope": {
    "include": ["example.com", "*.example.com"],
    "exclude": ["dev.example.com"]
  }
}
```

Then run:

```bash
./nett --config nett.json config
```

---

## 6. Logging

### Levels

```bash
# Quiet (errors only)
./nett -q version

# Normal (info level)
./nett version

# Verbose (debug)
./nett -v version

# Or explicit
./nett --log-level debug version
```

### Formats

Text (default, human-readable):

```bash
./nett --log-format text version
```

JSON (structured, easy to parse/filter):

```bash
./nett --log-format json version
```

### File output

```bash
./nett --log-level debug --log-format json --config <(cat <<'EOF'
{
  "logging": {
    "level": "debug",
    "format": "json",
    "file": "./logs/nett.log"
  }
}
EOF
) config
```

---

## 7. Available commands (M1)

Currently implemented:

| Command | Purpose | Example |
|---------|---------|---------|
| `version` | Print version and build metadata | `./nett version` |
| `capabilities` | List implemented capabilities (truthful, never overstates) | `./nett capabilities` |
| `modules` | List all modules with status (implemented/planned) or show one module's detail | `./nett modules` or `./nett modules dns` |
| `config` | Print the resolved effective configuration (JSON) | `./nett config` |

**Intentionally absent (not yet implemented):**
- `scan` — added in M7 (HTTP engine)
- `pipeline` — added in M21 (event/pivot engine)
- `resume` — added in M23 (incremental reconnaissance)

The CLI never lists a command it cannot run.

---

## 8. Next steps — Milestone 2 roadmap

Once **M2 (SQLite store)** lands, you'll be able to:

```bash
./nett scan --target example.com --project acme
```

This will:
- Initialize an SQLite database in `./data/acme/`
- Store discoveries in an asset graph (domains, hosts, IPs, relationships)
- Record provenance (why the tool believes each asset exists)

Once **M3–M7** land (scope, DNS, CT, subdomain, HTTP engines), the pipeline will:

```bash
./nett pipeline --target example.com --project acme
```

Automatically:
1. Validate the target is in scope
2. Enumerate subdomains (passive providers + CT)
3. Resolve hostnames to IPs
4. Probe for HTTP/HTTPS
5. Fingerprint technologies
6. Crawl discovered URLs
7. Extract JavaScript intelligence
8. Discover APIs

Later milestones add:
- **M22:** Differential reconnaissance — `./nett diff old-project new-project`
- **M23:** Resume/incremental — `./nett resume acme` (pick up where you left off)

---

## 9. Troubleshooting

### "command not found"

You built locally but `./nett` is not in your `$PATH`:

```bash
# Use the relative path
./nett --help

# Or move it somewhere in your PATH
mv nett /usr/local/bin/
```

### "invalid configuration"

A config value is wrong. Look at the error:

```bash
./nett config 2>&1 | grep -i invalid
```

Check `./nett --help` for valid values.

### "unknown command"

The command isn't implemented yet (it's planned for a future milestone):

```bash
./nett modules
```

shows which commands are coming and when.

### "permission denied" writing logs

The log file directory needs write permission:

```bash
mkdir -p logs
./nett --config nett.json config
```

---

## 10. Development and testing

If you cloned the repo:

```bash
# Build
make build

# Run all quality gates (must pass before each milestone)
make check     # go vet + go test + go test -race

# Run benchmarks
make bench

# Clean build artifacts
make clean
```

---

## What's implemented right now (M1)

✅ CLI with proper help and command dispatch  
✅ Layered configuration (defaults → file → env → flags)  
✅ Structured logging (text/JSON, levels, components, file output)  
✅ Module catalog showing the roadmap  
✅ Zero external dependencies (builds offline)  

## What's coming

📅 **M2:** SQLite asset-graph store + data model  
📅 **M3:** Scope engine (domain/IP/CIDR/URL validation)  
📅 **M4:** Native DNS resolver (A/AAAA/CNAME/MX/NS/TXT/…)  
📅 **M5:** Subdomain discovery (passive providers + CT)  
📅 **M6:** ASN/IP intelligence  
📅 **M7:** HTTP probing + TLS inspection  
📅 **M8–M21:** Crawling, JS analysis, API discovery, GraphQL, repositories, cloud ID, scoring, event/pivot engine  
📅 **M22–M26:** Differential recon, resume/incremental, export, testing, docs, optimization  

---

## More information

- `docs/ARCHITECTURE.md` — repository layout, concurrency model, event engine
- `docs/DATA_MODEL.md` — the SQLite schema and Go domain types
- `docs/MODULES.md` — engineering notes for each module (purpose, algorithm, tests)
