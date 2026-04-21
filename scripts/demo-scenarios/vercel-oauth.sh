#!/usr/bin/env bash
# vercel-oauth.sh — the 2026-04-19 Vercel OAuth token leak scenario.
#
# Realistic target: a single leaked token package, narrow blast radius.
# The canned IoC is published as `packageVersion` kind (not
# `publisherAnomaly`) because Phase 1's matcher has publisherAnomaly
# as a deliberate no-op (see engine/internal/matcher/matcher.go). The
# feed's description field records the original detection signal —
# Phase 2 publisher graph will let us restore the original IoC kind
# when the matcher learns to reason about maintainer metadata.
set -euo pipefail

engine_url="${RAMPART_ENGINE_URL:-http://localhost:8080}"
mock_npm_url="${MOCK_NPM_URL:-http://localhost:8081}"
here="$(cd "$(dirname "$0")/.." && pwd)"

log() { printf '[vercel-oauth] %s\n' "$*" >&2; }

log "waiting for engine"
"$here/wait-for-engine.sh"

log "registering auth-gateway + ingesting compromised lockfile"
curl -sSf -X POST "$engine_url/v1/components" \
  -H 'Content-Type: application/json' \
  -d '{"ref":"kind:Component/default/auth-gateway","kind":"Component","namespace":"default","name":"auth-gateway","owner":"team-security"}' \
  > /dev/null

escaped_ref=$(python3 -c 'import urllib.parse;print(urllib.parse.quote("kind:Component/default/auth-gateway",safe=""))')
lockfile_b64=$(curl -sSf "$mock_npm_url/-/lockfile/vercel-oauth" | base64 | tr -d '\n')
curl -sSf -X POST "$engine_url/v1/components/$escaped_ref/sboms" \
  -H 'Content-Type: application/json' \
  -d "{\"ecosystem\":\"npm\",\"sourceFormat\":\"npm-package-lock-v3\",\"content\":\"$lockfile_b64\"}" \
  > /dev/null

log "publishing Vercel OAuth IoC"
ioc=$(curl -sSf "$mock_npm_url/-/iocs" | python3 -c 'import json,sys;print(json.dumps(json.load(sys.stdin)["vercel_oauth"][0]))')
echo "$ioc" | curl -sSf -X POST "$engine_url/v1/iocs" \
  -H 'Content-Type: application/json' \
  --data @- > /dev/null

sleep 1

log "GET /v1/incidents — expecting 1 opened (narrow blast radius, 1 component)"
curl -sSf "$engine_url/v1/incidents" | python3 -m json.tool
