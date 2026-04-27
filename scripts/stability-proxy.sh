#!/usr/bin/env bash
set -euo pipefail

PROXY="${PROXY:-http://172.16.7.31:7777}"
DURATION="${DURATION:-600}"
CONCURRENCY="${CONCURRENCY:-50}"
TOTAL="${TOTAL:-80}"
PAUSE="${PAUSE:-2}"
BENCH_SCRIPT="${BENCH_SCRIPT:-scripts/bench-proxy.sh}"
SSH_OPTS="${SSH_OPTS:--o BatchMode=yes}"
LOCAL_HOST="${LOCAL_HOST:-test}"
RELAY_HOST="${RELAY_HOST:-sh}"
EXIT_HOST="${EXIT_HOST:-jp}"
LOCAL_SERVICE="${LOCAL_SERVICE:-detour2-local}"
RELAY_SERVICE="${RELAY_SERVICE:-detour2-relay}"
EXIT_SERVICE="${EXIT_SERVICE:-detour2-server}"

start_epoch="$(date +%s)"
end_epoch="$((start_epoch + DURATION))"
result_file="$(mktemp)"
trap 'rm -f "$result_file"' EXIT

run_batch() {
    PROXY="$PROXY" CONCURRENCY="$CONCURRENCY" TOTAL="$TOTAL" bash "$BENCH_SCRIPT"
}

service_errors() {
    local host="$1"
    local service="$2"
    ssh $SSH_OPTS "$host" \
        "sudo journalctl -u '$service' --since '@$start_epoch' --no-pager | grep -Ei 'fatal error|concurrent map|panic|fatal|error|timeout|refused|reset|close 1006|connect error|failed|connect, failed' | tail -80" || true
}

service_status() {
    local host="$1"
    local service="$2"
    ssh $SSH_OPTS "$host" \
        "systemctl --no-pager --full status '$service' | sed -n '1,12p'; printf '\nconnections:\n'; ss -tan | awk 'NR==1 || /:7777/ {print}' | head -80" || true
}

printf 'stability start=%s duration=%ss proxy=%s concurrency=%s total_per_batch=%s\n' \
    "$(date -Is)" "$DURATION" "$PROXY" "$CONCURRENCY" "$TOTAL"

batch=0
while (( "$(date +%s)" < end_epoch )); do
    batch=$((batch + 1))
    now="$(date -Is)"
    output="$(run_batch)"
    printf '\n== batch %d %s ==\n%s\n' "$batch" "$now" "$output"
    printf '%s\n' "$output" | awk -v batch="$batch" -F'[ =]' '
        /^samples=/ {
            for (idx=1; idx<=NF; idx+=2) values[$idx]=$(idx+1)
            samples=values["samples"]; ok=values["ok"]; fail=values["fail"]
        }
        /^latency / {
            for (idx=2; idx<=NF; idx+=2) values[$idx]=$(idx+1)
            printf "%d %s %s %s %s %s %s %s\n", batch, samples, ok, fail, values["p50"], values["p90"], values["p95"], values["max"]
        }
    ' >>"$result_file"
    sleep "$PAUSE"
done

printf '\n== aggregate ==\n'
awk '
    {
        batches++
        samples += $2
        ok += $3
        fail += $4
        p50[batches] = $5 + 0
        p90[batches] = $6 + 0
        p95[batches] = $7 + 0
        max[batches] = $8 + 0
    }
    END {
        if (batches == 0) {
            print "no batches"
            exit
        }
        for (idx=1; idx<=batches; idx++) {
            p50_sum += p50[idx]
            p90_sum += p90[idx]
            p95_sum += p95[idx]
            if (max[idx] > max_seen) max_seen = max[idx]
        }
        printf "batches=%d samples=%d ok=%d fail=%d\n", batches, samples, ok, fail
        printf "avg_p50=%.3f avg_p90=%.3f avg_p95=%.3f max_seen=%.3f\n", p50_sum/batches, p90_sum/batches, p95_sum/batches, max_seen
    }
' "$result_file"

printf '\n== service status ==\n'
service_status "$LOCAL_HOST" "$LOCAL_SERVICE"
service_status "$RELAY_HOST" "$RELAY_SERVICE"
service_status "$EXIT_HOST" "$EXIT_SERVICE"

printf '\n== suspicious logs since start ==\n'
service_errors "$LOCAL_HOST" "$LOCAL_SERVICE"
service_errors "$RELAY_HOST" "$RELAY_SERVICE"
service_errors "$EXIT_HOST" "$EXIT_SERVICE"