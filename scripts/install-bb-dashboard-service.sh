#!/bin/sh
# Install the local read-only dashboard review service:
#   bb serve on 127.0.0.1:7091, exposed to the tailnet on HTTPS :7443.
set -eu

cd "$(dirname "$0")/.."
ROOT=$(pwd -P)

LABEL=${BB_DASHBOARD_LABEL:-com.misty-step.bb-dashboard}
PORT=${BB_DASHBOARD_PORT:-7091}
TAILSCALE_HTTPS_PORT=${BB_DASHBOARD_TAILSCALE_HTTPS_PORT:-7443}
PLANE=${BB_DASHBOARD_PLANE:-"$HOME/.local/share/bitterblossom/bb-dashboard-plane"}
STATE_DIR=${BB_DASHBOARD_STATE_DIR:-"$HOME/.local/state/bitterblossom"}
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
BIN=${BB_DASHBOARD_BIN:-"$ROOT/target/debug/bb"}

if [ ! -x "$BIN" ]; then
  cargo build --locked
fi

mkdir -p "$PLANE" "$STATE_DIR" "$HOME/Library/LaunchAgents"

if [ ! -f "$PLANE/plane.toml" ]; then
  cp -R "$ROOT/examples/local-plane/agents" "$PLANE/agents"
  cp -R "$ROOT/examples/local-plane/tasks" "$PLANE/tasks"
  cp "$ROOT/examples/local-plane/plane.toml" "$PLANE/plane.toml"
  rm -rf "$PLANE/.bb"
  "$BIN" --config "$PLANE" run hello --payload '{"ok":true}' --json >/dev/null
fi

cat > "$PLIST" <<EOF_PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$LABEL</string>
  <key>WorkingDirectory</key>
  <string>$ROOT</string>
  <key>ProgramArguments</key>
  <array>
    <string>$BIN</string>
    <string>--config</string>
    <string>$PLANE</string>
    <string>serve</string>
  </array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>BB_INGRESS_BIND</key>
    <string>127.0.0.1:$PORT</string>
    <key>PATH</key>
    <string>/Users/phaedrus/.cargo/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>$STATE_DIR/bb-dashboard.out.log</string>
  <key>StandardErrorPath</key>
  <string>$STATE_DIR/bb-dashboard.err.log</string>
</dict>
</plist>
EOF_PLIST

uid=$(id -u)
launchctl bootout "gui/$uid" "$PLIST" >/dev/null 2>&1 || launchctl remove "$LABEL" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$uid" "$PLIST"
launchctl enable "gui/$uid/$LABEL"
launchctl kickstart -k "gui/$uid/$LABEL"

if command -v tailscale >/dev/null 2>&1; then
  tailscale serve --yes --bg --https="$TAILSCALE_HTTPS_PORT" "http://127.0.0.1:$PORT" >/dev/null
fi

printf 'Dashboard service installed\n'
printf '  launchd: %s\n' "$LABEL"
printf '  plane:   %s\n' "$PLANE"
printf '  local:   http://127.0.0.1:%s/\n' "$PORT"
printf '  tailnet: https://serenity.tail5f5eb4.ts.net:%s/\n' "$TAILSCALE_HTTPS_PORT"
