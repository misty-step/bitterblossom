#!/bin/sh
# launchd entrypoint for the Bitterblossom local-primary service.
# Secrets stay in operator-local env files; this script only selects the stable
# installed release binary, durable plane, and canonical loopback bind.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
plane_dir="$repo_dir/plane"

. "$repo_dir/scripts/bb-operator-env.sh"
bb_source_operator_env "$repo_dir" || { echo "bb local-primary: failed to load operator env" >&2; exit 2; }

bb_bin="${BB_LOCAL_PRIMARY_BIN:-$HOME/.local/libexec/bitterblossom/bb}"
[ -x "$bb_bin" ] || { echo "bb local-primary: installed release binary missing at $bb_bin; run scripts/install-bb-local-primary.sh" >&2; exit 2; }
[ -f "$plane_dir/plane.toml" ] || { echo "bb local-primary: plane config missing at $plane_dir/plane.toml" >&2; exit 2; }

export BB_INGRESS_BIND="${BB_INGRESS_BIND:-127.0.0.1:7093}"
exec "$bb_bin" --config "$plane_dir" serve
