#!/usr/bin/env bash
# shai-hulud.sh — the 2026-04-18 rampage-* worm scenario.
#
# Unlike axios-compromise.sh this scenario exercises the "many IoCs,
# many matches, same blast radius" shape: 5 IoCs land at once, a
# single target component (`shai-hulud-target`) has all 10 worm
# packages installed, the engine opens 5 incidents against that one
# component (one per IoC, idempotency key is (ioc, ref)).
set -euo pipefail

engine_url="${RAMPART_ENGINE_URL:-http://localhost:8080}"
mock_npm_url="${MOCK_NPM_URL:-http://localhost:8081}"
here="$(cd "$(dirname "$0")/.." && pwd)"

log() { printf '[shai-hulud] %s\n' "$*" >&2; }

log "waiting for engine"
"$here/wait-for-engine.sh"

log "registering shai-hulud-target + ingesting infected lockfile"
curl -sSf -X POST "$engine_url/v1/components" \
  -H 'Content-Type: application/json' \
  -d '{"ref":"kind:Component/default/shai-hulud-target","kind":"Component","namespace":"default","name":"shai-hulud-target","owner":"team-platform"}' \
  > /dev/null

escaped_ref=$(python3 -c 'import urllib.parse;print(urllib.parse.quote("kind:Component/default/shai-hulud-target",safe=""))')
lockfile_b64=$(curl -sSf "$mock_npm_url/-/lockfile/shai-hulud" | base64 | tr -d '\n')
curl -sSf -X POST "$engine_url/v1/components/$escaped_ref/sboms" \
  -H 'Content-Type: application/json' \
  -d "{\"ecosystem\":\"npm\",\"sourceFormat\":\"npm-package-lock-v3\",\"content\":\"$lockfile_b64\"}" \
  > /dev/null

log "publishing 5 IoC batch from mock-npm-registry"
iocs=$(curl -sSf "$mock_npm_url/-/iocs" | python3 -c 'import json,sys;print(json.dumps(json.load(sys.stdin)["shai_hulud"]))')
count=$(echo "$iocs" | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')
log "posting $count IoCs"
echo "$iocs" | RAMPART_ENGINE_URL="$engine_url" python3 -c '
import json, subprocess, sys, os
iocs = json.load(sys.stdin)
url = os.environ["RAMPART_ENGINE_URL"] + "/v1/iocs"
for ioc in iocs:
    subprocess.check_call(["curl", "-sSf", "-X", "POST", url,
                           "-H", "Content-Type: application/json",
                           "--data", json.dumps(ioc),
                           "-o", "/dev/null"])
    sys.stderr.write("published " + ioc["id"] + "\n")
'

sleep 1

log "GET /v1/incidents — expecting 5 opened (5 IoCs × 1 matched component)"
curl -sSf "$engine_url/v1/incidents" | python3 -m json.tool
