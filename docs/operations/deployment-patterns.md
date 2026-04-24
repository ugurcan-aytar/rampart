# Deployment patterns

Three supported shapes for putting the rampart engine in front of
real traffic. Each section lists the env vars and app-config keys
that matter, what CORS + auth look like, and the failure modes to
watch for.

The patterns are orthogonal to the engine's feature surface — they
only differ in who terminates the browser connection and who mints
the engine's JWT.

## Pattern 1 — Backstage-fronted (recommended)

The `rampart-backend` Backstage plugin mounts `/api/rampart/*` on
the Backstage backend and forwards calls to the engine. The
rampart frontend plugin resolves `${backend.baseUrl}/api/rampart`
via Backstage's `discoveryApiRef`, so browser requests are
same-origin against Backstage — **no browser-side CORS handshake is
ever issued**. The engine's CORS middleware stays at its default
(wildcard) because it never sees a cross-origin request in this
topology.

```
 ┌─────────────┐   same-origin   ┌────────────────────┐   service-to-service   ┌────────┐
 │  browser    │ ──────────────▶ │  Backstage backend │ ────────────────────▶ │ engine │
 │ (Backstage  │                 │  (rampart-backend  │    Authorization:     │ :8080  │
 │  frontend)  │ ◀────────────── │   proxy + sync)    │      Bearer <JWT>     │        │
 └─────────────┘                 └────────────────────┘                       └────────┘
```

**Operator-facing config (app-config.yaml):**

```yaml
rampart:
  engine:
    baseUrl: ${RAMPART_ENGINE_URL:-http://engine:8080}
    authToken: ${RAMPART_ENGINE_AUTH_TOKEN}  # required if engine has auth on
  catalogSyncInterval: 30m
```

**Engine-side env:**

```shell
RAMPART_AUTH_ENABLED=true
RAMPART_AUTH_SIGNING_KEY=<HS256 secret that mints RAMPART_ENGINE_AUTH_TOKEN>
# RAMPART_CORS_* left unset — the engine sees no browser traffic.
```

**Failure mode to watch:** if the Backstage `auth` provider is
misconfigured, the `rampart-backend` proxy still forwards but the
Backstage backend itself rejects the request upstream. Confirm with
`curl -sSf http://localhost:7007/api/rampart/_health` — the
rampart-backend-local endpoint bypasses upstream and exposes only
the proxy's liveness.

## Pattern 2 — Standalone engine (no Backstage)

The engine is served directly to browsers. Used by CLI-only setups
and prototype integrations that don't use Backstage.

```
 ┌──────────────┐   cross-origin fetch   ┌────────┐
 │   frontend   │ ────────────────────▶ │ engine │
 │ (your UI)    │        OPTIONS /v1/*  │ :8080  │
 │              │ ◀────────────────────  │        │
 └──────────────┘  Access-Control-…      └────────┘
```

**Required env on the engine:**

```shell
RAMPART_CORS_ORIGINS=https://app.example.com,https://admin.example.com
RAMPART_AUTH_ENABLED=true
RAMPART_AUTH_SIGNING_KEY=<HS256 secret or RS256 PEM>
RAMPART_AUTH_ALGORITHM=HS256   # or RS256
RAMPART_AUTH_AUDIENCE=rampart-prod
```

Your frontend must mint a JWT per user (via your IdP or the
engine's `/v1/auth/token` for internal tooling) and attach it as
`Authorization: Bearer <token>` on every call.

**Failure mode to watch:** browsers cache the
`Access-Control-Allow-Origin` decision aggressively. If you widen
or narrow `RAMPART_CORS_ORIGINS`, clients may need a hard refresh
before the new policy takes effect.

## Pattern 3 — Reverse proxy (nginx, traefik, envoy)

A reverse proxy owns the TLS terminator and the origin allow-list;
the engine sits on a private network. Used when ops wants a single
ingress point for multiple tools.

```
 ┌──────────────┐  HTTPS   ┌────────┐   cleartext, same-network   ┌────────┐
 │   browser    │ ──────▶ │ nginx  │ ────────────────────────▶ │ engine │
 │              │         │  :443  │   Authorization: Bearer     │ :8080  │
 └──────────────┘         └────────┘                             └────────┘
```

**Nginx upstream sketch:**

```nginx
upstream rampart_engine {
  server engine.internal:8080;
}

server {
  listen 443 ssl http2;
  server_name rampart.example.com;

  location /v1/ {
    # Proxy-side CORS: engine's policy stays at default because we
    # fully own the browser-origin contract here.
    if ($request_method = OPTIONS) {
      add_header Access-Control-Allow-Origin "https://app.example.com" always;
      add_header Access-Control-Allow-Headers "Authorization, Content-Type" always;
      add_header Access-Control-Allow-Methods "GET, POST, OPTIONS" always;
      return 204;
    }
    proxy_pass http://rampart_engine;
    proxy_http_version 1.1;
    proxy_set_header Connection "";        # SSE needs keep-alive
    proxy_buffering off;                   # stream events as they arrive
    proxy_read_timeout 1d;                 # /v1/stream stays open
  }
}
```

**Engine-side env:**

```shell
RAMPART_AUTH_ENABLED=true
RAMPART_AUTH_SIGNING_KEY=<…>
# RAMPART_CORS_* left unset — nginx owns the preflight.
```

**Failure mode to watch:** SSE (`/v1/stream`) traverses proxies
unhappily by default. The `proxy_buffering off` + `proxy_read_timeout`
+ `Connection ""` triplet above is what keeps the stream alive past
the default 60 s nginx timeout. Traefik's equivalent is
`middlewares.retry` off + `forwardAuth.headers.Connection` preserved.
