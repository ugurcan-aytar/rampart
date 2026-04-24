# Operations documentation

Reserved for operator-facing guides — Compose / Kubernetes
deployment patterns, reverse-proxy templates, observability
(metrics, traces, structured logs, SSE health probes).

## Guides

- [deployment-patterns.md](./deployment-patterns.md) — three
  supported shapes for putting the engine in front of traffic
  (Backstage-fronted, standalone, reverse-proxied) with env-var /
  app-config checklists and nginx/traefik notes.

## Other v0.1.x references

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

Extended operational documentation (observability dashboards,
on-call runbooks, structured-log schema) is planned for a future
release.
