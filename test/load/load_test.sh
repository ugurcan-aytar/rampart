#!/usr/bin/env bash
# load_test.sh — v0.2.0 load test orchestrator.
#
# Runs end-to-end:
#   1. fresh postgres + engine via docker compose
#   2. parallel ingest of fixtures (components → SBOMs → IoCs)
#   3. query latency sampling (blast-radius + incident detail)
#   4. results.json + human-readable summary on stdout
#
# Re-runnable: each invocation does `make demo-down` first to wipe
# state. Targets:
#   - blast-radius p95 < 500ms
#   - detail endpoint p95 < 200ms
#   - ingest 10k SBOMs < 5min
#
# Usage:
#   bash test/load/load_test.sh [--components 10000] [--iocs 500] \
#                               [--blast-samples 1000] [--detail-samples 200] \
#                               [--workers 10]
set -euo pipefail

# Force C locale so awk's printf %.2f emits dot decimals + jq parses
# the synthesised JSON. Turkish (and many European) locales default
# to comma decimal, which produces invalid JSON like
# "p50_ms":0,00.
export LC_ALL=C
export LANG=C

# --- Config ---------------------------------------------------------
COMPONENTS=${COMPONENTS:-10000}
IOCS=${IOCS:-500}
SNAPSHOTS=${SNAPSHOTS:-200}
BLAST_SAMPLES=${BLAST_SAMPLES:-1000}
DETAIL_SAMPLES=${DETAIL_SAMPLES:-200}
WORKERS=${WORKERS:-10}
ENGINE_URL=${ENGINE_URL:-http://localhost:8080}
FIXTURES=${FIXTURES:-test/load/fixtures}
RESULTS=${RESULTS:-test/load/results.json}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --components)     COMPONENTS=$2; shift 2 ;;
    --iocs)           IOCS=$2; shift 2 ;;
    --snapshots)      SNAPSHOTS=$2; shift 2 ;;
    --blast-samples)  BLAST_SAMPLES=$2; shift 2 ;;
    --detail-samples) DETAIL_SAMPLES=$2; shift 2 ;;
    --workers)        WORKERS=$2; shift 2 ;;
    *) echo "unknown flag: $1" >&2; exit 1 ;;
  esac
done

log() { printf '[load-test] %s\n' "$*" >&2; }

# split_into_chunks <input> <output-prefix> <num-chunks>
#
# macOS's BSD `split` lacks GNU coreutils' `-n l/<chunks>` mode, so
# we round-robin lines into <num-chunks> output files via awk —
# portable across Linux + macOS, no GNU-only flag required.
split_into_chunks() {
  local input=$1 prefix=$2 chunks=$3
  awk -v prefix="$prefix" -v n="$chunks" '
    {
      out = sprintf("%s%02d", prefix, NR % n)
      print >> out
    }
  ' "$input"
}

# --- Step 0: regenerate fixtures if size mismatch -------------------
need_regen=true
if [[ -f "$FIXTURES/manifest.json" ]]; then
  current_components=$(jq -r '.components' "$FIXTURES/manifest.json")
  if [[ "$current_components" == "$COMPONENTS" ]]; then
    need_regen=false
    log "fixtures already at $COMPONENTS components — reusing"
  fi
fi
if $need_regen; then
  log "generating fixtures: $COMPONENTS components, $IOCS IoCs, $SNAPSHOTS snapshots"
  go run ./test/load/generate_fixtures.go \
    -out "$FIXTURES" \
    -components "$COMPONENTS" \
    -iocs "$IOCS" \
    -snapshots "$SNAPSHOTS"
fi

# --- Step 1: fresh stack --------------------------------------------
log "tearing down any prior state"
make demo-down >/dev/null 2>&1 || true

log "starting postgres + engine (no Backstage, no native, no demo-app)"
docker compose up -d postgres engine >/dev/null

log "waiting for /healthz"
for i in {1..60}; do
  if curl -sf "$ENGINE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 1
  if [[ $i -eq 60 ]]; then
    echo "engine never came up" >&2
    docker compose logs engine | tail -40 >&2
    exit 1
  fi
done

# --- Step 2: bulk-ingest --------------------------------------------
ingest_start=$(date +%s)

