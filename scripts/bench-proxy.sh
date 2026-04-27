#!/usr/bin/env bash
set -euo pipefail

PROXY="${PROXY:-http://172.16.7.31:7777}"
CONCURRENCY="${CONCURRENCY:-50}"
TOTAL="${TOTAL:-80}"
CONNECT_TIMEOUT="${CONNECT_TIMEOUT:-5}"
MAX_TIME="${MAX_TIME:-20}"
VERBOSE="${VERBOSE:-0}"

URLS=(
    "https://fonts.gstatic.com/s/roboto/v30/KFOmCnqEu92Fr1Mu4mxK.woff2"
    "https://i.ytimg.com/generate_204"
    "https://www.gstatic.com/generate_204"
    "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg"
)

raw="$(mktemp)"
times="$(mktemp)"
sorted="$(mktemp)"
trap 'rm -f "$raw" "$times" "$sorted"' EXIT

export PROXY CONNECT_TIMEOUT MAX_TIME

for idx in $(seq 1 "$TOTAL"); do
    printf '%s\n' "${URLS[$(((idx - 1) % ${#URLS[@]}))]}"
done | xargs -r -P"$CONCURRENCY" -I{} sh -c '
    curl -x "$PROXY" -o /dev/null -sS --connect-timeout "$CONNECT_TIMEOUT" -m "$MAX_TIME" \
        -w "url=%{url_effective}\ttotal=%{time_total}\tstart=%{time_starttransfer}\tcode=%{http_code}\terr=%{errormsg}\n" \
        "$1" 2>/dev/null || true
' sh {} >"$raw"

if [[ "$VERBOSE" == "1" ]]; then
    cat "$raw"
fi

awk -F'\t' '
    {
        total=""; code=""; err=""; url=""
        for (idx=1; idx<=NF; idx++) {
            if ($idx ~ /^url=/) { url=$idx; sub(/^url=/, "", url) }
            if ($idx ~ /^total=/) { total=$idx; sub(/^total=/, "", total) }
            if ($idx ~ /^code=/) { code=$idx; sub(/^code=/, "", code) }
            if ($idx ~ /^err=/) { err=$idx; sub(/^err=/, "", err) }
        }
        if (total == "") next
        samples++
        print total > times_file
        if (err == "" && code ~ /^2/) {
            ok++
        } else {
            fail++
            if (err == "") err="http_" code
            errors[err]++
        }
        host=url
        sub(/^https?:\/\//, "", host)
        sub(/\/.*$/, "", host)
        by_host[host]++
    }
    END {
        printf "samples=%d ok=%d fail=%d\n", samples, ok, fail
        for (err in errors) printf "error[%s]=%d\n", err, errors[err]
        for (host in by_host) printf "host[%s]=%d\n", host, by_host[host]
    }
' times_file="$times" "$raw"

sort -n "$times" >"$sorted"

percentile() {
    local pct="$1"
    local count="$2"
    local index
    index=$(( (count * pct + 99) / 100 ))
    if (( index < 1 )); then
        index=1
    fi
    sed -n "${index}p" "$sorted"
}

count="$(wc -l <"$sorted")"
if (( count == 0 )); then
    echo "latency no samples"
    exit 0
fi

min="$(sed -n '1p' "$sorted")"
max="$(tail -n 1 "$sorted")"
p50="$(percentile 50 "$count")"
p90="$(percentile 90 "$count")"
p95="$(percentile 95 "$count")"
p99="$(percentile 99 "$count")"

printf 'latency min=%.3f p50=%.3f p90=%.3f p95=%.3f p99=%.3f max=%.3f\n' \
    "$min" "$p50" "$p90" "$p95" "$p99" "$max"