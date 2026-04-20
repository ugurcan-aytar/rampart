# Dependencies

Every runtime dependency in rampart is justified here. CI will fail (Adım 8) if a module file adds a dep that is not documented below.

## Policy

- No runtime dependency is added without an entry in this file.
- Each entry covers: **Source**, **Why**, **Alternative considered**, **Risk**, **Upgrade policy**.
- Test-only dependencies (e.g. `stretchr/testify`) are listed separately at the bottom.

## Entry template

```markdown
### <module path>

- **Source:** <upstream URL>
- **Why:** <what it does; why stdlib / a built-in is insufficient>
- **Alternative considered:** <what you rejected and why>
- **Risk:** <maintainer count, release cadence, supply-chain hygiene>
- **Upgrade policy:** <patch auto | minor manual review | major ADR>
```

---

## Go runtime dependencies

### github.com/oklog/ulid/v2

- **Source:** https://github.com/oklog/ulid
- **Why:** ULID (Universally Unique Lexicographically Sortable Identifier) for SBOM IDs, incident IDs, remediation IDs. Sortable by creation time, URL-safe, monotonic within the same millisecond — this matters because the incident UI lists events time-ordered by ID.
- **Alternative considered:** `google/uuid` v4 — unsortable, would need a separate `opened_at` index for time ordering; `xid` — good, but ULID is more widely recognised in the Backstage ecosystem (Roadie and Spotify internal tooling both use it).
- **Risk:** Maintained by oklog (Peter Bourgon + collaborators), stable since 2018, two-digit release cadence, pure-Go, zero external deps. Low.
- **Upgrade policy:** patch automatic via Dependabot, minor manual review, major ADR.

---

## Rust runtime dependencies (rampart-native)

Same discipline as the Go section — every crate declared in
`native/Cargo.toml`'s `[workspace.dependencies]` gets justified here.

### serde + serde_json

