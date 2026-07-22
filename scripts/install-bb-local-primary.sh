#!/bin/sh
# Install the reproducible local-primary release services for macOS launchd.
# The release binary is copied atomically into a user-owned stable path so a
# source checkout rebuild or cleanup cannot unlink the running launchd binary.
# Credentials and the ignored plane/ instance remain outside Git.
set -eu

usage() {
  cat <<'USAGE'
usage: scripts/install-bb-local-primary.sh [--retire-legacy-dashboard]

Build target/release/bb first. The installer stages it under
~/.local/libexec/bitterblossom/bb and atomically replaces that stable binary;
launchd never executes from target/release. The optional cleanup action unloads
and removes the exact retired com.misty-step.bb-dashboard plist. Without the
flag, detection is reported and no legacy service is deleted.
USAGE
}

repo_dir=${BB_REPO_DIR:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}
plane_dir="$repo_dir/plane"
release_bin="$repo_dir/target/release/bb"
install_dir=${BB_INSTALL_DIR:-$HOME/.local/libexec/bitterblossom}
install_bin="$install_dir/bb"
previous_bin="$install_dir/bb.previous"
plist_dir="$HOME/Library/LaunchAgents"
log_dir=${BB_LOG_DIR:-$HOME/.local/state/bitterblossom}
retire_legacy_dashboard=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --retire-legacy-dashboard) retire_legacy_dashboard=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "install local-primary: unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[ -x "$release_bin" ] || { echo "install local-primary: run cargo build --release first ($release_bin)" >&2; exit 2; }
[ -f "$plane_dir/plane.toml" ] || { echo "install local-primary: missing $plane_dir/plane.toml" >&2; exit 2; }
grep -q '^dev[[:space:]]*=[[:space:]]*false' "$plane_dir/plane.toml" || { echo "install local-primary: plane must set dev = false" >&2; exit 2; }
grep -q '^allow_local_substrate[[:space:]]*=[[:space:]]*true' "$plane_dir/plane.toml" || { echo "install local-primary: plane must set allow_local_substrate = true" >&2; exit 2; }
grep -q '^bind[[:space:]]*=[[:space:]]*"127.0.0.1:7093"' "$plane_dir/plane.toml" || { echo "install local-primary: plane must set [ingress] bind = 127.0.0.1:7093" >&2; exit 2; }
mkdir -p "$install_dir" "$plist_dir" "$log_dir"

chmod_operator_env() {
  env_file=$1
  [ -f "$env_file" ] || return 0
  chmod 600 "$env_file"
}
[ -z "${BB_ENV_FILE:-}" ] || chmod_operator_env "$BB_ENV_FILE"
chmod_operator_env "$repo_dir/.env.bb"
chmod_operator_env "$repo_dir/.env.bb.local-primary"

tmp_bin=$(mktemp "$install_dir/.bb-local-primary.XXXXXX")
cleanup() {
  [ -z "${tmp_bin:-}" ] || rm -f "$tmp_bin"
}
trap cleanup EXIT INT TERM
cp "$release_bin" "$tmp_bin"
chmod 755 "$tmp_bin"
if [ -e "$install_bin" ] || [ -L "$install_bin" ]; then
  mv -f "$install_bin" "$previous_bin"
fi
mv -f "$tmp_bin" "$install_bin"
tmp_bin=

python3 - "$repo_dir" "$log_dir" "$plist_dir" "$install_bin" <<'PY'
import os
import pathlib
import plistlib
import sys
from xml.parsers.expat import ExpatError
from xml.sax.saxutils import escape
repo, log_dir, plist_dir, install_bin = map(pathlib.Path, sys.argv[1:])
source = repo / "deploy" / "launchd"
replacements = {
    "__BB_REPO_DIR__": escape(str(repo)),
    "__BB_LOG_DIR__": escape(str(log_dir)),
    "__BB_INSTALL_BIN__": escape(str(install_bin)),
}
rendered_plists = []
for name in ("com.misty-step.bb-serve", "com.misty-step.bb-plane-litestream"):
    template = (source / f"{name}.plist.template").read_text()
    rendered = template
    for placeholder, value in replacements.items():
        rendered = rendered.replace(placeholder, value)
    destination = pathlib.Path(plist_dir) / f"{name}.plist"
    temporary = destination.with_name(destination.name + ".tmp")
    temporary.write_text(rendered)
    rendered_plists.append((destination, temporary))
