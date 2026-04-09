#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FUNCTION_NAME="${1:?Usage: scripts/ci/dagger-call.sh <function> [extra dagger args...]}"
shift || true

SNAPSHOT_DIR=""
DAGGER_CONFIG_HOME=""
DOCKER_SHIM_DIR=""

require_command() {
  local command_name="${1:?command required}"

  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "error: required command missing from PATH: ${command_name}" >&2
    exit 127
  fi
}

require_trusted_runtime() {
  if [[ "${CI:-}" == "true" || "${CI:-}" == "1" ]]; then
    if [[ "${BB_ALLOW_PRIVILEGED_DAGGER_IN_CI:-}" != "1" ]]; then
      cat >&2 <<'EOF'
error: scripts/ci/dagger-call.sh uses Dagger's privileged engine path.
Refusing to run automatically in CI unless BB_ALLOW_PRIVILEGED_DAGGER_IN_CI=1
is set for a trusted runner.
EOF
      exit 1
    fi
  fi
}

ensure_docker_connectivity() {
  local current_context=""
  local fallback_context="${BB_DOCKER_CONTEXT_FALLBACK:-}"

  if docker version >/dev/null 2>&1; then
    return 0
  fi

  if [[ -n "${DOCKER_HOST:-}" || -n "${DOCKER_CONTEXT:-}" ]]; then
    echo "error: configured Docker endpoint is unreachable" >&2
    return 1
  fi

  current_context="$(docker context show 2>/dev/null || true)"

  if [[ -z "${fallback_context}" ]]; then
    if [[ -n "${current_context}" ]]; then
      echo "error: Docker context ${current_context} is unreachable; set BB_DOCKER_CONTEXT_FALLBACK=<context> to opt into a fallback" >&2
    else
      echo "error: Docker is unreachable; set BB_DOCKER_CONTEXT_FALLBACK=<context> to opt into a fallback" >&2
    fi

    return 1
  fi

  if ! docker context ls --format '{{.Name}}' | grep -Fx -- "${fallback_context}" >/dev/null; then
    echo "error: requested Docker fallback context not found: ${fallback_context}" >&2
    return 1
  fi

  if DOCKER_CONTEXT="${fallback_context}" docker version >/dev/null 2>&1; then
    export DOCKER_CONTEXT="${fallback_context}"
    echo "warning: Docker context ${current_context:-<unknown>} is unreachable; using explicit fallback ${fallback_context}" >&2
    return 0
  fi

  echo "error: Docker fallback context is unreachable: ${fallback_context}" >&2
  return 1
}

cleanup() {
  if [[ -n "${SNAPSHOT_DIR}" ]]; then
    rm -rf "${SNAPSHOT_DIR}"
  fi

  if [[ -n "${DAGGER_CONFIG_HOME}" ]]; then
    rm -rf "${DAGGER_CONFIG_HOME}"
  fi

  if [[ -n "${DOCKER_SHIM_DIR}" ]]; then
    rm -rf "${DOCKER_SHIM_DIR}"
  fi
}

trap cleanup EXIT

create_snapshot() {
  local snapshot_dir
  snapshot_dir="$(mktemp -d "${TMPDIR:-/tmp}/bbci.XXXXXX")"

  (
    cd "${ROOT_DIR}"
    git ls-files --cached --others --exclude-standard -z | \
      while IFS= read -r -d '' path; do
        [[ -e "${path}" ]] || continue
        printf '%s\0' "${path}"
      done | \
      rsync -a --from0 --files-from=- ./ "${snapshot_dir}/"
  )

  mkdir -p "${snapshot_dir}/dagger"
  : > "${snapshot_dir}/dagger/.env"

  printf '%s\n' "${snapshot_dir}"
}

create_dagger_config_home() {
  local config_dir
  local engine_config="${ROOT_DIR}/dagger/engine.json"

  config_dir="$(mktemp -d "${TMPDIR:-/tmp}/bbci-dagger-config.XXXXXX")"
  mkdir -p "${config_dir}/dagger"
  cp "${engine_config}" "${config_dir}/dagger/engine.json"
  printf '%s\n' "${config_dir}"
}

resolve_docker_cli() {
  if command -v docker >/dev/null 2>&1; then
    return 0
  fi

  if command -v colima >/dev/null 2>&1 && colima status >/dev/null 2>&1; then
    DOCKER_SHIM_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bbci-docker.XXXXXX")"

    cat > "${DOCKER_SHIM_DIR}/docker" <<'EOF'
#!/usr/bin/env bash
exec colima ssh -- docker "$@"
EOF
    chmod +x "${DOCKER_SHIM_DIR}/docker"
    export PATH="${DOCKER_SHIM_DIR}:${PATH}"
    echo "warning: docker CLI missing; using Colima docker shim" >&2
    return 0
  fi

  echo "error: required command missing from PATH: docker" >&2
  exit 127
}

require_command git
require_command rsync
require_command dagger
require_trusted_runtime
resolve_docker_cli
ensure_docker_connectivity

SNAPSHOT_DIR="$(create_snapshot)"
DAGGER_CONFIG_HOME="$(create_dagger_config_home)"

(
  cd "${SNAPSHOT_DIR}"
  export XDG_CONFIG_HOME="${DAGGER_CONFIG_HOME}"
  dagger -m "${SNAPSHOT_DIR}" call "${FUNCTION_NAME}" "$@"
)
