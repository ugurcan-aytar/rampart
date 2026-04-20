# rampart example Backstage app

Minimal shell for demonstrating the three rampart plugins end-to-end.
Full app structure (`packages/app`, `packages/backend`, React bootstrap,
Webpack config) lands in Adım 7 when the demo stack needs it.

Today this directory owns one file of substance: `app-config.yaml`,
which both the frontend plugin (via `rampart.baseUrl`) and the backend
plugin (via `rampart.baseUrl` + `rampart.catalogSyncInterval`) consume.
Wire-up instructions for `packages/backend/src/index.ts` and
`packages/app/src/App.tsx` are inline in each plugin's README.