for destination, temporary in rendered_plists:
    try:
        with temporary.open("rb") as stream:
            plistlib.load(stream)
    except (OSError, plistlib.InvalidFileException, ExpatError, ValueError) as error:
        for _, candidate in rendered_plists:
            try:
                candidate.unlink()
            except FileNotFoundError:
                pass
        print(
            f"install local-primary: invalid launchd plist {destination}: {error}; "
            "fix the rendered XML before bootout",
            file=sys.stderr,
        )
        raise SystemExit(2)
for destination, temporary in rendered_plists:
    os.replace(temporary, destination)
PY

if [ "$(uname -s)" = "Darwin" ] && command -v plutil >/dev/null 2>&1; then
  for plist in "$plist_dir/com.misty-step.bb-serve.plist" "$plist_dir/com.misty-step.bb-plane-litestream.plist"; do
    plutil -lint "$plist" >/dev/null || {
      echo "install local-primary: invalid launchd plist $plist; fix the rendered XML before bootout" >&2
      exit 2
    }
  done
fi

uid=$(id -u)
wait_for_bootout() {
  label=$1
  attempts=0
  while [ "$attempts" -lt 20 ]; do
    if ! launchctl print "gui/$uid/$label" >/dev/null 2>&1; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 0.1
  done
  echo "install local-primary: launchd label $label did not settle after bootout; recover with launchctl print gui/$uid/$label and launchctl bootout gui/$uid/$label" >&2
  return 1
}
bootstrap_label() {
  label=$1
  plist=$2
  attempts=1
  while [ "$attempts" -le 3 ]; do
    if launchctl bootstrap "gui/$uid" "$plist"; then
      return 0
    fi
    attempts=$((attempts + 1))
    sleep 0.2
  done
  echo "install local-primary: failed to bootstrap $label; recover with launchctl bootout gui/$uid/$label then launchctl bootstrap gui/$uid $plist" >&2
  return 1
}

legacy_label=com.misty-step.bb-dashboard
legacy_plist="$plist_dir/$legacy_label.plist"
if [ -e "$legacy_plist" ]; then
  if [ "$retire_legacy_dashboard" -eq 1 ]; then
    launchctl bootout "gui/$uid/$legacy_label" 2>/dev/null || true
    wait_for_bootout "$legacy_label"
    rm -f "$legacy_plist"
    printf '%s
' "retired legacy launchd service: $legacy_label"
  else
    printf '%s
' "legacy launchd service detected: $legacy_label ($legacy_plist); rerun with --retire-legacy-dashboard to unload and remove it" >&2
  fi
fi

for label in com.misty-step.bb-serve com.misty-step.bb-plane-litestream; do
  launchctl bootout "gui/$uid/$label" 2>/dev/null || true
  wait_for_bootout "$label"
done
bootstrap_label com.misty-step.bb-serve "$plist_dir/com.misty-step.bb-serve.plist"
bootstrap_label com.misty-step.bb-plane-litestream "$plist_dir/com.misty-step.bb-plane-litestream.plist"
launchctl kickstart -k "gui/$uid/com.misty-step.bb-serve"
launchctl kickstart -k "gui/$uid/com.misty-step.bb-plane-litestream"
printf '%s
' "installed local-primary services: com.misty-step.bb-serve and com.misty-step.bb-plane-litestream (binary $install_bin)"
