# nett — Data Model

This is the complete data model for `nett`. Discovery in `nett` is an
**asset graph**: nodes are discovered assets (domains, hostnames, IPs, URLs,
certificates, endpoints, parameters, JS files, repositories, cloud providers,
ASNs), and edges are typed relationships between them. Every node and edge
carries **provenance** so a researcher can always answer *why does the tool
believe this exists?*

The graph is persisted in **SQLite** (via Go's standard `database/sql`), chosen
because it is embedded, single-file, transactional, and requires no external
service — consistent with the "clean container, no external tools" requirement.
The schema below is created by the store package (M2). It is documented in full
now so every later milestone writes into a stable shape.

The Go domain types in `internal/model` mirror these tables. This document is the
contract between the two.

---

## 1. Conventions

- **IDs.** Every asset has a stable, deterministic `id` = a hex SHA-256 of
  `type + "\x00" + canonical_key`, where `canonical_key` is the normalized
  identity (e.g. lowercased FQDN, `ip`, canonical URL). Determinism means the
  same asset discovered by two modules collapses to one node — enabling
  deduplication and cross-source provenance.
- **Timestamps.** Stored as RFC3339 UTC strings (`first_seen`, `last_seen`,
  `created_at`, `updated_at`). `first_seen` is set once; `last_seen` advances on
  every re-observation.
- **Provenance.** Every asset and edge references the module/source that produced
  it, an evidence blob, and a confidence in `[0,1]`.
- **Confidence.** `1.0` = directly observed (e.g. a hostname in a CT log, a
  resolved A record). `< 1.0` = inferred (permutation candidate, weak
  fingerprint). Confidence is never fabricated from a single weak signal.
- **Historical marking.** Assets sourced from historical providers (wayback-style
  datasets) carry `historical = 1` and are never silently merged with live
  observations; they are visibly distinct.
- **Scope decision.** Every asset stores the scope verdict at insert time
  (`in_scope`), so out-of-scope discoveries are recorded (for graph completeness)
  but never actioned by active modules.

---

## 2. Core tables

### 2.1 `projects`
One row per reconnaissance project; the unit of resume and diff.

| column | type | notes |
|--------|------|-------|
| id            | TEXT PK  | slug (e.g. `example-com-2026-09`) |
| name          | TEXT     | human name |
| created_at    | TEXT     | RFC3339 |
| updated_at    | TEXT     | RFC3339 |
| config_json   | TEXT     | effective config snapshot for reproducibility |
| status        | TEXT     | `active` \| `paused` \| `complete` |

### 2.2 `assets` (graph nodes)
Every discovered thing.

| column | type | notes |
|--------|------|-------|
| id          | TEXT PK  | deterministic hash (see §1) |
| project_id  | TEXT     | FK → projects.id |
| type        | TEXT     | `domain`,`hostname`,`ip`,`cidr`,`asn`,`url`,`endpoint`,`parameter`,`certificate`,`jsfile`,`repository`,`cloud_provider`,`service`,`port` |
| key         | TEXT     | canonical identity (normalized) |
| value_json  | TEXT     | type-specific attributes (see §4) |
| in_scope    | INTEGER  | 0/1 scope verdict at insert |
| historical  | INTEGER  | 0/1 sourced from a historical dataset |
| confidence  | REAL     | max confidence across observations |
| first_seen  | TEXT     | RFC3339 |
| last_seen   | TEXT     | RFC3339 |

Indexes: `(project_id, type)`, `(project_id, key)`, unique `(project_id, id)`.

### 2.3 `edges` (typed relationships)
The graph's edges. Directed `src → dst`.

| column | type | notes |
|--------|------|-------|
| id          | TEXT PK  | hash(src_id + rel + dst_id) |
| project_id  | TEXT     | FK |
| src_id      | TEXT     | FK → assets.id |
| dst_id      | TEXT     | FK → assets.id |
| rel         | TEXT     | relationship type (see §3) |
| confidence  | REAL     | |
| first_seen  | TEXT     | |
| last_seen   | TEXT     | |

Indexes: `(project_id, src_id, rel)`, `(project_id, dst_id, rel)`.

### 2.4 `provenance`
Why an asset/edge is believed to exist. Many rows per asset (one per
observation/source).

| column | type | notes |
|--------|------|-------|
| id          | INTEGER PK AUTOINCREMENT | |
| project_id  | TEXT     | FK |
| subject_id  | TEXT     | asset.id or edge.id |
| subject_kind| TEXT     | `asset` \| `edge` |
| module      | TEXT     | producing module (e.g. `certificate_transparency`) |
| source      | TEXT     | fine-grained source (provider name, resolver, URL) |
| evidence    | TEXT     | short human-readable evidence |
| confidence  | REAL     | this observation's confidence |
| parent_id   | TEXT     | asset that led here (pivot parent), nullable |
| observed_at | TEXT     | RFC3339 |

This table is what powers the JSON provenance record in the spec:
`{asset, type, source, confidence, first_seen, parent}`.

### 2.5 `events`
The event log that drives the pivot engine (M21) and gives an audit trail.

| column | type | notes |
|--------|------|-------|
| id          | TEXT PK  | deterministic hash(type + subject) — dedup key |
| project_id  | TEXT     | FK |
| type        | TEXT     | `NEW_HOSTNAME`, `CERTIFICATE_DISCOVERED`, `DNS_RESOLVED`, `HTTP_PROBED`, … |
| asset_id    | TEXT     | subject asset |
| source      | TEXT     | producing module |
| data_json   | TEXT     | event payload |
| depth       | INTEGER  | pivot depth (loop guard) |
| status      | TEXT     | `pending` \| `processed` \| `dropped` |
| created_at  | TEXT     | RFC3339 |
| processed_at| TEXT     | nullable |

### 2.6 `module_runs`
Execution state for resume/incremental recon (M23).

| column | type | notes |
|--------|------|-------|
| id          | INTEGER PK AUTOINCREMENT | |
| project_id  | TEXT     | FK |
| module      | TEXT     | module name |
| target      | TEXT     | what it ran against |
| input_fp    | TEXT     | fingerprint of inputs (skip if unchanged) |
| cursor_json | TEXT     | resumable progress cursor |
| status      | TEXT     | `pending` \| `running` \| `done` \| `error` |
| error       | TEXT     | last error, nullable |
| started_at  | TEXT     | |
| finished_at | TEXT     | nullable |

Unique `(project_id, module, target)` so a completed run is not repeated.

### 2.7 `classifications` (M18)

| column | type | notes |
|--------|------|-------|
| id         | INTEGER PK | |
| project_id | TEXT | FK |
| asset_id   | TEXT | FK |
| label      | TEXT | `production`,`staging`,`development`,`test`,`legacy`,`api`,`admin`,`auth`,`static`,`cdn`,`cloud`,`internal`,`third_party` |
| confidence | REAL | |
| evidence   | TEXT | why this label |

### 2.8 `scores` (M19)

| column | type | notes |
|--------|------|-------|
| id         | INTEGER PK | |
| project_id | TEXT | FK |
| asset_id   | TEXT | FK, unique per project |
| score      | INTEGER | deterministic total |
| priority   | TEXT | `low` \| `medium` \| `high` \| `critical` |
| reasons_json | TEXT | ordered list of `{reason, weight, evidence}` |
| computed_at | TEXT | RFC3339 |

Scoring is **prioritization, not vulnerability detection**. Every point is
explained by a reason with evidence.

---

## 3. Relationship (`edges.rel`) vocabulary

```
domain        → subdomain      HAS_SUBDOMAIN
hostname      → ip             RESOLVES_TO
ip            → asn            ANNOUNCED_BY
asn           → cidr           CONTAINS
cidr          → ip             CONTAINS
ip            → hostname       PTR
hostname      → certificate    PRESENTED
certificate   → hostname       SAN            (cert → names it covers)
hostname      → url            SERVES
url           → endpoint       EXPOSES
endpoint      → parameter      ACCEPTS
url           → jsfile         REFERENCES
jsfile        → endpoint       REVEALS
jsfile        → hostname       MENTIONS
domain        → repository     LINKED_REPO
hostname      → cloud_provider HOSTED_ON
service       → port           ON_PORT
```

Every edge is provenance-backed; a relationship discovered by two modules keeps
two provenance rows but one edge.

---

## 4. Type-specific attributes (`assets.value_json`)

The `value_json` blob holds the fields relevant to each asset type. Shapes:

- **domain / hostname:** `{ fqdn, apex, wildcard_of, resolved:bool }`
- **ip:** `{ ip, version:4|6, ptr:[…] }`
- **cidr:** `{ cidr, version }`
- **asn:** `{ number, org, country }`
- **url:** `{ scheme, host, port, path, query_keys:[…], status, title, content_length, server, http_version, tech:[…] }`
- **endpoint:** `{ method, path, kind:rest|graphql|rpc, auth_hint, params:[…] }`
- **parameter:** `{ name, location:query|body|path|header, endpoint }`
- **certificate:** `{ sha256, issuer, subject, sans:[…], not_before, not_after, sig_alg, wildcard:bool }`
- **jsfile:** `{ url, sha256, size, sourcemap:bool, urls:[…], hosts:[…], paths:[…] }`
- **repository:** `{ platform, owner, name, url }`
- **cloud_provider:** `{ name, signal:cname|header|asn|cert, detail }`
- **service:** `{ protocol, product, version, banner_excerpt }`
- **port:** `{ number, transport:tcp, state:open, service }`

Attributes are additive: re-observation merges new fields and advances
`last_seen` and `confidence` (max), never destroying earlier evidence.

---

## 5. Go domain types (`internal/model`, M2)

```go
type Asset struct {
    ID         string
    ProjectID  string
    Type       AssetType
    Key        string
    Value      map[string]any
    InScope    bool
    Historical bool
    Confidence float64
    FirstSeen  time.Time
    LastSeen   time.Time
}

type Edge struct {
    ID        string
    ProjectID string
    SrcID     string
    DstID     string
    Rel       RelType
    Confidence float64
    FirstSeen time.Time
    LastSeen  time.Time
}

type Provenance struct {
    SubjectID   string
    SubjectKind string   // asset | edge
    Module      string
    Source      string
    Evidence    string
    Confidence  float64
    ParentID    string
    ObservedAt  time.Time
}

type Event struct {
    Type      string
    AssetID   string
    Source    string
    Timestamp time.Time
    Depth     int
    Data      map[string]any
}
```

`AssetType` and `RelType` are string enums matching the vocabularies above.

---

## 6. Integrity & performance notes

- **WAL mode** is enabled for concurrent readers during a run.
- **Foreign keys** are declared; the DAO layer enforces insertion order (asset
  before edge before provenance).
- **Upserts** are used for assets/edges keyed on their deterministic `id`, so
  concurrent producers converge without duplicate rows.
- **Batched writes** behind a single writer goroutine (SQLite is single-writer);
  worker pools hand results to that writer over a channel, matching the
  bounded-concurrency model in `ARCHITECTURE.md`.
- Indices listed per table keep the common lookups (by type, by key, by edge
  direction) fast as the graph grows.

---

## 7. Export & diff shapes (M22/M25)

- **Export:** the graph serializes to JSON (`{assets:[…], edges:[…],
  provenance:[…]}`), JSONL (one asset per line), and CSV (per asset type).
  Historical and out-of-scope assets remain labeled in exports.
- **Diff:** `nett diff old new` computes set differences over `assets` and
  `edges` keyed by deterministic `id`, and field-level differences for
  certificates and URL/HTTP attributes, emitting: new/removed domains, IPs,
  ports, technologies, URLs, API endpoints, and changed certs/HTTP behavior.
