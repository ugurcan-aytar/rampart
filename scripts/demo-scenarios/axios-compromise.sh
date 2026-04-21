#!/usr/bin/env bash
# axios-compromise.sh — 2026-03-31 axios maintainer account compromise
# scenario, end-to-end against a running demo stack.
#
# Flow:
#   1. seed 3 components + SBOMs (web-app + billing carry axios@1.11.0;
#      reporting is clean on simple-webapp.json)
#   2. fetch the canned IoC from mock-npm-registry and POST it to the
#      engine's /v1/iocs — forward match opens 2 incidents
#   3. GET /v1/incidents — expect 2 pending incidents
#
# The slack-notifier container is already subscribed to /v1/stream and
# will log a dry-run JSON payload for each incident.opened event. Tail
# its output with `docker compose logs slack-notifier` to see it.
set -euo pipefail

engine_url="${RAMPART_ENGINE_URL:-http://localhost:8080}"
mock_npm_url="${MOCK_NPM_URL:-http://localhost:8081}"
here="$(cd "$(dirname "$0")/.." && pwd)"

log() { printf '[axios-compromise] %s\n' "$*" >&2; }

log "seeding catalog (3 components + SBOMs)"
"$here/seed-catalog.sh"

log "fetching axios IoC from mock-npm-registry"
ioc=$(curl -sSf "$mock_npm_url/-/iocs" | python3 -c 'import json,sys;print(json.dumps(json.load(sys.stdin)["axios_compromise"][0]))')

log "publishing IoC to engine"
echo "$ioc" | curl -sSf -X POST "$engine_url/v1/iocs" \
  -H 'Content-Type: application/json' \
  --data @- > /dev/null

# Give SSE a moment to propagate before we read — bus is in-process but
# the subscribers loop through a channel.
sleep 1

log "GET /v1/incidents — expecting 2 opened"
incidents=$(curl -sSf "$engine_url/v1/incidents")
count=$(echo "$incidents" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)["items"]))')
log "incidents opened: $count"

echo "$incidents" | python3 -m json.tool
