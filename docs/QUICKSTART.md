# nett — Quickstart: Install to First Scan

This guide walks you through installing nett and running your first reconnaissance. All you need is Go 1.25+. No other security tools required.

---

## 1. Prerequisites

- **Go 1.25+** — [download](https://golang.org/dl)
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
git clone https://github.com/offsecfergali/nett.git
cd nett
go build -o nett ./cmd/nett
```

The binary `./nett` is now ready to use.

### Option B: Install to `$GOPATH/bin`

```bash
go install github.com/offsecfergali/nett/cmd/nett@latest
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

**Output (current):**

```
Implemented capabilities:

CLI
 ├── capabilities
 ├── config
 ├── help
 ├── modules
 └── version

CT
 └── crt.sh

Config
 ├── defaults
 ├── env-overrides
 ├── flag-overrides
 ├── json-file
 └── validation

DNS
 ├── A
 ├── AAAA
 ├── CAA
 ├── CNAME
 ├── MX
 ├── NS
 ├── PTR
 ├── SOA
 ├── SRV
 └── TXT

DNS-Features
 ├── negative-cache
 ├── positive-cache
 ├── resolver-rotation
 ├── retries
 ├── ttl
 └── wildcard-detection

DNS-Transport
 ├── DoH
 ├── DoT
 ├── TCP
 └── UDP

Logging
 ├── component-scoping
 ├── file-output
 ├── json
 ├── levels
 └── text

Permute
 ├── common-affix-permutations
 ├── wildcard-filtering
 └── wordlist

Portscan
 ├── all-ports
 ├── bounded-concurrency
 ├── common-ports
 ├── port-list
 ├── port-range
 ├── rate-limited
 └── single-port

Scope
 ├── allowlist
 ├── cidr
 ├── denylist
 ├── domain
 ├── dynamic-ip-learning
 ├── hostname
 ├── ip
 ├── path
 ├── url
 └── wildcard

Store
 ├── additive-merge
 ├── assets
 ├── edges
 ├── event-dedup
 ├── events
 ├── module-runs
 ├── projects
 └── provenance

Subdomain
 ├── ct-merge
 ├── dns-validation
 ├── scope-filter
 └── wildcard-flagging
```

Notice: **HTTP probing, TLS inspection, crawling, and API discovery are not here yet** (they are planned for later milestones). The tool never claims a capability it doesn't have — run `nett capabilities` yourself at any time to see exactly what's real today.

### See the full module roadmap

```bash
./nett modules
```

**Output:**

```
NAME         MILE   STATUS       DESCRIPTION
core         M1     implemented  CLI, layered configuration, and structured logging
store        M2     implemented  SQLite asset-graph store: assets, edges, provenance, events, module runs
scope        M3     implemented  Scope engine (domain/host/IP/CIDR/URL/path, allow/deny, fail-closed)
dns          M4     implemented  Native DNS resolver with caching and wildcard detection
ct           M5     implemented  Certificate Transparency discovery via crt.sh
subdomain    M5     implemented  Passive subdomain discovery: CT + DNS validation + wildcard filtering
permute      M7     implemented  DNS brute force / permutation engine for candidate hostnames
portscan     M9     implemented  Native TCP connect scanner (explicit activation only)
asn          M6     planned      ASN/IP intelligence and netblock relationships
http         M7     planned      Native HTTP probing engine and reusable client
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
Status:     implemented
Active:     true  (makes network requests; enforces scope before contacting targets)
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
    "user_agent": "nett/0.1 (+https://github.com/offsecfergali/nett)",
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

## 7. Available commands

Currently implemented:

| Command | Purpose | Example |
|---------|---------|---------|
| `version` | Print version and build metadata | `./nett version` |
| `capabilities` | List implemented capabilities (truthful, never overstates) | `./nett capabilities` |
| `modules` | List all modules with status (implemented/planned) or show one module's detail | `./nett modules` or `./nett modules dns` |
| `config` | Print the resolved effective configuration (JSON) | `./nett config` |
| `scan` | Run reconnaissance against a target (`-d`, `-brute`, `-tcp`, `-p`) | `./nett scan example.com -d` |

**Intentionally absent (not yet implemented):**
- `pipeline` — added in M21 (event/pivot engine)
- `resume` — added in M23 (incremental reconnaissance)

`scan` itself also parses `-udp`, `-f`, `-en`, and `-dir` — each fails with an
explicit "not implemented yet (planned M#)" error rather than being silently
accepted, since their underlying modules (M8–M21) don't exist yet.

The CLI never lists a command, or accepts a flag, it cannot actually run.

---

## 8. Running your first scan

```bash
./nett --project acme scan example.com -d
```

This:
- Queries Certificate Transparency (crt.sh) for hostnames issued a certificate under `example.com`
- Validates each candidate against the native DNS engine, flagging any that only match the domain's own DNS wildcard
- Creates `./data/acme.db` (a SQLite asset graph: domains, hostnames, IPs, relationships, provenance)

Add a DNS brute force pass and a TCP scan in the same run:

```bash
./nett --project acme scan example.com -d -brute words.txt -tcp 1-1000
```

Every discovered/resolved IP becomes part of that run's scope automatically,
so `-tcp`/`-p` can scan what `-d`/`-brute` just found in a future combined
invocation — see `docs/ARCHITECTURE.md` §5–§6 for the full discover → validate
→ classify → store → relate → prioritize → pivot design this is built toward.

Later milestones add:
- **M21:** the actual event/pivot engine (automatic re-discovery from what's found, not just a shared scope)
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

## What's implemented right now

✅ CLI with proper help and command dispatch, including `scan`
✅ Layered configuration (defaults → file → env → flags)
✅ Structured logging (text/JSON, levels, components, file output)
✅ Module catalog showing the roadmap
✅ SQLite asset-graph store (projects, assets, edges, provenance, events, module runs)
✅ Fail-closed scope engine (domain/wildcard/hostname/IP/CIDR/URL/path)
✅ Native DNS engine (A/AAAA/CNAME/MX/NS/TXT/SRV/CAA/PTR/SOA over UDP/TCP/DoH/DoT, caching, wildcard detection)
✅ Certificate Transparency + passive subdomain discovery (`-d`)
✅ DNS brute force with common-affix permutations (`-brute`)
✅ Native TCP connect scanning (`-tcp`, `-p`)
✅ Almost zero external dependencies (one pure-Go SQLite driver; builds offline otherwise)

## What's coming

📅 **M6:** ASN/IP intelligence and reverse DNS
📅 **M7 (remaining):** native HTTP probing engine and reusable client
📅 **M8:** TLS inspection
📅 **M10:** service fingerprinting + `-f`/`-en`
📅 **M11–M17:** crawling, JS intelligence, `-dir` content discovery, API/GraphQL discovery, repository intelligence, cloud attribution
📅 **M18–M21:** asset classification, scoring, the relationship-graph query layer, and the real event/pivot engine
📅 **M22–M26:** differential recon, resume/incremental, export, testing, docs, performance

---

## More information

- `docs/ARCHITECTURE.md` — repository layout, concurrency model, event engine
- `docs/DATA_MODEL.md` — the SQLite schema and Go domain types
- `docs/MODULES.md` — engineering notes for each module (purpose, algorithm, tests)
