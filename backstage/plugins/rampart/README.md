# @ugurcan-aytar/backstage-plugin-rampart

Frontend plugin for [rampart](https://github.com/ugurcan-aytar/rampart) —
exposes an `IncidentDashboard` page and per-incident detail view inside
your Backstage app, fed by a running rampart engine.

> **npm publish status**: this package will appear on npm from rampart
> v0.1.1. Until then, install via the workspace path or use the
> pre-built `ghcr.io/ugurcan-aytar/rampart-backstage` container image
> (which bundles all three plugins).

## Install

```bash
yarn add @ugurcan-aytar/backstage-plugin-rampart
```

## Setup

In `packages/app/src/App.tsx`, register the routable extensions:

```tsx
import {
  IncidentDashboardPage,
  IncidentDetailPage,
  rampartRouteRef,
} from '@ugurcan-aytar/backstage-plugin-rampart';

const routes = (
  <FlatRoutes>
    {/* …existing routes… */}
    <Route path="/rampart" element={<IncidentDashboardPage />} />
    <Route path="/rampart/incidents/:id" element={<IncidentDetailPage />} />
  </FlatRoutes>
);
```

Add a sidebar link in `packages/app/src/components/Root/Root.tsx`:

```tsx
import ShieldIcon from '@material-ui/icons/Security';

<SidebarItem icon={ShieldIcon} to="rampart" text="rampart" />
```

## Configuration

No frontend configuration is required. `RampartClient` resolves the
engine path through Backstage's `discoveryApiRef`, which expands
`rampart` to `${backend.baseUrl}/api/rampart` — same-origin against
Backstage, no browser-side CORS handshake, no `rampart.baseUrl` shipped
to the bundle.

The engine URL lives on the backend side (see the
`@ugurcan-aytar/backstage-plugin-rampart-backend` README) under
`rampart.engine.baseUrl`.

## What ships

The package exports:

- `IncidentDashboardPage` and `IncidentDetailPage` — lazy routable
  extensions for the two main views.
- `IncidentDashboard`, `IncidentDetail`, `BlastRadius`,
  `ComponentCard` — bare components for embedding inside other pages
  (custom EntityPage tabs, scorecards, etc.).
- `rampartApiRef` and `RampartClient` — the typed API binding for
  consumers that want to call the engine directly.
- `rampartRouteRef`, `rampartIncidentRouteRef` — route refs for
  cross-plugin navigation.

The dashboard subscribes to the engine's `/v1/stream` SSE feed for
live incident updates; no polling.

## Compatibility

| Dependency | Version |
|---|---|
| `@backstage/core-plugin-api` | `^1.10.0` |
| `react` | `^18.0.0` |
| Node.js | `>=20` |

Tested against Backstage `1.30+` releases. The plugin uses the
modern frontend system (`createPlugin` + `createRoutableExtension`)
and is compatible with both `createApp` setups and the new
`createFrontendApp` flow.

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source and issues:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
