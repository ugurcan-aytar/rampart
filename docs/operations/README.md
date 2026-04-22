# Operations documentation

Reserved for operator-facing guides — Compose / Kubernetes
deployment patterns, reverse-proxy templates, observability
(metrics, traces, structured logs, SSE health probes).

The directory is intentionally empty at v0.1.0. The authoritative
operational references for v0.1.0 are:

- Root [`README.md`](../../README.md) — the three-segment
  quickstart paths (solo developer, mid-size team, platform team)
  cover the supported deployment shapes today.
- [`docker-compose.yml`](../../docker-compose.yml) — the canonical
  Compose stack that the demo scenarios run against. The comments
  at the top of that file document why each service is present and
  what is deliberately absent (Postgres, real auth, TLS
  terminator).
- [`SECURITY.md`](../../SECURITY.md) — operator-relevant gaps and
  the trust assumptions rampart makes about its surrounding
  infrastructure.

Extended operational documentation is planned for a future
release.
