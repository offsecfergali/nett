# nett — Module Engineering Notes

For every module this document records the same nine facets requested in the
specification: **Purpose, Input, Algorithm, Concurrency model, Output, Data
model, Failure behavior, Security considerations, Tests.** This is intended so
the engineering behind each module is understandable, not just the code.

Modules are documented as they are implemented. Sections marked *(planned)*
describe the intended design and will be filled with implementation detail when
their milestone lands. Everything under "Milestone 1" below is fully implemented
and tested today.

---

## Milestone 1 — implemented

### `internal/version`

- **Purpose.** Expose build/release metadata (version, commit, date, Go version,
  platform), overridable via `-ldflags`.
- **Input.** Compile-time `-X` linker values; runtime info from `runtime`.
- **Algorithm.** Read package vars and `runtime.Version()`/`GOOS`/`GOARCH`;
  format into `Info` and a one-line string.
- **Concurrency model.** None; pure, stateless functions.
- **Output.** `Info` struct (JSON-tagged) and `String()`.
- **Data model.** `Info{Version, Commit, Date, GoVersion, Platform}`.
- **Failure behavior.** Cannot fail; unset values default to sentinels
  (`0.1.0-dev`, `unknown`).
- **Security considerations.** No untrusted input; nothing sensitive emitted.
- **Tests.** `version_test.go` asserts runtime fields are populated and that
  `String()` includes the version.

### `internal/config`

- **Purpose.** Produce one validated, effective `Config` from four layers with a
  strict precedence: **defaults → JSON file → `NETT_*` env → CLI flags**.
- **Input.** Optional file path; the process environment (injected as a function
  for testability); flag values (applied by the cli package).
- **Algorithm.** Start from `Default()`. If a path is given, JSON-decode it *onto*
  the defaults with `DisallowUnknownFields` (so only present keys override and
  typos are caught). Overlay env via typed setters that parse ints/floats/bools
  and CSV lists, returning descriptive errors on malformed values. `Validate()`
  accumulates every problem into a single error.
- **Concurrency model.** None; config is built once at startup before goroutines
  exist.
- **Output.** `*Config`; `JSON()` for `nett config`.
- **Data model.** `Config` with `Logging`, `Concurrency`, `HTTP`, `DNS`, `Scope`
  sections plus a `Modules` enable map. Sections for later milestones are defined
  now so the on-disk format is stable.
- **Failure behavior.** Missing file, parse error, unknown field, unparsable env,
  or any invalid value returns a wrapped error; the CLI exits non-zero and runs
  nothing. Fail-closed, never silent fallback.
- **Security considerations.** No network. Validation bounds concurrency and
  timeouts to positive values so a bad config cannot spawn unbounded work.
- **Tests.** `config_test.go`: default validity, validation catches each class of
  problem, env overrides (incl. CSV + bad-number rejection), file-over-defaults
  merge, unknown-field rejection, missing file, JSON output.

### `internal/logging`

- **Purpose.** Structured logging with configurable level, format, and sink; a
  component-scoped child logger per module.
- **Input.** `config.LoggingConfig` and a fallback writer (stderr).
- **Algorithm.** Parse level string → `slog.Level`; select a text or JSON
  `slog.Handler`; if a file is configured, open it (append/create) and use it as
  the sink, tracking a closer.
- **Concurrency model.** `slog` handlers are safe for concurrent use; loggers are
  shared across worker pools unchanged.
- **Output.** `*Logger` embedding `*slog.Logger`, plus `Component(name)` and
  `Close()`.
- **Data model.** `Logger{*slog.Logger, closer io.Closer}`.
- **Failure behavior.** Unknown level/format or an unopenable file returns an
  error (and closes a half-opened file). `Discard()` provides a safe no-op logger
  for tests.
- **Security considerations.** Log file opened `0644`; callers control what is
  logged (no secrets logged by the framework itself).
- **Tests.** `logging_test.go`: JSON field presence, level filtering, component
  tag, invalid level/format errors, file output, Discard safety.

### `internal/module`

- **Purpose.** The catalog/registry powering `nett modules` and
  `nett capabilities`. Enforces the honesty rule: only implemented modules
  contribute advertised capabilities.
