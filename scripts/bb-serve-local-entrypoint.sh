#!/bin/sh
# launchd entrypoint for the Bitterblossom local-primary service.
# Secrets stay in operator-local env files; this script only selects the stable
# installed release binary, durable plane, and canonical loopback bind.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
plane_dir="$repo_dir/plane"

source_env() {
  env_file=$1
  [ -f "$env_file" ] || return 0
  _env_xtrace=0
  case "$-" in *x*) _env_xtrace=1; set +x ;; esac
  if . "$env_file" >/dev/null 2>&1; then
    _env_status=0
  else
    _env_status=$?
  fi
  [ "$_env_xtrace" -eq 1 ] && set -x
  [ "$_env_status" -eq 0 ] || { echo "bb local-primary: failed to source operator env" >&2; exit 2; }
}

if [ -n "${BB_ENV_FILE:-}" ]; then
  source_env "$BB_ENV_FILE"
fi
source_env "$repo_dir/.env.bb"
source_env "$repo_dir/.env.bb.local-primary"

bb_bin="${BB_LOCAL_PRIMARY_BIN:-$HOME/.local/libexec/bitterblossom/bb}"
[ -x "$bb_bin" ] || { echo "bb local-primary: installed release binary missing at $bb_bin; run scripts/install-bb-local-primary.sh" >&2; exit 2; }
[ -f "$plane_dir/plane.toml" ] || { echo "bb local-primary: plane config missing at $plane_dir/plane.toml" >&2; exit 2; }

export BB_INGRESS_BIND="${BB_INGRESS_BIND:-127.0.0.1:7093}"
exec "$bb_bin" --config "$plane_dir" serve
