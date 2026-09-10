# nett — Architecture

`nett` is a native Go reconnaissance framework. Its entire core recon stack is
implemented inside this Go codebase. It does **not** shell out to `subfinder`,
`amass`, `httpx`, `dnsx`, `naabu`, `massdns`, `nuclei`, `ffuf`, `gobuster`,
`katana`, `hakrawler`, `assetfinder`, `findomain`, `waybackurls`, `gau`, or any
other external recon program. The only runtime dependencies are the Go standard
library and (for later milestones) a small, deliberately chosen set of
**general-purpose** third-party libraries (protocol/parsing helpers), never a
wrapper around an existing recon tool.

This document is the authoritative map of the repository. It is written up
front, before most milestones exist, so the shape of the system is fixed and the
data flowing between components is predictable. Each milestone fills in one slice
of this map. A component is only described as *implemented* here once its code
exists and its tests pass; everything else is marked *planned* and carries the
milestone that will deliver it.

---

## 1. Design principles

1. **Native first.** Every advertised capability is implemented in Go. Third-party
   Go libraries are allowed only when they provide general-purpose
   protocol/parsing functionality (e.g. an HTML tokenizer, a DNS message codec),
   never a packaged recon workflow.
2. **Every function is real.** No `// TODO` bodies, no "coming soon" printers. A
   feature is not merged until it has `input → processing → error handling →
   output → logging → tests`. The module catalog distinguishes *implemented* from
   *planned* explicitly so the CLI never claims a capability it does not have.
3. **Scope is mandatory for active work.** Every module that touches the network
   asks the scope engine before it makes a request. The scope engine is
   fail-closed: if a target cannot be proven in scope, it is not contacted.
4. **Provenance everywhere.** Every discovered asset records *why* the tool
   believes it exists: source module, evidence, confidence, first/last seen, and
   parent asset. Discovery is deterministic, reproducible, and explainable.
5. **Bounded concurrency.** No "one goroutine per target." Every concurrent stage
   runs behind a worker pool with an explicit limit, context cancellation, and a
   rate limiter. Resources (HTTP connections, DNS sockets, DB handles) are pooled
   and reused.
6. **Persistent and resumable.** State lives in SQLite. A pipeline can be stopped
   and resumed; expensive discovery is not repeated. Two project states can be
   diffed to produce continuous attack-surface monitoring.
7. **Safe by construction.** This is reconnaissance. There is no exploitation,
   brute forcing, credential stuffing, evasion, or payload delivery anywhere in
   the tree. Aggressive active scanning (port scans, range reverse-DNS, content
   discovery) requires explicit activation and respects rate/timeout/scope limits.

---

## 2. Repository layout

