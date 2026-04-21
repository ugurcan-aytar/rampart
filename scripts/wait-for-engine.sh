#!/usr/bin/env bash
# wait-for-engine.sh — blocks until engine /healthz returns 200 (or
# timeout). Scenario scripts source this so they don't race against
# container boot.
set -euo pipefail

engine_url="${RAMPART_ENGINE_URL:-http://localhost:8080}"
timeout_seconds="${WAIT_TIMEOUT_SECONDS:-30}"

deadline=$(( $(date +%s) + timeout_seconds ))
while (( $(date +%s) < deadline )); do
  if curl -sSf "$engine_url/healthz" > /dev/null 2>&1; then
    exit 0
  fi
  sleep 1
done
echo "wait-for-engine: timed out after ${timeout_seconds}s waiting for $engine_url/healthz" >&2
exit 1