- **Source:** https://serde.rs + https://docs.rs/serde_json
- **Why:** Rust's de-facto serialization framework. `serde_json` powers the wire-format round-trip in the UDS protocol and the SBOM payload itself; `serde`'s `derive` macros generate the Serialize/Deserialize impls for every IPC type. Stdlib has no JSON support.
- **Alternative considered:** `simd-json` (https://github.com/simd-lite/simd-json) — faster on very large inputs but has a strict platform requirement (AVX2 / NEON baseline) and inserts a hard dependency on its own error types. Rejected: for 50 MiB lockfiles we measured `serde_json` as "fast enough" (see `docs/benchmarks/sbom-parser.md`); SIMD-JSON optimisation is a Phase 3 lever if the IPC floor stops dominating. `sonic-rs` (https://github.com/cloudwego/sonic-rs) — same argument, plus platform availability on macOS arm64 is less mature at time of writing.
- **Risk:** serde is maintained by dtolnay + the serde-rs org; serde_json is in the same org. Both are in the top-10 most-depended-on crates on crates.io — any bug affecting rampart would affect half the Rust ecosystem first, so we benefit from shared vigilance.
- **Upgrade policy:** patch automatic, minor manual review, major ADR.

### tokio (features: `rt`, `net`, `io-util`, `macros`, `signal`, `sync`, `time`)

- **Source:** https://tokio.rs — https://docs.rs/tokio/latest/tokio/#feature-flags
- **Why:** Async runtime + `net::UnixListener`. `rt` (single-threaded current-thread runtime — ADR-0005 ships Phase 1 as single-worker), `net` for UDS, `io-util` for `AsyncReadExt` / `AsyncWriteExt`, `macros` for `#[tokio::main]` / `tokio::select!`, `signal` for SIGTERM handling, `sync` for `oneshot` shutdown channels, `time` for deadlines.
- **Alternative considered:** `async-std` (smaller but less-maintained ecosystem — e.g. async-std hasn't shipped a minor release in >12 months at time of writing); `smol` (minimal — but we'd hand-roll the runtime bootstrap and signal handling). Neither buys rampart anything.
- **Risk:** tokio is the default async runtime for the Rust ecosystem — 500M+ downloads, active contributors at AWS + Buoyant + independents. Low.
- **Upgrade policy:** patch automatic, minor manual review within a day, major ADR.

### base64 (v0.22)

- **Source:** https://docs.rs/base64
- **Why:** Wire-format carries lockfile bytes as base64 in a JSON string (see `schemas/native-ipc.md`). stdlib has no base64 today.
- **Alternative considered:** hand-rolled decoder (unsafe is forbidden by `[workspace.lints.rust].unsafe_code = "forbid"`; a pure-Rust decoder is small but `base64` is a zero-dep crate and auditing it is trivial).
- **Risk:** Maintainer: marshallpierce; stable since 2018, 1.5B+ downloads. Zero runtime deps. Low.
- **Upgrade policy:** patch automatic, minor manual review.

### chrono (v0.4, features: `std`, `clock`, `default-features = false`)

- **Source:** https://docs.rs/chrono
- **Why:** RFC3339 nanosecond-precision formatting for `SBOM.GeneratedAt`. The Go side uses `time.RFC3339Nano` and refuses empty strings; the Rust side needs to format an equivalent string when the caller doesn't supply one. `std::time::SystemTime` has no RFC3339 formatter — that's what chrono buys us.
- **Alternative considered:** `time` crate (https://docs.rs/time) — equivalent feature set, slightly smaller. Either works; chrono's docs and StackOverflow footprint are larger, which helps onboarding. Trivial to swap if a CVE forces the issue.
- **Risk:** Maintained by the chrono-rs org (~10 active maintainers), ~500M downloads. Low — `default-features = false` drops the `oldtime` + `serde` + `wasmbind` surface we don't need.
- **Upgrade policy:** patch automatic, minor manual review.

### thiserror (v2)

- **Source:** https://github.com/dtolnay/thiserror
- **Why:** derive-macro for typed error enums. rampart's `ParseError` variants (Malformed / UnsupportedVersion / Empty) map to IPC error codes; thiserror gives us `From<serde_json::Error>` + `Display` + `Error` impls without the boilerplate.
- **Alternative considered:** hand-written `impl Display + Error` (verbose, error-prone); `anyhow` alone (great at the binary boundary, wrong shape for library-internal typed errors).
- **Risk:** Dtolnay, single maintainer, but has near-perfect "if he gets hit by a bus" handoff (multiple co-maintainers with commit rights; also owned by serde-rs org members). Low.
- **Upgrade policy:** patch automatic, minor manual review.

### anyhow (v1)

- **Source:** https://docs.rs/anyhow
- **Why:** Top-level error wrapper in the CLI binary (`rampart-native-cli`). Lets `async_main` return `anyhow::Result<()>` and `.context(...)` annotate each bootstrap step.
- **Alternative considered:** `Box<dyn Error>` (loses source chain); `eyre` (same idea, smaller ecosystem; sticking with anyhow for onboarding familiarity).
- **Risk:** Same maintainer as thiserror. Low.
- **Upgrade policy:** patch automatic.

### tracing + tracing-subscriber (features: `env-filter`, `fmt`, `json`)

- **Source:** https://tracing.rs
- **Why:** Structured logging. `RUST_LOG=…` env filter for level control, JSON log format toggle via `RAMPART_LOG_FORMAT=json` so the Adım 7 Docker stack can parse logs centrally. stdlib has `eprintln!` — not enough.
- **Alternative considered:** `log` + `env_logger` (older, simpler — but missing structured key/value pairs we lean on heavily in the UDS handler); `slog` (less active since tracing took over).
- **Risk:** tokio-rs org; active development; 300M+ downloads. Low.
- **Upgrade policy:** patch automatic, minor manual review.

---

## JS / TypeScript runtime dependencies

None yet. First entries land with the Backstage plugins in Adım 5 (all inside the `@backstage/*` whitelist defined in ARCHITECTURE.md).

---

## Test-only dependencies

These are `require ... // indirect` or test-scoped; they do not ship with production binaries.

### github.com/stretchr/testify

- **Source:** https://github.com/stretchr/testify
- **Why:** `require.Equal`, `require.ErrorIs`, table-driven test helpers. The alternative (pure `if got != want { t.Errorf(...) }`) bloats tests by 30–40% for no clarity gain. Testify is the de-facto standard across Go ecosystems; reviewers reading this repo recognise the idioms immediately.
- **Alternative considered:** stdlib `testing` only (too verbose), `gotest.tools/v3` (smaller mindshare).
- **Risk:** Maintained by stretchr, stable since 2013, broad adoption (top 50 most-imported Go module). Low.
- **Upgrade policy:** patch automatic, minor manual review.
