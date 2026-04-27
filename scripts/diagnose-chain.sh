#!/usr/bin/env bash
set -euo pipefail

PROXY="${PROXY:-http://172.16.7.31:7777}"
LOCAL_HOST="${LOCAL_HOST:-test}"
RELAY_HOST="${RELAY_HOST:-sh}"
EXIT_HOST="${EXIT_HOST:-jp}"
LOCAL_SERVICE="${LOCAL_SERVICE:-detour2-local}"
RELAY_SERVICE="${RELAY_SERVICE:-detour2-relay}"
EXIT_SERVICE="${EXIT_SERVICE:-detour2-server}"
LOCAL_TO_RELAY_URL="${LOCAL_TO_RELAY_URL:-http://47.116.180.26:7777/}"
RELAY_TO_EXIT_URL="${RELAY_TO_EXIT_URL:-http://8.216.49.30:7777/}"
REPEAT="${REPEAT:-5}"
CONNECT_TIMEOUT="${CONNECT_TIMEOUT:-5}"
MAX_TIME="${MAX_TIME:-15}"
IP_URL="${IP_URL:-http://ip.me}"
FONT_URL="${FONT_URL:-https://fonts.gstatic.com/s/roboto/v30/KFOmCnqEu92Fr1Mu4mxK.woff2}"
DNS_NAME="${DNS_NAME:-fonts.gstatic.com}"
DNS_SERVERS="${DNS_SERVERS:-8.8.8.8 1.1.1.1}"
SSH_OPTS="${SSH_OPTS:--o BatchMode=yes}"

section() {
    printf '\n== %s ==\n' "$1"
}

proxy_probe() {
    local label="$1"
    local url="$2"
    local idx
    for idx in $(seq 1 "$REPEAT"); do
        curl -x "$PROXY" -o /dev/null -sS \
            --connect-timeout "$CONNECT_TIMEOUT" -m "$MAX_TIME" \
            -w "$label-$idx total=%{time_total} connect=%{time_connect} starttransfer=%{time_starttransfer} err=%{errormsg}\n" \
            "$url" || true
    done
}

remote_curl() {
    local host="$1"
    local label="$2"
    local url="$3"
    ssh $SSH_OPTS "$host" \
        "curl -o /dev/null -sS --connect-timeout '$CONNECT_TIMEOUT' -m '$MAX_TIME' -w '$label total=%{time_total} connect=%{time_connect} starttransfer=%{time_starttransfer} remote=%{remote_ip} err=%{errormsg}\\n' '$url'" || true
}

service_status() {
    local host="$1"
    local service="$2"
    ssh $SSH_OPTS "$host" "systemctl is-active '$service'; systemctl --no-pager --full status '$service' | sed -n '1,12p'" || true
}

service_errors() {
    local host="$1"
    local service="$2"
    ssh $SSH_OPTS "$host" \
        "sudo journalctl -u '$service' --since '10 minutes ago' --no-pager | grep -Ei 'panic|fatal|error|timeout|failed|203\\.208\\.40\\.2' | tail -80" || true
}

dns_probe() {
    local server
    for server in $DNS_SERVERS; do
        ssh $SSH_OPTS "$EXIT_HOST" "printf '$server '; dig '@$server' '$DNS_NAME' +short | head -5 | tr '\n' ' '; printf '\n'" || true
    done
}

section "proxy timings via $PROXY"
proxy_probe ip "$IP_URL"
proxy_probe fonts "$FONT_URL"

section "per-hop health"
remote_curl "$LOCAL_HOST" "local-to-relay" "$LOCAL_TO_RELAY_URL"
remote_curl "$RELAY_HOST" "relay-to-exit" "$RELAY_TO_EXIT_URL"
remote_curl "$EXIT_HOST" "exit-ip" "$IP_URL"

section "exit DNS for $DNS_NAME"
dns_probe

section "systemd status"
service_status "$LOCAL_HOST" "$LOCAL_SERVICE"
service_status "$RELAY_HOST" "$RELAY_SERVICE"
service_status "$EXIT_HOST" "$EXIT_SERVICE"

section "recent suspicious logs"
service_errors "$LOCAL_HOST" "$LOCAL_SERVICE"
service_errors "$RELAY_HOST" "$RELAY_SERVICE"
service_errors "$EXIT_HOST" "$EXIT_SERVICE"