```
nett/
├── cmd/
│   └── nett/              main() — thin entrypoint, signal handling, calls cli.Execute
├── internal/
│   ├── version/             build/version metadata                         [M1  implemented]
│   ├── config/              layered config: defaults→file→env→flags + validation [M1 implemented]
│   ├── logging/             structured logging (slog) — levels, text/json, file [M1 implemented]
│   ├── module/              module descriptor + registry + capability catalog [M1 implemented]
│   ├── cli/                 command router, help, capabilities/modules output [M1 implemented]
│   │
│   ├── store/               SQLite schema, migrations, DAO for the asset graph [M2 implemented]
│   ├── model/               core domain types: Asset, Edge, Event, Provenance  [M2 implemented]
│   ├── scope/               scope engine (domain/host/IP/CIDR/URL/path)         [M3 implemented]
│   ├── dns/                 native resolver (A/AAAA/CNAME/MX/NS/TXT/SRV/CAA/PTR/SOA), UDP/TCP/DoH/DoT, cache, wildcard detect [M4 implemented]
│   ├── subdomain/           CT-merge + scope filter + DNS validation + wildcard flag [M5 implemented]
│   ├── ct/                  Certificate Transparency discovery via crt.sh      [M5 implemented]
│   ├── asn/                 ASN/IP intelligence, netblocks, relationships      [M6 planned]
│   ├── permute/            DNS brute-force / permutation engine                [M7 implemented]
│   ├── httpx/               native HTTP probing engine + reusable client       [M7 planned]
│   ├── fingerprint/         technology fingerprinting (external definitions)   [M10 planned]
│   ├── tlsx/                native TLS inspection                              [M8 planned]
│   ├── portscan/            native TCP connect scanner                        [M9 implemented]
│   ├── servicefp/           service/banner fingerprinting                     [M10 planned]
│   ├── crawler/             native concurrent web crawler                     [M11 planned]
│   ├── jsintel/             JavaScript intelligence engine                    [M12 planned]
│   ├── urldisc/             historical URL providers + robots/sitemap         [M13/M18/M19 planned]
│   ├── content/             optional content-discovery engine (BYO wordlist)  [M13/M20 planned]
│   ├── apidisc/             REST/OpenAPI/Swagger discovery + parsing           [M14 planned]
│   ├── graphql/             GraphQL reconnaissance                            [M15 planned]
│   ├── param/               parameter discovery                              [M21 planned]
│   ├── repo/                public repository intelligence                   [M16 planned]
│   ├── cloud/               cloud/CDN attribution                            [M17 planned]
│   ├── classify/            asset classification engine                      [M18 planned]
│   ├── score/               deterministic asset scoring                      [M19 planned]
│   ├── graph/               relationship-graph query layer over store        [M20 planned]
│   ├── event/               event bus + pivot scheduler + dedup/depth guard   [M21 planned]
│   ├── pipeline/            orchestration: wires modules onto the event bus   [M21 planned]
│   ├── diff/                differential reconnaissance                       [M22 planned]
│   ├── resume/              incremental/resume engine (module-run state)      [M23 planned]
│   ├── export/              exporters (JSON, CSV, JSONL)                      [M22/M25 planned]
│   └── ratelimit/, workerpool/, netutil/  shared concurrency + net helpers   [ratelimit implemented (M4); workerpool/netutil as needed]
├── docs/
│   ├── ARCHITECTURE.md      this file
│   ├── DATA_MODEL.md        SQLite schema + domain model
│   └── MODULES.md           per-module engineering notes (purpose/algo/tests)
├── testdata/                offline fixtures (DNS, HTTP, TLS, OpenAPI, …)
├── .github/workflows/ci.yml build + vet + test + race
├── Makefile
└── go.mod
```

`internal/` is used deliberately: the framework is consumed through its CLI, not
imported as a library by third parties, so the packages are free to evolve.

---

## 3. Layered dependency direction

Packages depend downward only. This keeps the graph acyclic and testable.

```
        cmd/nett
            │
           cli
            │
        pipeline ─────────────── event
       /   │   │   \                │
 subdomain ct  httpx crawler …  (modules)
       \   │   │   /
        scope  dns  httpx-client  tlsx        (shared capabilities)
            \   │   /
           model / store / config / logging   (foundation)
```

- **Foundation** (`config`, `logging`, `version`, `model`, `store`) knows nothing
  about recon. It is pure infrastructure.
- **Shared capabilities** (`scope`, `dns`, HTTP client, `tlsx`, `ratelimit`,
  `workerpool`) are reusable primitives with no knowledge of the pipeline.
- **Modules** implement discovery. Each depends on shared capabilities and the
  model, never on another module directly — they communicate only through the
  event bus and the store.
- **Orchestration** (`event`, `pipeline`, `cli`) wires modules together.

Because modules never call each other directly, any module can be tested in
isolation with fixtures, and the pivot graph (section 6) is data-driven rather
than hard-coded.

---

## 4. Concurrency model

Every concurrent stage follows the same template, provided by
`internal/workerpool` and `internal/ratelimit`:

