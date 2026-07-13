#!/bin/sh
# Establish the production plane's narrow tailnet path to Mint, then supervise
# that identity boundary and the existing Litestream/BB entrypoint as one unit.
set -eu

log() {
  printf '%s\n' "bb-mint-tailnet-entrypoint: $*" >&2
}

fail() {
  log "$*"
  exit 2
}

authkey=${BB_MINT_TAILNET_AUTHKEY:-}
if [ -z "$authkey" ]; then
  exec bb-litestream-entrypoint "$@"
fi

runtime_dir=${BB_MINT_RUNTIME_DIR:-/run/bb-mint}
socket=$runtime_dir/tailscaled.sock
authkey_file=$runtime_dir/authkey
hostname=${BB_MINT_TAILSCALE_HOSTNAME:-bitterblossom-plane}
tag=${BB_MINT_TAILSCALE_TAG:-tag:bb-plane}
mint_host=${BB_MINT_HOST:-mint.tail5f5eb4.ts.net}
mint_port=${BB_MINT_PORT:-4949}
local_port=${BB_MINT_LOCAL_PORT:-4949}
startup_timeout=${BB_MINT_STARTUP_TIMEOUT_SECONDS:-30}
probe_interval=${BB_MINT_PROBE_INTERVAL_SECONDS:-10}
shutdown_grace=${BB_MINT_SHUTDOWN_GRACE_SECONDS:-5}
powder_origin=${BB_MINT_POWDER_ORIGIN:-sanctum.tail5f5eb4.ts.net:10001}

case "$mint_host$hostname$tag" in
  *[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:-]*)
    fail "tailnet host, node name, and tag must contain only DNS/tag characters"
    ;;
esac
case "$runtime_dir" in
  /*) ;;
  *) fail "runtime path must be absolute" ;;
esac
case "$runtime_dir" in
  *[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/._-]*)
    fail "runtime path must contain only path-safe characters"
    ;;
esac
case "$powder_origin" in
  *[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789.:-]*)
    fail "Powder origin must contain only DNS and port characters"
    ;;
esac
case "$mint_port:$local_port:$startup_timeout:$probe_interval:$shutdown_grace" in
  *[!0123456789:]*) fail "ports and startup timeout must be decimal integers" ;;
esac

# The overlap deploy may still carry the legacy direct credential. Scrub it
# before any long-lived tunnel process can inherit it; bb receives only the
# Mint placeholder after the broker path is proven healthy.
unset POWDER_API_KEY POWDER_API_BASE_URL

for command in tailscaled tailscale socat curl setpriv setsid bb-litestream-entrypoint; do
  command -v "$command" >/dev/null 2>&1 || fail "$command is required"
done

umask 077
mkdir -p "$runtime_dir"
chmod 0700 "$runtime_dir"
printf '%s' "$authkey" >"$authkey_file"
unset BB_MINT_TAILNET_AUTHKEY authkey

cleanup() {
  rm -f "$authkey_file"
  children="${app_pid:-} ${forward_pid:-} ${tailscaled_pid:-}"
  for pid in $children; do
    kill -TERM -- "-$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  done
  shutdown_deadline=$(( $(date +%s) + shutdown_grace ))
  while :; do
    alive=0
    for pid in $children; do
      if kill -0 -- "-$pid" 2>/dev/null || kill -0 "$pid" 2>/dev/null; then
        alive=1
      fi
    done
    [ "$alive" -eq 0 ] && break
    if [ "$(date +%s)" -ge "$shutdown_deadline" ]; then
      for pid in $children; do
        kill -KILL -- "-$pid" 2>/dev/null || kill -KILL "$pid" 2>/dev/null || true
      done
      break
    fi
    sleep 1
  done
  for pid in $children; do
    wait "$pid" 2>/dev/null || true
  done
  app_pid= forward_pid= tailscaled_pid=
}
trap 'cleanup; exit 143' INT TERM

env -i PATH="$PATH" setsid tailscaled \
  --tun=userspace-networking \
  --state=mem: \
  --socket="$socket" \
  >/dev/null 2>&1 &
tailscaled_pid=$!

deadline=$(( $(date +%s) + startup_timeout ))
while ! env -i PATH="$PATH" tailscale --socket="$socket" up \
  --auth-key="file:$authkey_file" \
  --hostname="$hostname" \
  --advertise-tags="$tag" \
  --accept-routes=false >/dev/null 2>&1; do
  kill -0 "$tailscaled_pid" 2>/dev/null || { cleanup; fail "tailscaled exited before joining"; }
  [ "$(date +%s)" -lt "$deadline" ] || { cleanup; fail "tailnet join timed out"; }
  sleep 1
done
rm -f "$authkey_file"

env -i PATH="$PATH" setsid socat \
  "TCP-LISTEN:$local_port,bind=127.0.0.1,reuseaddr,fork,max-children=16" \
  "EXEC:tailscale --socket=$socket nc $mint_host $mint_port" \
  >/dev/null 2>&1 &
forward_pid=$!

powder_base_url="http://127.0.0.1:$local_port/proxy/https/$powder_origin"
mint_capability_is_healthy() {
  status=$(env -i PATH="$PATH" curl --disable --silent --show-error --max-time 5 \
    --output /dev/null --write-out '%{http_code}' \
    --header "Authorization: Bearer __mint.powder.bitterblossom__" \
    "$powder_base_url/api/v1/cards?limit=1") || return 1
  case "$status" in
    2??) return 0 ;;
    *) return 1 ;;
  esac
}

while ! mint_capability_is_healthy; do
  kill -0 "$tailscaled_pid" 2>/dev/null || { cleanup; fail "tailscaled exited during Mint probe"; }
  kill -0 "$forward_pid" 2>/dev/null || { cleanup; fail "Mint forward exited during probe"; }
  [ "$(date +%s)" -lt "$deadline" ] || { cleanup; fail "Mint Powder capability probe timed out"; }
  sleep 1
done

export POWDER_API_BASE_URL="$powder_base_url"
export POWDER_API_KEY="__mint.powder.bitterblossom__"
export BB_TAILNET_SSH_DIR=${BB_TAILNET_SSH_DIR:-/home/bb/.ssh}

HOME=/home/bb USER=bb LOGNAME=bb setsid setpriv \
  --reuid=bb --regid=bb --init-groups --no-new-privs \
  --bounding-set=-all --inh-caps=-all --ambient-caps=-all \
  bb-litestream-entrypoint "$@" &
app_pid=$!
next_probe=$(( $(date +%s) + probe_interval ))

while :; do
  if ! kill -0 "$app_pid" 2>/dev/null; then
    set +e
    wait "$app_pid"
    status=$?
    set -e
    cleanup
    exit "$status"
  fi
  kill -0 "$tailscaled_pid" 2>/dev/null || { cleanup; fail "tailscaled exited; stopping bb"; }
  kill -0 "$forward_pid" 2>/dev/null || { cleanup; fail "Mint forward exited; stopping bb"; }
  if [ "$(date +%s)" -ge "$next_probe" ]; then
    mint_capability_is_healthy || { cleanup; fail "Mint Powder capability probe failed; stopping bb"; }
    next_probe=$(( $(date +%s) + probe_interval ))
  fi
  sleep 1
done
