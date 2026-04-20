# @ugurcan-aytar/backstage-plugin-rampart-backend

Backend plugin for rampart. Mounts `/api/rampart/*` on the Backstage
backend and relays calls to the engine. Also owns the nightly
CatalogSync that pushes Component entities into the engine.

Uses the **modern Backstage backend system** (`createBackendPlugin` +
`registerInit`), not the legacy `createRouter` pattern.

## Install

```bash
yarn add @ugurcan-aytar/backstage-plugin-rampart-backend
```

In `packages/backend/src/index.ts`:

```ts
import { rampartPlugin } from '@ugurcan-aytar/backstage-plugin-rampart-backend';

backend.add(rampartPlugin);
```

And in `app-config.yaml`:

```yaml
rampart:
  baseUrl: http://engine:8080
  catalogSyncInterval: 30m   # optional, default 24h
```

## Status

Adım 5 iskelet. Typechecks via shims. Real proxy behaviour (SSE
passthrough for /v1/stream, auth forwarding, header filtering) and a
functional CatalogSync land in Adım 7 when the demo stack runs real
Backstage infrastructure.