- **Input.** Descriptors registered at startup via `DefaultRegistry()`.
- **Algorithm.** A name→descriptor map plus an insertion-order slice.
  `Capabilities()` merges capability groups from *implemented* descriptors only,
  unioning and sorting items.
- **Concurrency model.** `sync.RWMutex`-guarded; safe for concurrent reads once
  built.
- **Output.** `[]*Descriptor` views (`List`, `Implemented`) and `[]Capability`.
- **Data model.** `Descriptor{Name, Short, Milestone, Status, Active,
  Capabilities}`; `Capability{Group, Items}`; `Status` enum.
- **Failure behavior.** `Register` rejects nil/empty-name/bad-status/duplicate;
  `MustRegister` panics (catalog is code, a dup is a programming error).
- **Security considerations.** `Active` marks modules that touch the network so
  the pipeline can enforce the "scope before request" rule; capabilities never
  overstate what exists.
- **Tests.** `registry_test.go`: register/get, bad-input rejection, order,
  implemented filter, capability merge/dedup/exclusion, default-registry shape.

### `internal/cli`

- **Purpose.** Parse args, assemble config, init logging, build the registry, and
  dispatch to a subcommand. Testable end-to-end via `Execute(ctx, args, out, err)`.
- **Input.** `os.Args[1:]` (or injected args in tests); stdout/stderr writers.
- **Algorithm.** Parse global flags with `flag` (stops at the subcommand);
  handle `--version`/help; look up the command; handle per-command `--help`
  without needing a valid config; else load config, apply flag overrides,
  re-validate, build the logger and registry, and run the command. Exit codes:
  0 ok, 1 runtime error, 2 usage error.
- **Concurrency model.** Single-threaded dispatch; a cancellable `context.Context`
  threads to commands (SIGINT/SIGTERM cancels it) for future long-running work.
- **Output.** Written to the injected writers; commands render version info,
  the capability tree, the module catalog, and the effective config.
- **Data model.** `App{Config, Log, Registry, Out, Err}`; `Command{Name, Short,
  Long, Args, Run}`.
- **Failure behavior.** Invalid config/flags/logging exit 1 with a `nett: …`
  message; unknown command/module reported with usage; nothing runs on error.
- **Security considerations.** No commands in M1 touch the network. `scan` and
  `pipeline` are deliberately unregistered until their modules exist, so the CLI
  never offers a non-functional action.
- **Tests.** `cli_test.go`: version (cmd+flag), capabilities (implemented-only,
  DNS excluded), module list/detail/unknown, config JSON validity, unknown
  command, help/no-args, per-command help, bad-flag failure, flag>env>file
  precedence, config-file loading.

---

## Milestone 2 — implemented

### `internal/model`

- **Purpose.** The in-memory domain types for the asset graph — `Asset`,
  `Edge`, `Provenance`, `Event`, `ModuleRun`, `Project` — and the deterministic
  ID scheme that lets independent modules converge on the same node.
- **Input.** None; pure type/constant definitions plus three hashing helpers.
- **Algorithm.** `AssetID(type, key)`, `EdgeID(src, rel, dst)`, and
  `EventID(type, assetID)` are each a hex SHA-256 over their NUL-joined parts,
  matching `docs/DATA_MODEL.md` §1 exactly so the store, and anything reading
  the database directly, agree on identity.
- **Concurrency model.** None; stateless value types.
- **Output.** The structs themselves, plus `AssetType`/`RelType` string enums
  matching the vocabularies in `DATA_MODEL.md` §2.2 and §3.
- **Data model.** This package *is* the data model; see `DATA_MODEL.md` §5.
- **Failure behavior.** N/A — hashing cannot fail.
- **Security considerations.** No untrusted input handled here.
- **Tests.** Covered indirectly by `internal/store`'s tests (deterministic-ID
  round trips, merge behavior); no separate test file since there is no
  branching logic to exercise beyond the store's use of it.

### `internal/store`

- **Purpose.** The SQLite-backed asset graph: the only package that opens the
  database, so every other package goes through its DAO methods.
- **Input.** A file path (or `:memory:`) to `Open`; `model.Asset`/`Edge`/
  `Provenance`/`Event`/`ModuleRun` values from callers.
