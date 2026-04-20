# @ugurcan-aytar/backstage-plugin-rampart

rampart Backstage plugin — **scaffold only**. Adım 5 fills in the real
frontend (IncidentDashboard, IncidentDetail, BlastRadius, ComponentCard).

Adım 3's purpose for this package is to be the TS-side target of the
OpenAPI codegen pipeline — `src/api/gen/schema.ts` is produced from
`schemas/openapi.yaml` via `make gen-ts`.
