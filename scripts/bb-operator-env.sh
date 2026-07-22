#!/bin/sh
# Shared operator environment loader for launchd and read-only operations.
# Sources only repo-root env files in a fixed order and never echoes values.

bb_source_env_file() {
  _bb_env_file=$1
  [ -f "$_bb_env_file" ] || return 0
  _bb_env_mode=$(stat -f '%Lp' "$_bb_env_file" 2>/dev/null) || {
    echo "bb operator env: cannot inspect permissions for $_bb_env_file" >&2
    return 2
  }
  case "$_bb_env_mode" in
    ''|*[!0-9]*)
      echo "bb operator env: invalid permissions for $_bb_env_file" >&2
      return 2
      ;;
  esac
  if [ "$_bb_env_mode" -gt 600 ]; then
    echo "bb operator env: $_bb_env_file must be owner-only (0600 or stricter)" >&2
    return 2
  fi
  _bb_env_xtrace=0
  case "$-" in *x*) _bb_env_xtrace=1; set +x ;; esac
  if . "$_bb_env_file" >/dev/null 2>&1; then
    _bb_env_status=0
  else
    _bb_env_status=$?
  fi
  [ "$_bb_env_xtrace" -eq 1 ] && set -x
  if [ "$_bb_env_status" -ne 0 ]; then
    echo "bb operator env: failed to source operator env" >&2
    return 2
  fi
}

bb_source_operator_env() {
  _bb_env_repo_dir=$1
  _bb_env_explicit=${BB_ENV_FILE:-}
  if [ -n "$_bb_env_explicit" ]; then
    bb_source_env_file "$_bb_env_explicit" || return
  fi
  bb_source_env_file "$_bb_env_repo_dir/.env.bb" || return
  bb_source_env_file "$_bb_env_repo_dir/.env.bb.local-primary" || return
}
