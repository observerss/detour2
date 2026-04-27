#!/usr/bin/env bash
set -euo pipefail

BIN="${BIN:-./dist/detour}"
PASSWORD="${PASSWORD:-password}"
POOL="${POOL:-64}"
LOCAL_ADDR="${LOCAL_ADDR:-127.0.0.1:3810}"
MIDDLE_ADDR="${MIDDLE_ADDR:-127.0.0.1:3811}"
EXIT_ADDR="${EXIT_ADDR:-127.0.0.1:3812}"

if [[ ! -x "$BIN" ]]; then
    go build -o dist/detour .
fi

pids=()
cleanup() {
    if [[ ${#pids[@]} -gt 0 ]]; then
        kill "${pids[@]}" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

"$BIN" relay -l "tcp://$EXIT_ADDR" -p "$PASSWORD" &
pids+=("$!")

"$BIN" relay -l "tcp://$MIDDLE_ADDR" -r "ws://$EXIT_ADDR/ws" -p "$PASSWORD" -pool "$POOL" &
pids+=("$!")

"$BIN" local -l "tcp://$LOCAL_ADDR" -r "ws://$MIDDLE_ADDR/ws" -p "$PASSWORD" -t socks5 -pool "$POOL" &
pids+=("$!")

echo "local socks5 proxy: $LOCAL_ADDR"
echo "middle relay:       $MIDDLE_ADDR -> ws://$EXIT_ADDR/ws"
echo "exit relay:         $EXIT_ADDR"
echo "try: curl --socks5-hostname $LOCAL_ADDR https://example.com/"

wait