- **Algorithm.** `Open` sets `PRAGMA journal_mode=WAL`, `foreign_keys=ON`,
  `busy_timeout=5000`, then applies the full schema via idempotent `CREATE
  TABLE/INDEX IF NOT EXISTS` statements (safe to call on an existing
  database). `UpsertAsset`/`UpsertEdge` read the existing row (if any) before
  writing: `value_json` is merged additively (`mergeValue` — scalars overwrite,
  `[]any` fields are unioned and deduplicated by JSON representation),
  confidence takes the max of old and new, `first_seen` is preserved, and
  `in_scope`/`historical` verdicts are fixed at first insert and never
  overwritten. `InsertEvent` uses `INSERT ... ON CONFLICT DO NOTHING` keyed on
  the deterministic event ID and reports whether the row was newly inserted —
  the loop-guard primitive the pivot engine (M21) will build on.
  `UpsertModuleRun` is a keyed upsert on `(project, module, target)` that
  preserves the original `started_at`, backing `nett resume` (M23).
- **Concurrency model.** `db.SetMaxOpenConns(1)`: SQLite is single-writer
  regardless, so the connection pool is capped at one to turn concurrent
  writers into a safely serialized queue instead of "database is locked"
  errors, without a bespoke writer-goroutine abstraction nothing yet needs.
