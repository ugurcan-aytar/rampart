# @ugurcan-aytar/backstage-plugin-rampart-backend

Backend plugin for [rampart](https://github.com/ugurcan-aytar/rampart) —
mounts `/api/rampart/*` on the Backstage backend, proxies calls to the
rampart engine, and runs a periodic `CatalogSync` that pushes Backstage
Component entities into the engine so its incident matcher has
something to match against.

> **npm publish status**: this package will appear on npm from rampart
> v0.1.1. Until then, install via the workspace path or use the
> pre-built `ghcr.io/ugurcan-aytar/rampart-backstage` container image.

Built on the modern Backstage backend system (`createBackendPlugin` +
`registerInit`), not the legacy `createRouter` pattern.

## Install

```bash
yarn add @ugurcan-aytar/backstage-plugin-rampart-backend
```

## Setup

In `packages/backend/src/index.ts`:

```ts
import { rampartPlugin } from '@ugurcan-aytar/backstage-plugin-rampart-backend';

backend.add(rampartPlugin);
```

## Configuration

In `app-config.yaml`:

```yaml
rampart:
  baseUrl: http://engine:8080
  catalogSyncInterval: 30m   # optional; default 24h
```

| Key | Required | Default | Notes |
|---|---|---|---|
| `rampart.baseUrl` | yes | — | Origin of the rampart engine. |
| `rampart.catalogSyncInterval` | no | `24h` | Go duration string. Set to `0s` to disable. |

## Routes mounted

- `/api/rampart/v1/*` — proxied to the engine (`{baseUrl}/v1/*`).
  Streaming endpoints (`/v1/stream` SSE) are passed through with
  the headers required for `text/event-stream` to survive
  intermediate proxies.
- `/api/rampart/_health` — backend-local readiness check (does not
  contact the engine).

## CatalogSync

The plugin reads the Backstage catalog on each tick and POSTs every
Component entity to the engine's `/v1/components`. Components removed
from the catalog are deleted from the engine on the next tick. The
sync is idempotent — re-running it on an unchanged catalog is a
no-op.

## Compatibility

| Dependency | Version |
|---|---|
| `@backstage/backend-plugin-api` | `^1.0.0` |
| Node.js | `>=20` |

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
