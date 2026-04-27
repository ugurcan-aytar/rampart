# Auth providers

Templates for wiring an external Identity Provider (IdP) into rampart.
The engine's JWT middleware (Theme A1) verifies tokens; this doc
covers the four common IdPs operators ask about plus the generic-OIDC
fallback for everything else.

All four templates assume `RAMPART_AUTH_ENABLED=true` on the engine.
The demo `RAMPART_AUTH_SIGNING_KEY=local-dev` works only for the
self-issued `POST /v1/auth/token` flow — production deployments use
the IdP's signing key and `RAMPART_AUTH_ALGORITHM=RS256`.

## Decision flow — which IdP fits

| Question | Pick |
|---|---|
| GitHub user / repo identity already drives access? | **GitHub OAuth** |
| Microsoft 365 / Entra ID is the corporate directory? | **Azure AD** |
| Okta is the corporate SSO? | **Okta** |
| Anything else with `.well-known/openid-configuration`? | **Generic OIDC** |

The four templates differ in how Backstage discovers the IdP and
how scopes / audience translate to the engine's JWT contract. The
engine itself only cares about the resulting JWT shape.

## 1. GitHub OAuth

GitHub is the natural fit for repo-coupled access — the same OAuth
App that gates Backstage can mint tokens the engine accepts.

### Register the GitHub OAuth App

1. github.com → Settings → Developer settings → OAuth Apps → New
2. **Homepage URL**: `https://backstage.example.com`
3. **Authorization callback URL**: `https://backstage.example.com/api/auth/github/handler/frame`
4. Note the **Client ID** + generated **Client Secret**.

### Backstage `app-config.yaml`

```yaml
auth:
  environment: production
  providers:
    github:
      production:
        clientId: ${AUTH_GITHUB_CLIENT_ID}
        clientSecret: ${AUTH_GITHUB_CLIENT_SECRET}
        # Backstage signs its own JWT for downstream service calls;
        # the rampart-backend plugin re-signs that JWT with the
        # engine's RAMPART_AUTH_SIGNING_KEY before forwarding (per
        # ADR-0012 — the engine is the auth boundary).
```

### Engine env

```bash
export RAMPART_AUTH_ENABLED=true
export RAMPART_AUTH_ALGORITHM=HS256
export RAMPART_AUTH_SIGNING_KEY=<32+-byte-secret-shared-with-rampart-backend-plugin>
export RAMPART_AUTH_AUDIENCE=rampart-engine-prod
```

### Backstage rampart-backend plugin env

```bash
# Same secret as the engine; the plugin uses it to sign the
# service-to-service token it forwards on every proxied request.
export RAMPART_AUTH_SIGNING_KEY=<same-secret-as-engine>
```

## 2. Microsoft Azure AD (Entra ID)

Common for Microsoft-365 shops. Tokens are RS256 by default.

### Register the Azure AD app

1. Azure portal → Entra ID → App registrations → New
2. **Redirect URI**: `https://backstage.example.com/api/auth/microsoft/handler/frame`
3. Capture the **Application (client) ID**, **Directory (tenant) ID**,
   and a fresh client secret.
4. Under "Token configuration", add `email` + `preferred_username`
   optional claims so Backstage can render user identities.

### Backstage `app-config.yaml`

```yaml
auth:
  environment: production
  providers:
    microsoft:
      production:
        clientId: ${AUTH_MICROSOFT_CLIENT_ID}
        clientSecret: ${AUTH_MICROSOFT_CLIENT_SECRET}
        tenantId: ${AUTH_MICROSOFT_TENANT_ID}
```

### Engine env

```bash
export RAMPART_AUTH_ENABLED=true
export RAMPART_AUTH_ALGORITHM=RS256
# Public key from Azure's JWKS endpoint:
# https://login.microsoftonline.com/${TENANT_ID}/discovery/v2.0/keys
export RAMPART_AUTH_SIGNING_KEY=<PEM-encoded-public-key>
export RAMPART_AUTH_AUDIENCE=api://rampart-engine-prod
```

