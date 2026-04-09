#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT_DIR/scripts/lib/verdicts.sh"

usage() {
  cat <<'EOF'
usage: scripts/land.sh <branch> [-m "commit message"] [--sync-origin] [--publish] [--delete-branch]

Validate the local verdict ref, run the local Dagger verification bundle, and
squash-land the branch into the local default branch.
EOF
}

branch=""
message=""
publish_after_merge=false
delete_branch=false
sync_origin=false
fetched_origin=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    -m|--message)
      message="${2:?message required}"
      shift 2
      ;;
    --publish|--push)
      publish_after_merge=true
      sync_origin=true
      shift
      ;;
    --sync-origin)
      sync_origin=true
      shift
      ;;
    --delete-branch)
      delete_branch=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      if [[ -z "$branch" ]]; then
        branch="$1"
      else
        echo "unexpected argument: $1" >&2
        usage >&2
        exit 1
      fi
      shift
      ;;
  esac
done

if [[ -z "$branch" ]]; then
  echo "branch required" >&2
  usage >&2
  exit 1
fi

resolve_default_branch() {
  local remote_default=""

  remote_default="$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's@^refs/remotes/origin/@@')" || true

  if [[ -n "$remote_default" ]]; then
    printf '%s\n' "$remote_default"
  elif git show-ref --verify --quiet refs/heads/main; then
    printf 'main\n'
  elif git show-ref --verify --quiet refs/heads/master; then
    printf 'master\n'
  fi
}

ensure_local_branch() {
  local branch="${1:?branch required}"

  if git show-ref --verify --quiet "refs/heads/$branch"; then
    return 0
  fi

  if git show-ref --verify --quiet "refs/remotes/origin/$branch"; then
    git branch "$branch" "origin/$branch" >/dev/null 2>&1 || {
      echo "failed to create local $branch from origin/$branch" >&2
      return 1
    }
    return 0
  fi

  echo "default branch unavailable locally or at origin/$branch: $branch" >&2
  return 1
}

has_origin_remote() {
  git remote get-url origin >/dev/null 2>&1
}

maybe_fetch_origin() {
  if [[ "$sync_origin" != true ]]; then
    return 0
  fi

  if ! has_origin_remote; then
    if [[ "$publish_after_merge" == true ]]; then
      echo "--publish requires an origin remote" >&2
    else
      echo "--sync-origin requires an origin remote" >&2
    fi
    return 1
  fi

  if git fetch origin --quiet; then
    fetched_origin=true
  else
    echo "failed to fetch origin before landing" >&2
    return 1
  fi
}

fast_forward_default_branch() {
  local default_branch="${1:?default branch required}"

  if [[ "$sync_origin" != true || "$fetched_origin" != true ]]; then
    return 0
  fi

  if git show-ref --verify --quiet "refs/remotes/origin/$default_branch"; then
    git merge --ff-only "origin/$default_branch" >/dev/null
  fi
}

preflight_publish() {
  local default_branch="${1:?default branch required}"

  if [[ "$publish_after_merge" != true ]]; then
    return 0
  fi

  if ! has_origin_remote; then
    echo "--publish requires an origin remote" >&2
    return 1
  fi

  git push --dry-run origin "HEAD:refs/heads/${default_branch}" >/dev/null
}

run_verification_bundle() {
  local default_branch="${1:?default branch required}"
  local branch_sha="${2:?branch sha required}"
  local verification_dir

  verification_dir="$(mktemp -d "${TMPDIR:-/tmp}/bitterblossom-land.XXXXXX")"

  cleanup_verification_dir() {
    git worktree remove --force "$verification_dir" >/dev/null 2>&1 || true
    rm -rf "$verification_dir"
  }

  trap cleanup_verification_dir RETURN

  git worktree add --detach "$verification_dir" "$default_branch" >/dev/null

  (
    cd "$verification_dir"
    if git show-ref --verify --quiet "refs/remotes/origin/$default_branch" && [[ "$fetched_origin" == true ]]; then
      git merge --ff-only "origin/$default_branch" >/dev/null
    fi
    git merge --squash "$branch_sha" >/dev/null
    ./scripts/ci/dagger-call.sh check
  )
}

maybe_fetch_origin

default_branch="$(resolve_default_branch)"

if [[ -z "$default_branch" ]]; then
  echo "unable to determine default branch; create a local main/master first" >&2
  exit 1
fi

ensure_local_branch "$default_branch"

branch_sha="$(verdict_branch_head_sha "$branch")" || exit 1

if [[ "$branch" == "$default_branch" ]]; then
  echo "refusing to land the default branch directly" >&2
  exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "tracked changes present; commit or stash them before landing" >&2
  exit 1
fi

verdict_validate "$branch" ship

run_verification_bundle "$default_branch" "$branch_sha"

if [[ -z "$message" ]]; then
  if [[ "$(git rev-list --count "${default_branch}..${branch_sha}")" -eq 1 ]]; then
    message="$(git log -1 --format=%B "$branch_sha")"
  else
    message="$(git log --reverse --format=%s "${default_branch}..${branch_sha}" | head -1)"
  fi
fi

current_branch="$(git branch --show-current)"

git checkout "$default_branch"
fast_forward_default_branch "$default_branch"
preflight_publish "$default_branch"
git merge --squash "$branch_sha"
git commit -m "$message"

if [[ "$publish_after_merge" == true ]]; then
  BB_ALLOW_PROTECTED_PUSH=1 git push origin "$default_branch"
fi

if [[ "$delete_branch" == true ]]; then
  git branch -D "$branch"
fi

if [[ "$current_branch" != "$default_branch" && "$delete_branch" != true ]]; then
  echo "landed $branch into $default_branch"
fi
