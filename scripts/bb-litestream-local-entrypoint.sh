#!/bin/sh
# launchd entrypoint for the separate local Litestream sidecar.
# It writes the heartbeat only after a successful sync; no credential value is
# committed or printed. The sidecar does not run bb serve.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
plane_dir="$repo_dir/plane"

. "$repo_dir/scripts/bb-operator-env.sh"
bb_source_operator_env "$repo_dir" || { echo "bb litestream: failed to load operator env" >&2; exit 2; }

: "${LITESTREAM_REPLICA_URL:?set LITESTREAM_REPLICA_URL in operator-local environment}"
export BB_PLANE_DIR="$plane_dir"
export BB_LITESTREAM_DB_PATH="$plane_dir/.bb/plane.db"
export BB_LITESTREAM_CONFIG="$plane_dir/.bb/litestream-local.yml"
export BB_LITESTREAM_HEARTBEAT_PATH="$plane_dir/.bb/backup-last-success"
export BB_LITESTREAM_SOCKET_PATH="$plane_dir/.bb/litestream-local.sock"
export BB_LITESTREAM_REQUIRED=1
exec "$repo_dir/scripts/bb-litestream-entrypoint.sh" tail -f /dev/null
