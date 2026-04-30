#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'USAGE'
Usage:
  bash deploy.sh HOST server LISTEN PASSWORD
  bash deploy.sh HOST relay  LISTEN PASSWORD UPSTREAM
  bash deploy.sh HOST local  LISTEN PASSWORD UPSTREAM
    bash deploy.sh HOST http   PASSWORD UPSTREAM
    bash deploy.sh HOST http   LISTEN PASSWORD UPSTREAM
    bash deploy.sh HOST socks5 PASSWORD UPSTREAM
    bash deploy.sh HOST socks5 LISTEN PASSWORD UPSTREAM

Examples:
  bash deploy.sh jp server tcp://0.0.0.0:7777 jy230101
  bash deploy.sh sh relay tcp://0.0.0.0:7777 jy230101 tcp://8.216.49.30:7777
  bash deploy.sh test local http://0.0.0.0:7777 jy230101 tcp:://47.116.180.26:7777
    bash deploy.sh test http jy230101 tcp://47.116.180.26:7777
    bash deploy.sh test socks5 jy230101 tcp://47.116.180.26:7777

Environment:
  GOOS/GOARCH          build target, defaults to linux/amd64
    POOL                 websocket connections per next hop, defaults to 64
    HTTP_PORT            default port for http role, defaults to 7777
    SOCKS5_PORT          default port for socks5 role, defaults to 7776
    DNS                  comma-separated DNS servers for exit-node target dials
    METRICS              optional metrics listen address, exposes /debug/metrics
    SERVICE_SUFFIX       optional systemd service suffix for parallel chains
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
deploy_role="$role"
unit_role="$role"
force_proto=""
if [[ "$role" == "http" || "$role" == "local-http" ]]; then
    deploy_role="local"
    unit_role="http"
    service="detour2-http"
    force_proto="http"
    if [[ $# -eq 4 ]]; then
        listen="http://0.0.0.0:${HTTP_PORT:-7777}"
        password="$3"
        upstream="$4"
    else
        listen="$(normalize_url "$3")"
        password="$4"
        upstream="${5:-}"
    fi
elif [[ "$role" == "socks5" || "$role" == "local-socks5" ]]; then
    deploy_role="local"
    unit_role="socks5"
    service="detour2-socks5"
    force_proto="socks5"
    if [[ $# -eq 4 ]]; then
        listen="socks5://0.0.0.0:${SOCKS5_PORT:-7776}"
        password="$3"
        upstream="$4"
    else
        listen="$(normalize_url "$3")"
        password="$4"
        upstream="${5:-}"
    fi
else
    listen="$(normalize_url "$3")"
    password="$4"
    upstream="${5:-}"
    service="detour2-$role"
fi
pool="${POOL:-64}"
dns="${DNS:-}"
metrics="${METRICS:-}"
service_suffix="${SERVICE_SUFFIX:-}"
if [[ -n "$service_suffix" ]]; then
    case "$service_suffix" in
        *[!a-zA-Z0-9_.@-]*) fail "SERVICE_SUFFIX contains unsupported characters: $service_suffix" ;;
    esac
    service="${service}-${service_suffix}"
    unit_role="${unit_role}-${service_suffix}"
fi
args=()

case "$deploy_role" in
    server)
        [[ "$listen" == tcp://* ]] || fail "server LISTEN must be tcp://host:port"
        args=(server -l "$listen" -p "$password")
        if [[ -n "$dns" ]]; then
            args+=(-dns "$dns")
        fi
        if [[ -n "$metrics" ]]; then
            args+=(-metrics "$metrics")
        fi
        ;;
    relay)
        [[ "$listen" == tcp://* ]] || fail "relay LISTEN must be tcp://host:port"
        [[ -n "$upstream" ]] || fail "relay requires UPSTREAM"
        args=(relay -l "$listen" -p "$password" -r "$(to_ws_url "$upstream")" -pool "$pool")
        if [[ -n "$dns" ]]; then
            args+=(-dns "$dns")
        fi
        if [[ -n "$metrics" ]]; then
            args+=(-metrics "$metrics")
        fi
        ;;
    local)
        [[ -n "$upstream" ]] || fail "local requires UPSTREAM"
        proto="${force_proto:-${LOCAL_PROTO:-socks5}}"
        case "$listen" in
            http://*)
                [[ -z "$force_proto" || "$force_proto" == "http" ]] || fail "$role LISTEN must be tcp:// or socks5://"
                proto="http"
                listen="tcp://${listen#http://}"
                ;;
            socks5://*)
                [[ -z "$force_proto" || "$force_proto" == "socks5" ]] || fail "$role LISTEN must be tcp:// or http://"
                proto="socks5"
                listen="tcp://${listen#socks5://}"
                ;;
            tcp://*) ;;
            *) fail "local LISTEN must be tcp://, http://, or socks5://" ;;
        esac
        args=(local -l "$listen" -p "$password" -r "$(to_ws_url "$upstream")" -t "$proto" -pool "$pool")
        if [[ -n "$metrics" ]]; then
            args+=(-metrics "$metrics")
        fi
        ;;
    *)
        usage
        fail "role must be server, relay, local, http, socks5, local-http, or local-socks5"
        ;;
esac

unit_args="$(join_systemd_args "${args[@]}")"
unit="$(render_unit "$unit_role" "$unit_args")"
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