- **Output.** `*model.Asset`/`*model.Edge`/`*model.Provenance`/`*model.Event`/
  `*model.ModuleRun`/`*model.Project` values; `nil, nil` for a missing row
  (never a sentinel error, so callers can't mistake "not found" for failure).
- **Data model.** Implements the full schema in `docs/DATA_MODEL.md` §2:
  `projects`, `assets`, `edges`, `provenance`, `events`, `module_runs`,
  `classifications`, `scores` (the last two are schema-only until M18/M19 have
  a writer).
- **Failure behavior.** Every method wraps and returns the underlying SQL
  error with context (`store: <op> <subject>: %w`); nothing is swallowed.
- **Security considerations.** No network. Provenance rows are append-only
  (never merged/overwritten) so the "why does this exist" audit trail cannot
  be silently rewritten by a later observation.
- **Tests.** `store_test.go`: idempotent re-`Open` (schema re-apply), idempotent
  `CreateProject`, missing-project lookup, additive asset merge (scalar
  overwrite + list union + confidence max + `first_seen` preserved + scope
  verdict fixed at insert), type-filtered `ListAssets`, edge upsert not
  duplicating rows and taking max confidence, directional edge queries,
  provenance rows accumulating (never merged) per subject, event dedup by
  deterministic ID plus the pending→processed transition, and module-run
  upsert/resume lookup. Race-clean under `go test -race`.

---

## Milestone 3 — implemented

### `internal/scope`

- **Purpose.** The single authority every network-active module consults
  before contacting a target. Fail-closed: nothing is in scope unless an
  explicit include rule matches it, and any exclude match wins over any
  include match.
- **Input.** `config.ScopeConfig{Include, Exclude, Wildcards}` at
  construction; hostnames/IPs/URLs at query time; `AddDomain` for CLI-supplied
  scan targets; `LearnIP` for IPs resolved from an already in-scope host.
- **Algorithm.** Each `Include`/`Exclude` entry is self-classified at
  construction time by shape: a parseable IP is an IP rule, a parseable CIDR is
  a CIDR rule, a `scheme://` entry seeds its hostname as a domain rule, a
  leading `/` (exclude only) is a path-glob rule, an entry containing `*` is a
  glob compiled to an anchored case-insensitive regexp, and anything else is a
  domain rule matching itself and every subdomain (`api.example.com` matches
  `example.com`). `Wildcards` entries are always compiled as globs. `AllowHost`
  checks exclude domains/globs first (any match denies), then include
  domains/globs (any match allows); no match denies. `AllowIP` mirrors this
  over static IP/CIDR rules plus a `dynamic` set populated only by `LearnIP`,
  which itself only records an IP if its source host already passed
  `AllowHost` — so IP scope can legitimately grow from a scoped domain's own
  resolutions without a separate IP allowlist. `AllowURL` parses the URL,
  checks the host via `AllowHost`, then checks the path against excluded path
  globs.
- **Concurrency model.** `sync.RWMutex`-guarded; safe for concurrent reads
  during a scan and concurrent `LearnIP` writes from resolver workers.
- **Output.** `bool` (`AllowHost`, `AllowIP`) and `(bool, error)` (`AllowURL`,
  which also validates the URL is parseable).
- **Data model.** `Scope{domains, wildcards, excludeDomains, excludeWild,
  excludePaths []*regexp.Regexp, ips, cidrs, excludeIPs, excludeCIDRs, dynamic}`.
- **Failure behavior.** `New` rejects an unparsable wildcard/glob or an empty
  exclude entry with a wrapped error; the CLI would exit non-zero before any
  network module starts. `AllowURL` returns an error (not a silent `false`) for
  an unparsable URL so callers can distinguish "out of scope" from "malformed
  input."
- **Security considerations.** This *is* the safety boundary for M4+ active
  modules — DNS resolution, port scanning, HTTP probing, and content discovery
  all must call `AllowHost`/`AllowIP`/`AllowURL` before touching the network.
  Fail-closed by construction: an empty `Scope` allows nothing.
- **Tests.** `scope_test.go`: fail-closed empty scope, domain self/subdomain
  matching (incl. case and trailing-dot normalization), exclude-overrides-
  include (including subdomains of an excluded domain), wildcard config and
  wildcard-shaped include entries, CIDR include/exclude, exact-IP include,
  `LearnIP` gated on the source host's scope, `AddDomain` seeding, `AllowURL`
  happy path / excluded path / out-of-scope host / unparsable URL, URL include
  entries seeding their hostname. Race-clean under `go test -race`.

---

## Milestone 4 — implemented

### `internal/ratelimit`

- **Purpose.** One shared token-bucket rate limiter used by every active
  module (DNS now; HTTP and port scanning in later milestones) so "respect
  the configured rate limit" (docs/ARCHITECTURE.md §4) is implemented once.
- **Input.** A rate in operations/second at construction (`New`); `Wait(ctx)`
  calls from callers before each outbound operation.
- **Algorithm.** Classic token bucket: tokens accumulate at `rate`/sec up to a
  `burst` capacity (one second's worth, minimum 1); `Wait` consumes a token if
  one is available or sleeps until one will be, re-checking on a timer against
  `ctx.Done()` so cancellation is honored while waiting. A rate `<= 0` disables
  limiting (`Wait` returns immediately) — this is how DNS/HTTP config's
  `rate_limit_per_sec: 0` means "unlimited" rather than "frozen."
- **Concurrency model.** `sync.Mutex`-guarded; many goroutines can share one
  `*Limiter` safely (verified under `-race` with 20 concurrent goroutines).
- **Output.** `error` — nil once a token is acquired, or `ctx.Err()` if the
  context is cancelled first.
- **Data model.** `Limiter{rate, burst, tokens, last}`.
- **Failure behavior.** Never fails on its own; only propagates context
  cancellation/deadline errors from the caller.
- **Security considerations.** This is a safety control, not just a
  performance one — bounding request rate is part of not overwhelming a
  target that consented to reconnaissance.
- **Tests.** `limiter_test.go`: unlimited never blocks, a limited rate
  measurably throttles after the initial burst, context cancellation aborts a
  wait promptly, and 20 concurrent goroutines drawing from one limiter get
  exactly the expected total under `-race`.

### `internal/dns`

- **Purpose.** nett's entire DNS layer: RFC 1035 wire codec, UDP/TCP/DoH/DoT
  transports, a caching resolver, and wildcard detection. No system resolver,
  no external binary — every byte on the wire is built and parsed here.
- **Input.** `config.DNSConfig{Resolvers, TimeoutSeconds, Retries, Protocol,
  RateLimitPerSec}` at construction; a name (and record type, via the typed
  `Lookup*` methods) per call.
- **Algorithm.** `wire.go` encodes/decodes the full header, question, and
  resource-record sections for A/AAAA/CNAME/NS/PTR/MX/TXT/SRV/CAA/SOA,
  including RFC 1035 §4.1.4 name-compression pointers on decode (with a
  128-jump guard against a malicious or malformed pointer loop) — encoding
  never compresses, since nett only ever encodes small single-question
  queries. `transport.go` implements the four wire transports: UDP and TCP
  (2-byte length-prefixed stream per §4.2.2), DoT (the same stream framing
  inside a `crypto/tls` connection, RFC 7858), and DoH (an HTTP POST of the
  raw message with `Content-Type: application/dns-message`, RFC 8484).
  `resolver.go` is the caching layer: `Query` checks an in-memory cache keyed
  by `(name, qtype)`, then on a miss rate-limits (`internal/ratelimit`) and
  retries across every configured resolver round-robin (`Retries+1` total
  attempts). A positive result is cached for the minimum TTL among its
  records; a negative result (NXDOMAIN, or NOERROR with no matching answer) is
  cached using the response's SOA `MINIMUM` field per RFC 2308 §5, falling
  back to a 30s constant when no SOA is present. `LookupA`/`LookupAAAA` chase
  a returned CNAME up to 8 hops (with same-name loop detection) if the
  resolver did not already flatten the chain itself — answers are filtered by
  record type only, not by owner name, specifically because a chain-flattening
  recursive resolver returns the final A/AAAA record under the *target's*
  name, not the name that was queried. `wildcard.go` probes three random
  24-byte-random hex labels under a domain; any that resolve mark the domain
  as wildcarded, and `WildcardResult.Is` lets callers recognize a later
  discovery that merely reproduces the wildcard's own answer.
- **Concurrency model.** A `sync.Mutex` guards the resolver's cache and
  round-robin index; the shared `*ratelimit.Limiter` serializes outbound rate
  across concurrent callers. Each `Query` call owns its own connection (no
  pooled sockets yet — DoH's `http.Client` does pool internally).
- **Output.** `[]Record` plus the response `Rcode` from `Query`; typed slices
  (`[]net.IP`, `[]string`, `[]*MX`, etc.) from the `Lookup*` convenience
  methods; `*WildcardResult` from `DetectWildcard`.
- **Data model.** `Message{Header fields, Questions, Answers, Authority,
  Additional}`; `Record` with one populated field per `RRType`; `MX`, `SRV`,
  `CAA`, `SOA` sub-structs.
- **Failure behavior.** Every layer wraps errors with context
  (`dns: <op> <subject>: %w`). `exchangeWithRetry` returns the last transport
  error only after every resolver/attempt combination has failed or the
  context is cancelled (checked between attempts so cancellation aborts
  promptly instead of exhausting all retries). NXDOMAIN is not an error —
  `LookupA` et al. return `(nil, nil)` for it, distinguishing "definitively
  absent" from "the query failed."
- **Security considerations.** Rate-limited by construction; every query
  timeout-bounded; DoT verifies the resolver's TLS certificate via
  `tls.Config{ServerName: ...}` derived from the configured resolver address.
  Wildcard probe labels are randomly generated per query, not derived from
  any secret, so they carry no information leak.
- **Tests.** `wire_test.go`: query and every record-type round-trip through
  Marshal/Unmarshal, a hand-built response exercising chained name-compression
  pointers, and a self-referential pointer loop rejected rather than hanging.
  `resolver_test.go`: all tests run against a real UDP server on loopback (no
  external network) — basic A lookup, NXDOMAIN handling, CNAME chasing across
  two extra queries, cache hit avoiding a second wire query, failover across a
  dead resolver to a live one, exhausting all resolvers returning an error,
  PTR/TXT/MX lookups, config validation (no resolvers / unknown protocol),
  `reverseAddr` for IPv4, wildcard detected/absent, SOA-minimum-driven
  negative caching, and prompt abort on context cancellation. Race-clean under
  `go test -race`.

---

## Milestone 5 — implemented

### `internal/ct`

- **Purpose.** Certificate Transparency discovery: find hostnames the public
  CT ecosystem has already observed on issued certificates for a domain, via
  crt.sh's JSON API. This is passive — it contacts a public third-party
  aggregator, never the target.
- **Input.** A domain string; an `*http.Client` (timeout-bounded).
- **Algorithm.** GETs `crt.sh/?q=%.<domain>&output=json`, decodes the JSON
  array of `{common_name, name_value}` rows (crt.sh newline-joins multiple
  SANs into one `name_value` string when a certificate covers several
  names), splits and normalizes every name (lowercase, strip trailing dot,
  strip a leading `*.` wildcard marker), rejects anything that isn't
  plausibly a hostname (contains a space, `@`, `/`, `\`, or quote — catching
  a CA's non-hostname CN slipping into the same field), and returns the
  deduplicated, sorted result. An empty response body (crt.sh's shape for
  "no matches," not `[]`) is treated as zero results, not an error.
- **Concurrency model.** None — one HTTP request per `Query` call; the
  caller (subdomain discovery) is what runs concurrently.
- **Output.** `[]string` of normalized hostnames via the `Provider` interface
  (`Name() string`, `Query(ctx, domain) ([]string, error)`), so a second
  passive source can be added later without changing callers.
- **Data model.** `crtShEntry{CommonName, NameValue}` (unexported, JSON-only).
- **Failure behavior.** A non-200 response, a read/decode failure, or a
  request-construction error each return a wrapped error; the caller (M5's
  `subdomain.Discover`) treats one failing provider as a warning, not a fatal
  discovery failure.
- **Security considerations.** Read-only HTTP GET to a public aggregator; no
  credentials, no write operations, no contact with the target itself.
- **Tests.** `crtsh_test.go` runs entirely against an `httptest.Server` (no
  real network): multi-SAN entries parsed and merged, an empty response,
  a non-hostname CN (email address) rejected while a co-located valid name
  survives, a non-200 response treated as an error, malformed JSON rejected,
  and `normalizeHostname`'s edge cases unit-tested directly.

### `internal/subdomain`

- **Purpose.** Orchestrates `-d`: merges every CT provider's hits, enforces
  scope before any active DNS contact, resolves survivors, and flags results
  that are indistinguishable from the domain's own DNS wildcard.
- **Input.** A `*dns.Resolver`, a `*scope.Scope`, a `[]ct.Provider`, the
  target domain, and a concurrency bound.
- **Algorithm.** Refuses to run at all if the domain itself is not in scope
  (fail fast, matching the scope engine's fail-closed design). Runs
  `dns.DetectWildcard` once for the domain. Queries every provider,
  collecting per-hostname source attribution and per-provider errors
  separately (`map[string]error`) so one broken provider does not abort the
  others. Every candidate is filtered through `sc.AllowHost` *before* being
  resolved — discovery of a name from a passive source is not itself active
  contact, but resolving it is. Surviving candidates are resolved
  concurrently via `internal/workerpool`; each resolved IP is fed back into
  the scope via `sc.LearnIP` (so a subsequent `-tcp` in the same run can
  scan it), and `WildcardResult.Is` flags whether the answer is
  indistinguishable from the wildcard's own.
- **Concurrency model.** Bounded via `workerpool.Run`; a `sync.Mutex` guards
  the shared findings map during concurrent resolution.
- **Output.** `([]Finding, map[string]error, error)` — findings (which may be
  unresolved), per-provider errors, and a hard error only for an
  out-of-scope target.
- **Data model.** `Finding{Hostname, Sources []string, IPs []net.IP,
  Resolved, WildcardMatch bool}`.
- **Failure behavior.** A provider failure or a per-host resolution failure
  never aborts the batch; only an out-of-scope *target* is a hard error,
  since nothing useful can happen at all in that case.
- **Security considerations.** This is the scope boundary between "passive
  lookup about a name" and "active DNS contact" — every resolved lookup goes
  through `sc.AllowHost` first, with no exception.
- **Tests.** `subdomain_test.go` runs its own loopback fake DNS server (a
  `fakeProvider` supplies canned CT-shaped results): providers merged with
  combined source attribution, out-of-scope candidates filtered before
  resolution, an out-of-scope target refused outright, wildcard matches
  correctly flagged (and non-matches correctly not flagged), and a failing
  provider not aborting a working one. Race-clean under `go test -race`.

---

## Milestone 7 — implemented (partial: permute only; http client still planned)

### `internal/permute`

- **Purpose.** Implements `-brute`: DNS brute-forcing a domain against a
  user-supplied wordlist, with optional common-affix permutations, gated by
  scope and wildcard-aware like `-d`.
- **Input.** A wordlist file path (`LoadWordlist`); a `*dns.Resolver`, a
  `*scope.Scope`, the target domain, the word list, an `enablePermutations`
  flag, and a concurrency bound (`BruteForce`).
- **Algorithm.** `LoadWordlist` reads one label per line, lowercasing,
  trimming, skipping blanks and `#` comments, and deduplicating; an empty
  result is an error (a silently-no-op brute force would look like a
  false-negative "nothing found"). `BruteForce` refuses an out-of-scope
  target up front, runs `dns.DetectWildcard` once, and for each word builds
  either just `<word>.<domain>` or, with permutations enabled,
  `GeneratePermutations(word)` — the word itself plus
  `dev-/staging-/stage-/test-/uat-/qa-/01/02`-prefixed and suffixed variants
  — covering the environment-naming conventions a pentester would try by
  hand from a short base wordlist. Every generated candidate is
  scope-checked and deduplicated before resolution; resolution itself is
  bounded and concurrent via `internal/workerpool`, feeding resolved IPs
  back into scope via `sc.LearnIP` exactly like `-d`. Only resolved
  candidates are returned (an unresolved brute-force guess is noise, unlike
  `-d` where even a dead CT-observed hostname is itself informative).
- **Concurrency model.** Bounded via `workerpool.Run`; a `sync.Mutex` guards
  the shared findings map.
- **Output.** `[]Finding{Hostname, IPs, Resolved, WildcardMatch}`.
- **Data model.** Same `Finding` shape as `internal/subdomain`, independently
  defined since the two packages don't share a type dependency.
- **Failure behavior.** A missing/empty wordlist file or an out-of-scope
  target are hard errors returned before any network activity; a single
  candidate's resolution failure is silently treated as "not live," not
  surfaced as an error (a brute-force sweep expects most guesses to miss).
- **Security considerations.** Same scope-before-resolve guarantee as `-d`;
  brute-forcing is inherently noisier than passive discovery, which is why
  wildcard flagging matters even more here.
- **Tests.** `permute_test.go` (own loopback fake DNS server): wordlist
  loading (dedup/trim/lowercase/comment-skip), missing-file and empty-file
  rejection, live-host discovery, out-of-scope target refusal, a permutation
  actually finding a variant not in the base wordlist, wildcard-match
  flagging, and `GeneratePermutations` always including the base word.
  Race-clean under `go test -race`.

### `internal/workerpool`

- **Purpose.** The one bounded-concurrency primitive every active module
  (port scanning, brute force, subdomain resolution, and future HTTP
  probing) runs its fan-out through, per `ARCHITECTURE.md` §4's "no one
  goroutine per target."
- **Input.** A context, a concurrency limit, a slice of items, and a
  per-item function.
- **Algorithm.** A buffered channel of size `concurrency` acts as a
  semaphore; each item spawns a goroutine only once a slot is free. Context
  cancellation is checked *explicitly* before each admission (not only as a
  `select` case) — a send on a non-full buffered channel is always
  immediately ready, so relying on `select` alone would let a cancelled
  context lose that race indefinitely once workers drain faster than new
  ones are admitted.
- **Concurrency model.** Is the concurrency model: a `sync.WaitGroup` plus a
  channel-based semaphore, nothing more.
- **Output.** None (side-effecting `fn` calls); `Run` blocks until every item
  is processed or the context is done.
- **Data model.** Generic: `Run[T any](ctx, concurrency int, items []T, fn
  func(context.Context, T))`.
- **Failure behavior.** N/A — `fn` handles its own errors; `Run` itself
  cannot fail.
- **Security considerations.** This is what makes "bounded concurrency" true
  rather than aspirational across every active module.
- **Tests.** `pool_test.go`: every item processed, observed concurrency
  never exceeds the configured bound (measured with an atomic
  high-water-mark counter), an already-cancelled context schedules zero
  work (deterministically, not just "mostly"), and an empty item list calls
  `fn` zero times. Race-clean under `go test -race`.

---

## Milestone 9 — implemented

### `internal/portscan`

- **Purpose.** Native TCP connect scanning for `-tcp` and `-p`. No nmap, no
  naabu, no masscan — every connection attempt is a plain `net.Dial`.
- **Input.** `ParsePorts(spec)` parses a CLI port spec; `Scanner.Scan(ctx,
  ip, ports)` runs the scan itself.
- **Algorithm.** `ParsePorts` accepts empty (→ `CommonPorts`, a curated ~80
  entries), `"-"`/`"*"` (→ every port 1-65535), and a comma-separated mix of
  single ports and `lo-hi` ranges, deduplicated and sorted; malformed or
  out-of-range input is rejected rather than silently clamped. `Scan` fans
  every port out through `internal/workerpool` (bounded by
  `Config.Concurrency`), rate-limits each probe via `internal/ratelimit`,
  and dials with a per-probe timeout derived from the parent context.
  `classifyError` turns a dial failure into `StateClosed` (an
  `ECONNREFUSED`, checked both directly and unwrapped from a `*net.OpError`
  → `*os.SyscallError`), `StateFiltered` (the dial timed out — probably
  dropped by a firewall, not actively refused), or `StateUnknown` (anything
  else); a successful connect is `StateOpen` and the connection is closed
  immediately.
- **Concurrency model.** `workerpool.Run` bounds concurrent dials; scope
  enforcement is deliberately **not** this package's job — it is a plain,
  independently-testable network primitive, and the caller (the `scan` CLI
  command) checks `sc.AllowIP` once per target IP before invoking `Scan` at
  all, per `ARCHITECTURE.md` §3 ("shared capabilities have no knowledge of
  the pipeline").
- **Output.** `[]Result{IP, Port, State, Latency, Err}`.
- **Data model.** `State` is one of `open`/`closed`/`filtered`/`unknown`.
- **Failure behavior.** A single port's classification never aborts the
  scan; `Scan` returns whatever results were gathered if the context is
  cancelled mid-sweep.
- **Security considerations.** Explicit-activation only (never runs unless
  `-tcp`/`-p` is passed), rate-limited, timeout-bounded, and scope-gated by
  the caller — matching `ARCHITECTURE.md` §7's "aggressive active scanning
  requires explicit activation."
- **Tests.** `ports_test.go`: every `ParsePorts` shape (empty, single, list,
  range, mixed+deduped, all-ports, and seven invalid-input cases).
  `scanner_test.go`: a real local listener detected as open, a real closed
  local port detected as closed (exercising the actual `ECONNREFUSED` path
  end-to-end rather than a synthetic error), multiple ports scanned
  correctly in one call, a pre-cancelled context scanning nothing, and the
  timeout/generic branches of `classifyError` unit-tested directly with a
  synthetic `net.Error`. Race-clean under `go test -race`.

---

## `internal/cli` scan orchestration (M5/M7/M9 wiring)

`scan.go` is the first orchestration layer tying discovery modules together
ahead of the real event/pivot engine (M21): it hand-parses `nett scan`
arguments (`-tcp`/`-udp` need an *optional* trailing value, which the
standard `flag` package cannot express), builds one `scope.Scope` per
invocation (auto-including the CLI target via `AddDomain`/`AddIP`), opens the
project's SQLite store, and dispatches to `-d` → `subdomain.Discover`,
`-brute` → `permute.BruteForce`, `-tcp`/`-p` → `portscan.Scanner`, persisting
every finding as assets/edges/provenance via the store. `-udp`, `-f`, `-en`,
and `-dir` are recognized but rejected with an explicit "not implemented yet
(planned M#)" error — never silently accepted, never printing fake output.
Tested in `scan_test.go`: every usage example from the spec parses correctly,
`-tcp`'s optional value does not swallow a following flag, all four
not-yet-implemented flags are rejected, a real end-to-end scan against a
local TCP listener (verifying both the printed result and that a SQLite
database is actually created), a config-level scope exclude overriding the
CLI target's auto-inclusion, and a missing wordlist file surfaced as a clear
error.

---

## Planned modules

The design for every planned module (`asn`, `http`, `tls`, `servicefp`,
`fingerprint`, `crawler`, `jsintel`, `urldisc`, `content`, `apidisc`,
`graphql`, `param`, `repo`, `cloud`, `classify`, `score`, `graph`, `event`,
`diff`, `resume`) is captured in `ARCHITECTURE.md` (§2, §5–§7) and
`DATA_MODEL.md`. Each will receive its own nine-facet section here as its
milestone is completed, and its catalog `Status` will flip to `implemented`,
making its capabilities appear in `nett capabilities`.
