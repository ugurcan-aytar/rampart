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
