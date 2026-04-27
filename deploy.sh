#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'USAGE'
Usage:
  bash deploy.sh HOST server LISTEN PASSWORD
  bash deploy.sh HOST relay  LISTEN PASSWORD UPSTREAM
  bash deploy.sh HOST local  LISTEN PASSWORD UPSTREAM

Examples:
  bash deploy.sh jp server tcp://0.0.0.0:7777 jy230101
  bash deploy.sh sh relay tcp://0.0.0.0:7777 jy230101 tcp://8.216.49.30:7777
  bash deploy.sh test local http://0.0.0.0:7777 jy230101 tcp:://47.116.180.26:7777

Environment:
  GOOS/GOARCH          build target, defaults to linux/amd64
    POOL                 websocket connections per next hop, defaults to 64
    DNS                  comma-separated DNS servers for exit-node target dials
  DETOUR2_USER/GROUP   systemd service user/group, defaults to root/root
  LOCAL_PROTO          local proxy protocol when LISTEN is tcp://, defaults to socks5
  SSH_OPTS/SCP_OPTS    extra SSH/SCP options, defaults include BatchMode=yes
  DRY_RUN=1            render and print without SSH deployment
USAGE
}

fail() {
    echo "deploy.sh: $*" >&2
    exit 1
}

normalize_url() {
    local url="$1"
    case "$url" in
        tcp:://*) echo "tcp://${url#tcp:://}" ;;
        *) echo "$url" ;;
    esac
}

to_ws_url() {
    local url
    url="$(normalize_url "$1")"
    case "$url" in
        ws://*|wss://*) echo "$url" ;;
        tcp://*) echo "ws://${url#tcp://}/ws" ;;
        http://*) echo "ws://${url#http://}/ws" ;;
        https://*) echo "wss://${url#https://}/ws" ;;
        *) fail "unsupported upstream URL: $url" ;;
    esac
}

systemd_quote() {
    local arg="$1"
    arg="${arg//\\/\\\\}"
    arg="${arg//\"/\\\"}"
    printf '"%s"' "$arg"
}

join_systemd_args() {
    local out=""
    local arg
    for arg in "$@"; do
        out+=" $(systemd_quote "$arg")"
    done
    printf '%s' "${out# }"
}

render_unit() {
    local role="$1"
    local args="$2"
    local template="deploy/systemd/detour2.service.template"
    [[ -f "$template" ]] || fail "missing template: $template"

    local unit
    unit="$(<"$template")"
    unit="${unit//\{\{ROLE\}\}/$role}"
    unit="${unit//\{\{RUN_USER\}\}/${DETOUR2_USER:-root}}"
    unit="${unit//\{\{RUN_GROUP\}\}/${DETOUR2_GROUP:-root}}"
    unit="${unit//\{\{ARGS\}\}/$args}"
    printf '%s\n' "$unit"
}

build_binary() {
    local goos="${GOOS:-linux}"
    local goarch="${GOARCH:-amd64}"
    local bin="dist/detour2-${goos}-${goarch}"
    mkdir -p dist
    CGO_ENABLED="${CGO_ENABLED:-0}" GOOS="$goos" GOARCH="$goarch" go build -o "$bin" .
    printf '%s' "$bin"
}

[[ $# -ge 4 ]] || { usage; exit 2; }

host="$1"
role="$2"
listen="$(normalize_url "$3")"
password="$4"
upstream="${5:-}"

service="detour2-$role"
pool="${POOL:-64}"
dns="${DNS:-}"
args=()

case "$role" in
    server)
        [[ "$listen" == tcp://* ]] || fail "server LISTEN must be tcp://host:port"
        args=(server -l "$listen" -p "$password")
        if [[ -n "$dns" ]]; then
            args+=(-dns "$dns")
        fi
        ;;
    relay)
        [[ "$listen" == tcp://* ]] || fail "relay LISTEN must be tcp://host:port"
        [[ -n "$upstream" ]] || fail "relay requires UPSTREAM"
        args=(relay -l "$listen" -p "$password" -r "$(to_ws_url "$upstream")" -pool "$pool")
        if [[ -n "$dns" ]]; then
            args+=(-dns "$dns")
        fi
        ;;
    local)
        [[ -n "$upstream" ]] || fail "local requires UPSTREAM"
        proto="${LOCAL_PROTO:-socks5}"
        case "$listen" in
            http://*)
                proto="http"
                listen="tcp://${listen#http://}"
                ;;
            socks5://*)
                proto="socks5"
                listen="tcp://${listen#socks5://}"
                ;;
            tcp://*) ;;
            *) fail "local LISTEN must be tcp://, http://, or socks5://" ;;
        esac
        args=(local -l "$listen" -p "$password" -r "$(to_ws_url "$upstream")" -t "$proto" -pool "$pool")
        ;;
    *)
        usage
        fail "role must be server, relay, or local"
        ;;
esac

unit_args="$(join_systemd_args "${args[@]}")"
unit="$(render_unit "$role" "$unit_args")"
binary="$(build_binary)"

echo "host:    $host"
echo "service: $service"
echo "binary:  $binary"
echo "args:    $unit_args"

if [[ "${DRY_RUN:-0}" == "1" ]]; then
    echo
    echo "$unit"
    exit 0
fi

tmp_unit="$(mktemp)"
trap 'rm -f "$tmp_unit"' EXIT
printf '%s\n' "$unit" >"$tmp_unit"

ssh_opts="${SSH_OPTS:--o BatchMode=yes}"
scp_opts="${SCP_OPTS:--o BatchMode=yes}"
remote_bin="/tmp/detour2-${role}-$$"
remote_unit="/tmp/${service}.service.$$"

scp $scp_opts "$binary" "$host:$remote_bin"
scp $scp_opts "$tmp_unit" "$host:$remote_unit"
ssh $ssh_opts "$host" \
    "sudo install -D -m 0755 '$remote_bin' /opt/detour2/detour && \
     sudo install -D -m 0644 '$remote_unit' /etc/systemd/system/$service.service && \
     rm -f '$remote_bin' '$remote_unit' && \
     sudo systemctl daemon-reload && \
     sudo systemctl enable '$service.service' && \
     sudo systemctl restart '$service.service' && \
     sudo systemctl --no-pager --full status '$service.service'"