- A **bounded worker pool** (N goroutines, N from config) drains a work channel.
  N is never "unbounded" and never "one per target."
- A **token-bucket rate limiter** gates outbound requests per module (requests
  per second from config).
- A **`context.Context`** threads through every call for cancellation and
  deadlines; a SIGINT/SIGTERM from the CLI cancels the root context and every
  stage unwinds cleanly.
- Results flow back over channels and are **deduplicated** before they hit the
  store or the event bus.
- Shared clients (HTTP transport with connection pooling/keep-alive, DNS
  connection reuse, the DNS cache) are created once and shared across workers.

Concurrency limits are configured globally and per subsystem
(`concurrency.global`, `concurrency.dns`, `concurrency.http`,
`concurrency.port`) so a researcher can tune aggressiveness without touching
code.

---

## 5. Data flow (a pipeline run)

```
target(s) ──▶ scope engine (define what is in scope)
                     │
        ┌────────────┴───────────── passive discovery ─────────────┐
        │  subdomain providers, certificate transparency, ASN/IP,  │
        │  historical URLs, repository intel                        │
        └────────────┬─────────────────────────────────────────────┘
                     │ (each hit → Event on the bus, deduped, scope-checked)
                     ▼
             active resolution/probing (scope-gated, rate-limited)
        DNS resolve → wildcard filter → HTTP probe → TLS inspect →
        fingerprint → crawl → JS intel → API/GraphQL discovery →
        port scan/service FP (only when explicitly enabled)
                     │
                     ▼
        classification + scoring + relationship graph (provenance kept)
                     │
                     ▼
            SQLite store  ──▶ export / diff / resume
```

The mechanism that turns a discovery into the next action is the event/pivot
engine, described next.

---

## 6. Event-driven pivot engine (design)

Discovery is not a fixed linear script; it is a set of reactions. Every module
emits typed **events** (`CERTIFICATE_DISCOVERED`, `NEW_HOSTNAME`,
`DNS_RESOLVED`, `HTTP_PROBED`, `TECH_FINGERPRINTED`, `URL_FOUND`, `JS_FOUND`,
`API_FOUND`, …). The scheduler maps event types to the modules that consume
them:

```
CERTIFICATE_DISCOVERED → extract SANs → NEW_HOSTNAME
NEW_HOSTNAME           → scope check → DNS resolve → DNS_RESOLVED
DNS_RESOLVED           → HTTP probe → HTTP_PROBED
HTTP_PROBED            → fingerprint → crawl → JS intel → API discovery
```

Infinite loops are prevented structurally:

- **Deterministic event IDs** (hash of type + normalized subject) — a duplicate
  event is dropped before it schedules work.
- **Execution history** persisted per project: `(module, subject)` pairs already
  processed are not reprocessed on resume.
- **Maximum pivot depth** per chain, from config.
- **Module cooldown** to prevent hot loops on the same asset.

This is an actual scheduler (a bus + worker pool + a dedup/depth guard backed by
the store), not a metaphor. It is delivered in M21; the store tables that back
it (`events`, `module_runs`) are created in M2 so persistence is available
early.

---

## 7. Persistence, resume, and diff

- **Store (M2):** SQLite via the standard `database/sql` interface. The schema
  (see `DATA_MODEL.md`) models an asset graph: `assets` (nodes) and `edges`
  (typed relationships), plus `provenance`, `events`, `module_runs`,
  `projects`, `classifications`, and `scores`. Every row carries provenance and
  `first_seen`/`last_seen`.
- **Resume (M23):** each module records a `module_runs` row (status, cursor,
  fingerprint of inputs). On `nett resume <project>`, completed runs are
  skipped and in-flight work restarts from persisted cursors.
- **Diff (M22):** two project snapshots are compared to report new/removed
  domains, IPs, ports, technologies, URLs, API endpoints, and changed
  certificates/HTTP behavior — turning the tool into continuous monitoring.

