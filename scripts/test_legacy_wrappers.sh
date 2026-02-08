#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

CALLS_FILE="$TMP_DIR/calls.log"
FAKE_BB="$TMP_DIR/fake-bb"

cat > "$FAKE_BB" <<'FAKE'
#!/usr/bin/env bash
set -euo pipefail
calls_file="${BB_WRAPPER_CALLS_FILE:?missing BB_WRAPPER_CALLS_FILE}"
printf '%s\n' "$*" >> "$calls_file"
FAKE
chmod +x "$FAKE_BB"

assert_call() {
    local expected="$1"
    local actual
    actual="$(tail -n 1 "$CALLS_FILE")"
    if [[ "$actual" != "$expected" ]]; then
        echo "expected: $expected" >&2
        echo "actual:   $actual" >&2
        exit 1
    fi
}

run_case() {
    local expected="$1"
    shift
    : > "$CALLS_FILE"
    BB_BIN="$FAKE_BB" \
    BB_WRAPPER_CALLS_FILE="$CALLS_FILE" \
        "$@"
    assert_call "$expected"
}

run_case "provision --composition compositions/v2.yaml --all" \
    "$ROOT_DIR/scripts/provision.sh" --composition compositions/v2.yaml --all

run_case "sync --base-only willow" \
    "$ROOT_DIR/scripts/sync.sh" --base-only willow

run_case "status --format text bramble" \
    "$ROOT_DIR/scripts/status.sh" bramble

run_case "status --format json bramble" \
    "$ROOT_DIR/scripts/status.sh" --format json bramble

run_case "status --json bramble" \
    "$ROOT_DIR/scripts/status.sh" --json bramble

run_case "teardown --force fern" \
    "$ROOT_DIR/scripts/teardown.sh" --force fern

echo "legacy wrapper tests passed"
