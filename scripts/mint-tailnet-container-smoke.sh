#!/bin/sh
# Exercise the Mint boundary in the real production image. Only the external
# Tailscale/Mint network is replaced; user IDs, setpriv, filesystem modes,
# socat, curl, and both entrypoints are the shipped implementations.
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
image=${BB_MINT_SMOKE_IMAGE:-bitterblossom-mint-smoke:test}
mkdir -p "$repo_root/target"
fixture_dir=$(mktemp -d "$repo_root/target/bb-mint-container-smoke.XXXXXX")
cleanup() {
  rm -rf "$fixture_dir"
}
trap cleanup EXIT INT TERM

case "$(docker info --format '{{.Architecture}}')" in
  amd64|x86_64) target_arch=amd64 ;;
  arm64|aarch64) target_arch=arm64 ;;
  *) printf '%s\n' 'mint container smoke: unsupported Docker architecture' >&2; exit 2 ;;
esac

cat >"$fixture_dir/tailscaled" <<'SH'
#!/bin/sh
set -eu
socket=
for arg in "$@"; do
  case "$arg" in --socket=*) socket=${arg#--socket=} ;; esac
done
test -n "$socket"
: >"$socket"
chmod 0666 "$socket"
while :; do sleep 30; done
SH

cat >"$fixture_dir/tailscale" <<'SH'
#!/bin/sh
set -eu
case " $* " in
  *" up "*)
    auth_file=$(printf '%s\n' "$*" | sed -n 's/.*--auth-key=file:\([^ ]*\).*/\1/p')
    test -n "$auth_file"
    test -s "$auth_file"
    ;;
  *" nc "*)
    request=
    while IFS= read -r line; do
      request="$request
$line"
      [ "$line" = "$(printf '\r')" ] && break
    done
    printf '%s' "$request" | grep -F '/proxy/https/sanctum.tail5f5eb4.ts.net:10001/api/v1/cards?limit=1' >/dev/null
    printf '%s' "$request" | grep -F 'Authorization: Bearer __mint.powder.bitterblossom__' >/dev/null
    count_file=/tmp/bb-mint-smoke-probe-count
    count=$(cat "$count_file" 2>/dev/null || printf '0')
    count=$((count + 1))
    printf '%s' "$count" >"$count_file"
    if [ "$count" -eq 1 ]; then
      printf 'HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}'
    else
      printf 'HTTP/1.1 503 Service Unavailable\r\nContent-Length: 2\r\nConnection: close\r\n\r\n{}'
    fi
    ;;
  *) exit 2 ;;
esac
SH
chmod 0755 "$fixture_dir/tailscaled" "$fixture_dir/tailscale"

if [ "${BB_MINT_SMOKE_SKIP_BUILD:-0}" != 1 ]; then
  docker build --build-arg TARGETARCH="$target_arch" -t "$image" "$repo_root" >/dev/null
fi

output=$(docker run --rm \
  -e BB_MINT_TAILNET_AUTHKEY=tskey-auth-container-smoke-sentinel \
  -e BB_MINT_STARTUP_TIMEOUT_SECONDS=10 \
  -e BB_MINT_PROBE_INTERVAL_SECONDS=60 \
  -e POWDER_API_KEY=direct-powder-container-sentinel \
  -e POWDER_API_BASE_URL=https://direct-powder.invalid \
  -v "$fixture_dir/tailscaled:/usr/local/bin/tailscaled:ro" \
  -v "$fixture_dir/tailscale:/usr/local/bin/tailscale:ro" \
  "$image" /bin/sh -ec '
    test "$(id -un)" = bb
    test "$(id -u)" != 0
    test "$HOME" = /home/bb
    test "$POWDER_API_KEY" = __mint.powder.bitterblossom__
    test "$POWDER_API_BASE_URL" = http://127.0.0.1:4949/proxy/https/sanctum.tail5f5eb4.ts.net:10001
    test -z "${BB_MINT_TAILNET_AUTHKEY:-}"
    test ! -e /run/bb-mint/authkey
    test ! -r /run/bb-mint/tailscaled.sock
    test -w "$BB_PLANE_DIR"
    grep -Eq "^NoNewPrivs:[[:space:]]+1$" /proc/self/status
    grep -Eq "^Cap(Inh|Prm|Eff|Bnd|Amb):[[:space:]]+0000000000000000$" /proc/self/status
    test "$(grep -Ec "^Cap(Inh|Prm|Eff|Bnd|Amb):[[:space:]]+0000000000000000$" /proc/self/status)" -eq 5
    printf "%s\n" "mint container smoke: pass"
  ' 2>&1) || {
    printf '%s\n' "$output" >&2
    exit 1
  }

case "$output" in
  *tskey-auth-container-smoke-sentinel*|*direct-powder-container-sentinel*)
    printf '%s\n' 'mint container smoke: secret sentinel leaked' >&2
    exit 1
    ;;
esac
printf '%s\n' "$output" | grep -F 'mint container smoke: pass' >/dev/null

set +e
failure_started=$(date +%s)
failure_output=$(docker run --rm \
  -e BB_MINT_TAILNET_AUTHKEY=tskey-auth-container-smoke-sentinel \
  -e BB_MINT_STARTUP_TIMEOUT_SECONDS=10 \
  -e BB_MINT_PROBE_INTERVAL_SECONDS=1 \
  -e BB_MINT_SHUTDOWN_GRACE_SECONDS=1 \
  -e POWDER_API_KEY=direct-powder-container-sentinel \
  -e POWDER_API_BASE_URL=https://direct-powder.invalid \
  -v "$fixture_dir/tailscaled:/usr/local/bin/tailscaled:ro" \
  -v "$fixture_dir/tailscale:/usr/local/bin/tailscale:ro" \
  "$image" /bin/sh -ec '
    trap "" TERM
    /bin/sh -c "trap \"\" TERM; sleep 30" &
    wait
  ' 2>&1)
failure_status=$?
failure_elapsed=$(( $(date +%s) - failure_started ))
set -e
[ "$failure_status" -ne 0 ] || {
  printf '%s\n' 'mint container smoke: runtime capability loss did not stop bb' >&2
  exit 1
}
[ "$failure_elapsed" -le 5 ] || {
  printf 'mint container smoke: shutdown exceeded grace window (%ss)\n' "$failure_elapsed" >&2
  exit 1
}
printf '%s\n' "$failure_output" \
  | grep -F 'Mint Powder capability probe failed; stopping bb' >/dev/null
case "$failure_output" in
  *tskey-auth-container-smoke-sentinel*|*direct-powder-container-sentinel*)
    printf '%s\n' 'mint container smoke: secret sentinel leaked on failure path' >&2
    exit 1
    ;;
esac
printf '%s\n' 'mint container smoke: pass'