---

## 8. Configuration and logging (M1, implemented)

- **Config** (`internal/config`): a single `Config` struct with sections for
  general/logging/concurrency/HTTP/DNS/scope/modules. It is assembled by strict
  precedence — **built-in defaults → config file (JSON) → environment
  (`NETT_*`) → CLI flags** — and validated before anything runs. Invalid
  values produce descriptive errors rather than silent fallback.
- **Logging** (`internal/logging`): a thin layer over the standard library's
  `log/slog`. Levels (`debug`/`info`/`warn`/`error`), formats (`text`/`json`),
  and an optional log file are configured centrally; every module logs through a
  component-scoped child logger so output is filterable.

Full engineering notes for these live in `docs/MODULES.md`.

---

## 9. CLI surface (grows per milestone)

```
nett --help
nett version
nett capabilities        # tree of capabilities that are actually implemented
nett modules             # module catalog with status (implemented/planned) + milestone
nett modules <name>      # detail for one module
nett config              # the fully-resolved effective configuration
nett scan <target> [-d] [-brute <file>] [-tcp [spec]] [-p]
```

`nett scan` exists as of M5/M7/M9 and currently supports `-d` (CT +
DNS-validated subdomain discovery), `-brute <wordlist>` (DNS brute force with
common-affix permutations), `-tcp [spec]` and `-p` (native TCP connect
scanning). `-udp`, `-f`, `-en`, and `-dir` are parsed but rejected with an
explicit "not implemented yet (planned M#)" error rather than silently doing
nothing — the CLI never advertises a flag or command that would produce fake
output. `nett pipeline` is intentionally **absent** until the event/pivot
engine exists (M21). `nett capabilities` is generated from the module catalog
and lists implemented capabilities only.

---

## 10. Testing strategy

- **Unit tests** for every package (config precedence/validation, logging
  output, registry behavior, CLI dispatch, scope edge cases, DNS message
  encode/decode, …).
- **Offline fixtures** in `testdata/` for every network component (DNS wire
  captures, HTTP response fixtures via `httptest`, TLS handshakes, OpenAPI/JSON
  documents, crawler HTML). **No test requires the public Internet.**
- **CI** runs `go build ./...`, `go vet ./...`, `go test ./...`, and
  `go test -race ./...` on every change. A milestone is not "done" until all four
  pass.
- **Benchmarks** (`go test -bench`) for the hot paths (DNS cache, HTTP client,
  scope matching, dedup).

---

## 11. Milestone ledger

| Milestone | Scope | Status |
|-----------|-------|--------|
| M1  | CLI / config / logging | **implemented** |
| M2  | SQLite store + data model | **implemented** |
| M3  | Scope engine | **implemented** |
| M4  | DNS engine | **implemented** |
| M5  | CT + subdomain discovery | **implemented** |
| M6  | ASN / IP / reverse DNS | planned |
| M7  | HTTP engine + DNS permutation | **permute implemented; http client planned** |
| M8  | TLS engine | planned |
| M9  | Port scanner | **implemented** |
| M10 | Service fingerprinting | planned |
| M11 | Crawler | planned |
| M12 | JS intelligence | planned |
| M13 | URL / content discovery | planned |
| M14 | API / OpenAPI | planned |
| M15 | GraphQL | planned |
| M16 | Repository intelligence | planned |
| M17 | Cloud attribution | planned |
| M18 | Asset classification | planned |
| M19 | Scoring | planned |
| M20 | Asset graph | planned |
| M21 | Event / pivot engine | planned |
| M22 | Differential recon | planned |
| M23 | Resume / incremental | planned |
| M24 | Complete testing | planned |
| M25 | Documentation | planned |
| M26 | Performance optimization | planned |

Each milestone ends by running `go build ./... && go vet ./... && go test ./... &&
go test -race ./...`. Work does not proceed past a milestone with failing tests.
