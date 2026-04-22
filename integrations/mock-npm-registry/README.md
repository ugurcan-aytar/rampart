# mock-npm-registry

Demo-only HTTP server that stands in for `registry.npmjs.org` inside
the rampart Docker Compose stack. Serves a small set of canned
lockfile fixtures and an IoC feed so the
[`make demo-*` scenarios](https://github.com/ugurcan-aytar/rampart#path-3--platform-team-backstage)
can replay supply-chain incidents end-to-end without hitting the
real npm registry.

> This is **not** a general-purpose npm registry double. It only
> answers the specific routes the rampart demo scenarios hit. Do not
> deploy this anywhere besides the demo Compose stack.

## Routes

- `GET /healthz` — liveness probe.
- `GET /-/lockfile/{component}` — returns a canned `package-lock.json`
  for a known demo component:

  | Component | Fixture | Scenario |
  |---|---|---|
  | `web-app` | `axios-compromise.json` | axios@1.11.0 worm |
  | `billing` | `axios-compromise.json` | axios@1.11.0 worm |
  | `reporting` | `simple-webapp.json` | clean baseline |
  | `shai-hulud` | `shai-hulud.json` | 10-package worm |
  | `vercel-oauth` | `vercel-oauth.json` | leaked OAuth token |

- `GET /-/iocs` — returns the canned IoC list for the scenarios.
  Top-level keys: `axios_compromise`, `shai_hulud`, `vercel_oauth`.
  Each value is an array of
  [IoC](https://github.com/ugurcan-aytar/rampart/blob/main/schemas/openapi.yaml)
  objects ready to POST to the engine's `/v1/iocs` endpoint.

## Run standalone

```bash
go run ./cmd/mock-npm-registry -addr :8081
curl http://localhost:8081/healthz
curl http://localhost:8081/-/lockfile/web-app
curl http://localhost:8081/-/iocs | jq .axios_compromise
```

## Run as part of the demo stack

```bash
docker compose up mock-npm-registry engine
```

The scenario scripts under `scripts/demo-scenarios/*.sh` fetch the
fixtures from this service, submit them to the engine's
`/v1/components/{ref}/sboms`, then POST the matching IoC list to
`/v1/iocs` to trigger forward + retroactive matching.

## License

MIT — see [LICENSE](https://github.com/ugurcan-aytar/rampart/blob/main/LICENSE).

Source:
[github.com/ugurcan-aytar/rampart](https://github.com/ugurcan-aytar/rampart).
