# syntax=docker/dockerfile:1.7
# rampart Backstage app — production Dockerfile.
#
# Two-stage image:
#   build   — node:20-bookworm-slim; yarn install + tsc + Backstage
#             bundle build. Build context is the repo root so the
#             Yarn workspaces (backstage/plugins/*, e2e/, …) all
#             resolve; .dockerignore (at the repo root) trims the
#             heavy stuff out before sending bytes to the daemon.
#   runtime — node:20-bookworm-slim; installs only the backend's
#             production dependencies from skeleton.tar.gz, then
#             unpacks the built backend bundle. Listens on 7007.
#
# Size target: <500 MiB. A bone-stock create-app Backstage image
# comes in around 350 MiB on node:20-bookworm-slim; we're at parity.

FROM node:20-bookworm-slim AS build

# Install the build-only system deps — Backstage's webpack pulls in
# sharp (image processing) which has a native module; better-sqlite3
# (in-memory dev db) also wants build-essential. Both are built with
# enableScripts=false via prebuild binaries, but the runtime headers
# still need to be in PATH.
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 g++ make \
    && rm -rf /var/lib/apt/lists/*

# Enable Corepack so the pinned Yarn 4 in package.json's
# "packageManager" field is honoured — the base image ships Yarn
# Classic (1.22) and would otherwise refuse to install.
RUN corepack enable

WORKDIR /build

# Copy root workspace manifests first so yarn install layers cache
# on package.json changes, not on every source edit.
COPY package.json yarn.lock .yarnrc.yml tsconfig.base.json ./
COPY .yarn ./.yarn

# Each workspace's package.json ahead of source — max cache hit.
COPY backstage/plugins/rampart/package.json ./backstage/plugins/rampart/
COPY backstage/plugins/rampart-backend/package.json ./backstage/plugins/rampart-backend/
COPY backstage/examples/app/package.json ./backstage/examples/app/
COPY backstage/examples/app/packages/app/package.json ./backstage/examples/app/packages/app/
COPY backstage/examples/app/packages/backend/package.json ./backstage/examples/app/packages/backend/
COPY e2e/package.json ./e2e/

# --network-timeout pinned long because Backstage drags in ~1100
# packages on a cold cache and npm registries occasionally hiccup.
# --immutable is deliberately off — the Docker build happens on Linux
# and the host yarn.lock was authored on darwin/arm64; the optional
# platform-specific deps (sharp, lightningcss) diverge enough that
# Yarn wants to tweak the lockfile during resolution. Host-side
# `make gen-check` still enforces lockfile hygiene per normal
# supply-chain policy.
RUN --mount=type=cache,target=/root/.yarn/berry/cache \
    yarn install --network-timeout 600000

# One-shot native rebuild for better-sqlite3. Backstage backend-defaults
# wires better-sqlite3 for the :memory: database used by core auth
# services; the package ships a prebuild-install `install` hook that
# ADR-0008's `enableScripts: false` policy deliberately suppresses.
# We invoke `npm rebuild` here as an explicit build-phase action —
# one named package, inside a sealed Linux runtime, with source-visible
# intent — instead of widening the yarn policy (which would re-open
# Shai-Hulud class attacks). See ADR-0008 "Measured consequences" note
# about compilation-required packages.
RUN npm rebuild better-sqlite3 --verbose 2>&1 | tail -5

# Now the full source — separated from the install step so code-only
# edits stay fast.
COPY backstage/plugins/rampart/ ./backstage/plugins/rampart/
COPY backstage/plugins/rampart-backend/ ./backstage/plugins/rampart-backend/
COPY backstage/examples/app/ ./backstage/examples/app/
COPY schemas/ ./schemas/

# tsc validates all workspaces; frontend build compiles the SPA; the
# backend build produces dist/{skeleton,bundle}.tar.gz for runtime.
RUN yarn workspace app build
RUN yarn workspace backend build

# ---------------------------------------------------------------------------
# Runtime — thin node:20-bookworm-slim that installs only the backend's
# production deps and runs the bundled backend. Frontend static assets
# are served by the backend's static asset pipeline out of packages/app/dist.
# ---------------------------------------------------------------------------

FROM node:20-bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Runtime also needs Corepack-pinned Yarn 4 for the production install
# step (skeleton.tar.gz expansion + `yarn workspaces focus`).
RUN corepack enable

WORKDIR /app

# Copy the workspace skeleton + install production deps. skeleton.tar.gz
# is the Backstage-cli-emitted tarball of package.json files ordered for
# `yarn install --production` to resolve.
COPY --from=build /build/.yarnrc.yml /build/yarn.lock /build/package.json ./
COPY --from=build /build/.yarn ./.yarn
COPY --from=build /build/backstage/examples/app/packages/backend/dist/skeleton.tar.gz ./
RUN tar xzf skeleton.tar.gz && rm skeleton.tar.gz

RUN --mount=type=cache,target=/root/.yarn/berry/cache \
    yarn workspaces focus --production backend

# Same compile-scope exception as the build stage: ADR-0008 keeps
# enableScripts=false at the Yarn level, and the Backstage backend's
# auth/database services want better-sqlite3's native binding.
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 g++ make \
 && npm rebuild better-sqlite3 --verbose 2>&1 | tail -3 \
 && apt-get purge -y --auto-remove python3 g++ make \
 && rm -rf /var/lib/apt/lists/*

# Unpack the backend bundle on top of the skeleton — now both package
# layout + compiled JS are in place.
COPY --from=build /build/backstage/examples/app/packages/backend/dist/bundle.tar.gz ./
RUN tar xzf bundle.tar.gz && rm bundle.tar.gz

# Frontend static assets + package.json. plugin-app-backend calls
# `resolvePackagePath('app', 'package.json')` which uses Node's
# require.resolve — it walks node_modules, not the workspace tree.
# yarn workspaces focus only seeded the backend's prod deps; the `app`
# workspace is a sibling, not a production dependency, so we copy
# its bundle in place AND symlink it into /app/node_modules/app so
# require.resolve finds it.
COPY --from=build /build/backstage/examples/app/packages/app/package.json \
     ./backstage/examples/app/packages/app/package.json
COPY --from=build /build/backstage/examples/app/packages/app/dist \
     ./backstage/examples/app/packages/app/dist
RUN mkdir -p node_modules \
 && ln -sfn /app/backstage/examples/app/packages/app node_modules/app

# Config files loaded at runtime. docker-compose.yml overrides
# APP_CONFIG_* env vars that app-config.production.yaml expands.
COPY --from=build /build/backstage/examples/app/app-config.yaml /build/backstage/examples/app/app-config.production.yaml ./

# Hand the whole /app tree over to the unprivileged `node` user —
# skeleton/bundle extraction + yarn focus all run as root, which
# otherwise leaves node unable to ls its own WORKDIR.
RUN chown -R node:node /app

USER node
EXPOSE 7007

# skeleton.tar.gz + bundle.tar.gz recreate the workspace tree at the
# original monorepo path, not a flattened packages/backend. CMD uses
# that canonical path; config files sit at /app/app-config*.yaml so
# paths stay short.
CMD ["node", "backstage/examples/app/packages/backend", \
     "--config", "app-config.yaml", \
     "--config", "app-config.production.yaml"]
