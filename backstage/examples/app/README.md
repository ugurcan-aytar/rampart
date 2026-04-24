# rampart example Backstage app

The reference Backstage app that wires the three rampart plugins
(`rampart`, `rampart-backend`, `scaffolder-rampart-actions`) into a
runnable container. Used as the platform-team quickstart in the
[root README](../../../README.md#path-3--platform-team-backstage)
and as the live target for the Playwright e2e suite under `e2e/`.

## What's here

```
backstage/examples/app/
├── app-config.yaml          # rampart.engine.*, catalog locations, auth
├── packages/app/            # frontend bootstrap + sidebar wiring
├── packages/backend/        # backend bootstrap + plugin registration
├── Dockerfile               # multi-stage build → ~770 MiB image
└── package.json             # workspace root for the app
```

`app-config.yaml` is the file most operators edit: it carries
`rampart.engine.baseUrl` + `rampart.engine.authToken` for the
`rampart-backend` proxy and the `rampart.catalogSyncInterval`
consumed by `CatalogSync`. The frontend needs no rampart-specific
config — it resolves `/api/rampart` via Backstage's discoveryApi.

## Run

The container image is published with every release at
`ghcr.io/ugurcan-aytar/rampart-backstage:0.1.0`. From a fresh
checkout:

```bash
make demo-axios            # brings up the full five-service stack
open http://localhost:3000
```

To iterate locally without Docker:

```bash
yarn install
yarn workspace app start   # frontend on :3000, backend on :7007
```

## Plugin wiring

The example app demonstrates the canonical wiring for each plugin —
read its `packages/app/src/App.tsx` for the frontend integration and
`packages/backend/src/index.ts` for the backend integration. Both
files are short and meant to be copy-pasted into a real Backstage
deployment.

## License

MIT — see [LICENSE](../../../LICENSE).

Source:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
