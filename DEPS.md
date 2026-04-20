# Dependencies

Every runtime dependency in rampart is justified here. This file is populated as modules are scaffolded in Adım 2+.

## Policy

- No runtime dependency is added without an entry in this file.
- Each entry covers: **Source**, **Why**, **Alternative considered**, **Risk**, **Upgrade policy**.
- CI will fail if `go.mod`, `package.json`, or `Cargo.toml` introduces a dependency that is not documented here (Adım 8).

## Entry template

```markdown
### <module path>

- **Source:** <upstream URL>
- **Why:** <what it does; why stdlib / a built-in is insufficient>
- **Alternative considered:** <what you rejected and why>
- **Risk:** <maintainer count, release cadence, supply-chain hygiene>
- **Upgrade policy:** <patch auto | minor manual review | major ADR>
```

First entries land with `engine/go.mod` in Adım 2.