The `RAMPART_AUTH_AUDIENCE` value must match the `Application ID URI`
configured on the Azure app registration. Mismatch yields
`audience_mismatch` rejections (visible on `/v1/stream` as
`AuthRejected` events).

## 3. Okta

Common for Okta-fronted enterprises. Same RS256 pattern as Azure AD;
the difference is where Backstage discovers the IdP.

### Configure the Okta app

1. Okta admin → Applications → Create App Integration → OIDC / Web
2. **Sign-in redirect URI**: `https://backstage.example.com/api/auth/okta/handler/frame`
3. **Sign-out redirect URI**: `https://backstage.example.com`
4. Capture **Client ID**, **Client Secret**, **Okta domain**
   (e.g. `dev-12345.okta.com`).

### Backstage `app-config.yaml`

```yaml
auth:
  environment: production
  providers:
    okta:
      production:
        clientId: ${AUTH_OKTA_CLIENT_ID}
        clientSecret: ${AUTH_OKTA_CLIENT_SECRET}
        audience: https://${AUTH_OKTA_DOMAIN}
```

### Engine env

```bash
export RAMPART_AUTH_ENABLED=true
export RAMPART_AUTH_ALGORITHM=RS256
# Okta's JWKS is at https://${OKTA_DOMAIN}/oauth2/default/v1/keys
export RAMPART_AUTH_SIGNING_KEY=<PEM-encoded-public-key>
export RAMPART_AUTH_AUDIENCE=api://default
```

## 4. Generic OIDC

For any provider with a `.well-known/openid-configuration` endpoint
that's not in the named-provider list above (Auth0, Keycloak, Google
Workspace, Authelia, …).

### Backstage `app-config.yaml`

```yaml
auth:
  environment: production
  providers:
    oidc:
      production:
        metadataUrl: ${OIDC_METADATA_URL}
        clientId: ${OIDC_CLIENT_ID}
        clientSecret: ${OIDC_CLIENT_SECRET}
        prompt: auto
```

### Engine env

```bash
export RAMPART_AUTH_ENABLED=true
export RAMPART_AUTH_ALGORITHM=RS256
# Fetch the public key from the IdP's JWKS endpoint
# (the metadata document under "jwks_uri" tells you where).
export RAMPART_AUTH_SIGNING_KEY=<PEM-encoded-public-key>
export RAMPART_AUTH_AUDIENCE=<value-the-IdP-puts-in-aud>
```

## Verifying the wiring

After configuring any of the four templates:

```bash
# 1. Engine accepts an authenticated request.
TOKEN=<token-from-IdP-or-local-mint>
curl -X POST http://engine:8080/v1/components \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"ref":"verify-auth","ecosystem":"npm"}'
# Expected: 201 Created

# 2. Engine rejects an unauthenticated mutation.
curl -X POST http://engine:8080/v1/components -d '{}'
# Expected: 401 Unauthorized

# 3. Engine emits AuthRejected on bad tokens.
curl -X POST http://engine:8080/v1/components \
  -H "Authorization: Bearer eyJtotallybroken"
# Expected: 401, AND an AuthRejected event visible on /v1/stream
```

## Common failure modes

- **`audience_mismatch`** — `RAMPART_AUTH_AUDIENCE` doesn't equal
  the IdP-issued `aud` claim. Capture the actual `aud` from a
  decoded token (jwt.io) and align the env var.
- **`invalid_signature`** — `RAMPART_AUTH_SIGNING_KEY` is not the
  IdP's signing key. For RS256, fetch the JWKS and convert the
  matching key to PEM. For HS256, both sides must hold the same
  shared secret.
- **`expired`** — IdP-issued token TTL elapsed. Backstage refreshes
  on its own; CLI flows may need to re-mint.
- **`scope_insufficient`** — token's `scope` claim doesn't include
  `write` (mutation routes) or `admin` (token issuance). Check the
  IdP's scope mapping for the user.

## ADR references

- [ADR-0012](../decisions/0012-auth-boundary-at-engine.md) — why the
  engine is the auth boundary, not the Backstage proxy.
