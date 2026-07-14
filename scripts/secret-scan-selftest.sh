#!/bin/sh
# Prove the custom rules in .gitleaks.toml actually fire (bitterblossom-974).
#
# Plants clearly-SYNTHETIC fixtures in a throwaway directory (never committed,
# never real credentials) and asserts every bb- rule detects its shape, plus
# that the allowlisted non-secret conventions stay quiet. A scanner whose
# rules silently rot is worse than no scanner: this ran green the day the
# TruffleHog workflow was deleted, too — nothing asserted it still existed.
#
# Fixture values are assembled from concatenated halves so this script's own
# bytes never match a rule when the repo itself is scanned.
set -eu
cd "$(dirname "$0")/.."

command -v gitleaks >/dev/null 2>&1 || {
  echo "secret-scan-selftest: gitleaks not installed (brew install gitleaks)" >&2
  exit 1
}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/bb-secret-selftest.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

fixture="$tmp/planted"
hex64="0a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f9"

# Positive fixtures: one clearly-synthetic value per custom rule.
{
  printf '%s%s\n' "sk_powder_" "AaBb0CcDd1EeFf2GgHh3IiJj4KkLl5Mm"
  printf '%s%s\n' "sk-or-v1-" "$hex64"
  printf '%s%s\n' "ops_" "eyJTeU50aEV0SWMwRmlYdFVyRTBubHkwMDAwMDAwMDAwMDA"
  printf '%s%s\n' "fo1_" "SyNtHeTiCfIxTuRe0nLy0AaBb1CcDd2EeFf3GgHh"
  printf '%s%s\n' "dop_v1_" "$hex64"
  printf '%s%s\n' "DO00" "SYNTH3T1CF1XTUR3KEY0"
  printf '%s%s\n' "tskey-auth-" "kSyNtH3t1cCNTRL-fIxTuR3fIxTuR3fIx"
  printf 'MINT_SECRET_OPENROUTER_DEFAULT%s%s\n' "=" "SyNtHeTiCmintValue123456"
  printf 'export EXA_API_KEY%s%s\n' "=" "d2f8a1b4-9c3e-4f6a-8b2d-1e5c7a9f3b6d"
  printf 'export BB_WEBHOOK_SECRET%s%s\n' "=" "Zq7Xw2Kv9Rt4Yu1Pn6Ms3Lb8Jd5Fg0Hc"
} >"$fixture"

report="$tmp/report.json"
gitleaks dir "$fixture" --config .gitleaks.toml --no-banner --redact \
  --report-format json --report-path "$report" --exit-code 0 >/dev/null 2>&1

expected="bb-powder-api-key bb-openrouter-api-key bb-1password-service-account-token \
bb-fly-token bb-digitalocean-token bb-do-spaces-key bb-tailscale-key \
bb-mint-broker-secret bb-env-secret-uuid bb-env-secret-assignment"

fired=$(python3 -c '
import json, sys
print("\n".join(sorted({f["RuleID"] for f in json.load(open(sys.argv[1]))})))
' "$report")

status=0
for rule in $expected; do
  if ! printf '%s\n' "$fired" | grep -qx "$rule"; then
    echo "secret-scan-selftest: rule $rule did NOT fire on its planted fixture" >&2
    status=1
  fi
done

# Negative fixtures: allowlisted conventions must stay quiet.
{
  printf 'BB_MINT_TAILNET_AUTHKEY%s%s\n' "=" "tskey-auth-container-smoke-sentinel"
  printf 'const API_KEY%s%s\n' " = " "process.env.OPENROUTER_API_KEY"
  printf 'AUTH_TOKEN%s%s\n' "=" '"${OPENROUTER_API_KEY}"'
} >"$fixture"

quiet=$(gitleaks dir "$fixture" --config .gitleaks.toml --no-banner --redact \
  --report-format json --report-path "$tmp/quiet.json" --exit-code 0 >/dev/null 2>&1 &&
  python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))))' "$tmp/quiet.json")
if [ "$quiet" != "0" ]; then
  echo "secret-scan-selftest: allowlisted non-secret conventions fired ($quiet findings)" >&2
  status=1
fi

if [ "$status" -ne 0 ]; then
  echo "secret-scan-selftest: FAILED" >&2
  exit 1
fi
echo "secret-scan-selftest: all $(printf '%s\n' $expected | wc -l | tr -d ' ') custom rules fire; allowlist stays quiet"
