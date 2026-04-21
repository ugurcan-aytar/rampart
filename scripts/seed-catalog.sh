#!/usr/bin/env bash
# seed-catalog.sh — register 3 demo components and ingest their SBOMs.
#
# Called by each scenario script before publishing IoCs. Idempotent:
# re-running is safe (UpsertComponent is keyed on ref, and SBOMs with
# the same content get fresh ULIDs but no duplicate incidents open
# until an IoC is published).
set -euo pipefail

engine_url="${RAMPART_ENGINE_URL:-http://localhost:8080}"
mock_npm_url="${MOCK_NPM_URL:-http://localhost:8081}"

log() { printf '[seed] %s\n' "$*" >&2; }

register_component() {
  local ref="$1" name="$2" owner="$3"
  log "register $ref (owner=$owner)"
  curl -sSf -X POST "$engine_url/v1/components" \
    -H 'Content-Type: application/json' \
    -d "{\"ref\":\"$ref\",\"kind\":\"Component\",\"namespace\":\"default\",\"name\":\"$name\",\"owner\":\"$owner\"}" \
    > /dev/null
}

ingest_sbom() {
  local ref="$1" fixture="$2"
  local escaped_ref
  # Path-escape the ref (kind:Component/default/x has forward slashes).
  escaped_ref=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=''))" "$ref")
  local lockfile_b64
  lockfile_b64=$(curl -sSf "$mock_npm_url/-/lockfile/$fixture" | base64 | tr -d '\n')
  log "ingest sbom ref=$ref fixture=$fixture ($(echo -n "$lockfile_b64" | wc -c | tr -d ' ') b64 bytes)"
  curl -sSf -X POST "$engine_url/v1/components/$escaped_ref/sboms" \
    -H 'Content-Type: application/json' \
    -d "{\"ecosystem\":\"npm\",\"sourceFormat\":\"npm-package-lock-v3\",\"content\":\"$lockfile_b64\"}" \
    > /dev/null
}

log "waiting for engine at $engine_url"
"$(dirname "$0")/wait-for-engine.sh"

# Three components with Backstage-shaped refs. web-app + billing carry
# the compromised axios fixture; reporting stays clean.
register_component "kind:Component/default/web-app"   "web-app"   "team-platform"
register_component "kind:Component/default/billing"   "billing"   "team-payments"
register_component "kind:Component/default/reporting" "reporting" "team-data"

ingest_sbom "kind:Component/default/web-app"   "web-app"
ingest_sbom "kind:Component/default/billing"   "billing"
ingest_sbom "kind:Component/default/reporting" "reporting"

log "done — 3 components registered, 3 SBOMs ingested"