ingest_components_chunk() {
  while IFS= read -r line; do
    curl -sf -X POST "$ENGINE_URL/v1/components" \
      -H 'Content-Type: application/json' \
      -d "$line" >/dev/null
  done
}
export -f ingest_components_chunk
export ENGINE_URL

mkdir -p "$FIXTURES/.chunks"
rm -f "$FIXTURES/.chunks"/*

log "ingesting $COMPONENTS components ($WORKERS parallel workers)"
split_into_chunks "$FIXTURES/components.jsonl" "$FIXTURES/.chunks/comp-" "$WORKERS"
for chunk in "$FIXTURES/.chunks"/comp-*; do
  bash -c "cat '$chunk' | ingest_components_chunk" &
done
wait
components_done=$(date +%s)
log "components done in $((components_done - ingest_start))s"

ingest_sboms_chunk() {
  local idx
  while IFS= read -r idx; do
    local ref="kind:Component/default/svc-${idx}"
    local escaped
    escaped=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=''))" "$ref")
    curl -sf -X POST "$ENGINE_URL/v1/components/$escaped/sboms" \
      -H 'Content-Type: application/json' \
      --data-binary "@$FIXTURES/sboms/${idx}.json" >/dev/null
  done
}
export -f ingest_sboms_chunk
export FIXTURES

log "ingesting $COMPONENTS SBOMs ($WORKERS parallel workers)"
seq -f '%05g' 0 $((COMPONENTS - 1)) > "$FIXTURES/.chunks/sbom-indices"
split_into_chunks "$FIXTURES/.chunks/sbom-indices" "$FIXTURES/.chunks/sbom-chunk-" "$WORKERS"
for chunk in "$FIXTURES/.chunks"/sbom-chunk-*; do
  bash -c "cat '$chunk' | ingest_sboms_chunk" &
done
wait
sboms_done=$(date +%s)
log "SBOMs done in $((sboms_done - components_done))s"

ingest_iocs_chunk() {
  while IFS= read -r line; do
    curl -sf -X POST "$ENGINE_URL/v1/iocs" \
      -H 'Content-Type: application/json' \
      -d "$line" >/dev/null
  done
}
export -f ingest_iocs_chunk

log "ingesting $IOCS IoCs (matcher fan-out per IoC; lower parallelism)"
ioc_workers=4
split_into_chunks "$FIXTURES/iocs.jsonl" "$FIXTURES/.chunks/ioc-" "$ioc_workers"
for chunk in "$FIXTURES/.chunks"/ioc-*; do
  bash -c "cat '$chunk' | ingest_iocs_chunk" &
done
wait

ingest_done=$(date +%s)
total_ingest=$((ingest_done - ingest_start))
log "all ingestion done in ${total_ingest}s"

# --- Step 3: query latency sampling ---------------------------------
log "collecting incident IDs for detail-endpoint sampling"
incident_ids=$(curl -sf "$ENGINE_URL/v1/incidents?limit=10000" | jq -r '.items[].id')
total_incidents=$(echo "$incident_ids" | grep -c '.' || echo 0)
log "engine has $total_incidents incidents"

if [[ $total_incidents -eq 0 ]]; then
  log "ERROR: no incidents to sample — IoC matching produced zero?" >&2
  docker compose logs engine | tail -40 >&2
  exit 1
fi

# Sample BLAST_SAMPLES blast-radius calls — each posts a single IoC
# pulled at random from iocs.jsonl. Time the round-trip with %{time_total}.
#
# Pre-load all IoC payloads into a bash array so the hot loop is just
# array-index + curl — no per-call sed re-parse of a 500-line file.
# Also: --max-time 30 hard-caps any pathological hang, and we tolerate
# single-call curl failures (record 0 + continue) so one transient
# error can't void a 1000-call sample run via `set -e`.
log "blast-radius sampling: $BLAST_SAMPLES calls"
blast_log="$FIXTURES/.chunks/blast-times.txt"
rm -f "$blast_log"
mapfile -t ioc_array < "$FIXTURES/iocs.jsonl"
ioc_count=${#ioc_array[@]}
blast_failures=0
for ((i = 0; i < BLAST_SAMPLES; i++)); do
  ioc="${ioc_array[$((RANDOM % ioc_count))]}"
  body=$(printf '{"iocs":[%s]}' "$ioc")
  if t=$(curl -sf --max-time 30 -o /dev/null -w '%{time_total}\n' \
      -X POST "$ENGINE_URL/v1/blast-radius" \
      -H 'Content-Type: application/json' \
      -d "$body"); then
    echo "$t" >> "$blast_log"
  else
    blast_failures=$((blast_failures + 1))
  fi
done
if [[ $blast_failures -gt 0 ]]; then
  log "blast-radius sampling completed with $blast_failures/$BLAST_SAMPLES failures"
fi

log "incident-detail sampling: $DETAIL_SAMPLES calls"
detail_log="$FIXTURES/.chunks/detail-times.txt"
rm -f "$detail_log"
mapfile -t incident_array <<< "$incident_ids"
sample_size=${#incident_array[@]}
detail_failures=0
for ((i = 0; i < DETAIL_SAMPLES; i++)); do
  id="${incident_array[$((RANDOM % sample_size))]}"
  if t=$(curl -sf --max-time 30 -o /dev/null -w '%{time_total}\n' \
      "$ENGINE_URL/v1/incidents/$id/detail"); then
    echo "$t" >> "$detail_log"
  else
    detail_failures=$((detail_failures + 1))
  fi
done
if [[ $detail_failures -gt 0 ]]; then
  log "incident-detail sampling completed with $detail_failures/$DETAIL_SAMPLES failures"
fi

# --- Step 4: percentile computation ---------------------------------
percentiles_for() {
  local file=$1
  awk '{print $1*1000}' "$file" \
    | sort -n \
    | awk '
        { a[NR]=$1 }
        END {
          n = NR
          p50  = a[int(n*0.50)]
          p95  = a[int(n*0.95)]
          p99  = a[int(n*0.99)]
          sum = 0
          for (i=1; i<=n; i++) sum += a[i]
          printf "{\"count\":%d,\"p50_ms\":%.2f,\"p95_ms\":%.2f,\"p99_ms\":%.2f,\"avg_ms\":%.2f}",
                 n, p50, p95, p99, sum/n
        }'
}

blast_stats=$(percentiles_for "$blast_log")
detail_stats=$(percentiles_for "$detail_log")

# --- Step 5: results.json --------------------------------------------
cat > "$RESULTS" <<EOF
{
  "ts": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "fixtures": {
    "components": $COMPONENTS,
    "iocs": $IOCS,
    "snapshots": $SNAPSHOTS
  },
  "ingest_seconds": {
    "components": $((components_done - ingest_start)),
    "sboms": $((sboms_done - components_done)),
    "iocs": $((ingest_done - sboms_done)),
    "total": $total_ingest
  },
  "incidents_opened": $total_incidents,
  "blast_radius": $blast_stats,
  "incident_detail": $detail_stats
}
EOF

log "results written to $RESULTS"
jq . "$RESULTS" >&2

# --- Step 6: target check -------------------------------------------
fail=0
blast_p95=$(jq -r '.blast_radius.p95_ms' "$RESULTS")
detail_p95=$(jq -r '.incident_detail.p95_ms' "$RESULTS")
ingest_min=$(awk -v s="$total_ingest" 'BEGIN{print s/60}')

ok_or_fail() {
  local label="$1" actual="$2" target="$3" cmp="$4"
  if awk -v a="$actual" -v t="$target" "BEGIN{exit !($cmp)}"; then
    log "PASS $label: $actual (target $target)"
  else
    log "FAIL $label: $actual (target $target)"
    fail=1
  fi
}

ok_or_fail "blast-radius p95 < 500ms" "$blast_p95" "500" "a < t"
ok_or_fail "incident-detail p95 < 200ms" "$detail_p95" "200" "a < t"
ok_or_fail "ingest < 5min" "$ingest_min" "5" "a < t"

rm -rf "$FIXTURES/.chunks"

if [[ $fail -ne 0 ]]; then
  log "one or more targets missed — see results.json"
  exit 2
fi
log "all targets met"
