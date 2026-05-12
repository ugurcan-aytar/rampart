# syntax=docker/dockerfile:1.7
# rampart Backstage app — production Dockerfile.
#
# Three-stage image:
#   build   — node:20-bookworm-slim; yarn install + tsc + Backstage
#             bundle build. Build context is the repo root so Yarn
#             workspaces (backstage/plugins/*, e2e/, …) all resolve;
#             the root .dockerignore trims the heavy stuff out before
#             bytes hit the daemon.
#   prune   — installs ONLY the backend's production dependencies
#             (`yarn workspaces focus --production backend`) under
#             a node-owned WORKDIR. Native rebuild of better-sqlite3
#             happens here inside a single root-scoped RUN that
#             installs its toolchain, rebuilds, and purges — no apt
#             packages leak into the runtime layer.
#   runtime — node:20-bookworm-slim; copies the pruned tree as-is.
#             Everything is already owned by `node` (the prune stage
#             switched USER before its COPYs), so the runtime never
#             runs `chown -R` — that single command was previously
#             a 393 MiB layer that duplicated the whole /app tree.

# ---------------------------------------------------------------------------
# BUILD — yarn install (dev + prod), tsc, frontend SPA bundle, backend
# skeleton + bundle tarballs.
# ---------------------------------------------------------------------------
FROM node:26-bookworm-slim AS build

RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 g++ make \
    && rm -rf /var/lib/apt/lists/*

# Corepack-pinned Yarn 4; the base image's Yarn Classic would refuse
# to install against a v4 lockfile.
RUN corepack enable

WORKDIR /build

# Root workspace manifests + tsconfig first — maximises layer cache
# hits when only source files change.
COPY package.json yarn.lock .yarnrc.yml tsconfig.base.json ./
COPY backstage/plugins/rampart/package.json ./backstage/plugins/rampart/
COPY backstage/plugins/rampart-backend/package.json ./backstage/plugins/rampart-backend/
COPY backstage/examples/app/package.json ./backstage/examples/app/
COPY backstage/examples/app/packages/app/package.json ./backstage/examples/app/packages/app/
COPY backstage/examples/app/packages/backend/package.json ./backstage/examples/app/packages/backend/
COPY e2e/package.json ./e2e/

# --immutable is off deliberately: the Docker build happens on Linux
# while the host yarn.lock was authored on darwin/arm64; platform-
# specific optional deps (sharp, lightningcss) diverge enough that
# Yarn wants to tweak the lockfile during resolution. Host-side
# `make gen-check` still enforces lockfile hygiene under normal
# supply-chain policy.
RUN --mount=type=cache,target=/root/.yarn/berry/cache \
    yarn install --network-timeout 600000

# Full source after install — code edits don't bust the install layer.
COPY backstage/plugins/rampart/ ./backstage/plugins/rampart/
COPY backstage/plugins/rampart-backend/ ./backstage/plugins/rampart-backend/
COPY backstage/examples/app/ ./backstage/examples/app/
COPY schemas/ ./schemas/

RUN yarn workspace app build
RUN yarn workspace backend build

# ---------------------------------------------------------------------------
# PRUNE — install ONLY backend production deps into a node-owned tree.
# Rebuild better-sqlite3 in the same stage so the build toolchain stays
# out of the runtime image.
# ---------------------------------------------------------------------------
FROM node:26-bookworm-slim AS prune

RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 g++ make \
    && rm -rf /var/lib/apt/lists/*
RUN corepack enable

# Pre-create /app with node ownership so every subsequent write is
# node-owned from the start — no chown -R penalty layer in runtime.
RUN mkdir /app && chown node:node /app
WORKDIR /app
USER node

# Manifests + skeleton (just package.json files) so yarn focus can
# resolve the workspace graph.
COPY --from=build --chown=node:node /build/yarn.lock /build/package.json /build/.yarnrc.yml ./
COPY --from=build --chown=node:node /build/backstage/examples/app/packages/backend/dist/skeleton.tar.gz ./
RUN tar xzf skeleton.tar.gz && rm skeleton.tar.gz

# No cache mount — BuildKit creates the mount target but not its
# parent dirs (/home/node/.yarn/berry) with the right ownership,
# which leaves yarn unable to mkdir its `index` sibling next to
# `cache`. For a one-shot prune install the cache doesn't buy
# much; re-downloads are amortised across builds via build-stage
# cache mount already.
RUN yarn workspaces focus --production backend

# ADR-0008 exception: better-sqlite3 ships its Linux/arm64 binary
# via a prebuild-install postinstall hook, which yarn's
# `enableScripts: false` policy suppresses. We rebuild once here,
# scoped to one package, inside this sealed stage. The toolchain
# (python3/g++/make) never makes it into runtime.
RUN npm rebuild better-sqlite3 --verbose 2>&1 | tail -3

# ---------------------------------------------------------------------------
# RUNTIME — thin image, pruned tree already node-owned, no build tools.
# ---------------------------------------------------------------------------
FROM node:26-bookworm-slim AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Pre-create /app owned by node so the COPY + tar below land on a
# node-writable target — empty dir chown is a <1 KiB layer, unlike
# the 393 MiB `chown -R` pattern the old Dockerfile used.
RUN mkdir /app && chown node:node /app
WORKDIR /app
USER node

# Copy the fully-pruned /app wholesale. Because the prune stage did
# its writes as USER node, everything is already owned by node —
# no `chown -R` penalty layer.
COPY --from=prune --chown=node:node /app/ ./

# Backend bundle on top of the skeleton + pruned node_modules.
COPY --from=build --chown=node:node /build/backstage/examples/app/packages/backend/dist/bundle.tar.gz ./
RUN tar xzf bundle.tar.gz --no-same-owner --no-same-permissions --touch --overwrite \
 && rm bundle.tar.gz

# Frontend static assets. plugin-app-backend calls
# `resolvePackagePath('app', 'package.json')` which uses Node's
# require.resolve — it walks node_modules, not the workspace tree.
# The `app` workspace is a sibling, not a production dependency, so
# we copy its bundle + package.json and symlink into node_modules/app
# so require.resolve finds it. Source maps are stripped to shave
# ~7 MiB of dev-only overhead.
COPY --from=build --chown=node:node /build/backstage/examples/app/packages/app/package.json \
     ./backstage/examples/app/packages/app/package.json
COPY --from=build --chown=node:node /build/backstage/examples/app/packages/app/dist \
     ./backstage/examples/app/packages/app/dist
RUN find ./backstage/examples/app/packages/app/dist -name '*.map' -delete \
 && mkdir -p node_modules \
 && ln -sfn /app/backstage/examples/app/packages/app node_modules/app

# Config loaded at runtime; docker-compose.yml overrides the
# APP_CONFIG_* env vars that app-config.production.yaml expands.
COPY --from=build --chown=node:node /build/backstage/examples/app/app-config.yaml \
     /build/backstage/examples/app/app-config.production.yaml ./

EXPOSE 7007

# skeleton.tar.gz + bundle.tar.gz recreate the workspace tree at the
# original monorepo path, not a flattened packages/backend. CMD uses
# that canonical path; config files sit at /app/app-config*.yaml so
# relative paths stay short.
CMD ["node", "backstage/examples/app/packages/backend", \
     "--config", "app-config.yaml", \
     "--config", "app-config.production.yaml"]
