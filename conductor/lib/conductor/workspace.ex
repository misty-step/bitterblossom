defmodule Conductor.Workspace do
  @moduledoc """
  Worktree lifecycle on sprites.

  Each run gets an isolated git worktree under the warm mirror.
  Preparation and cleanup are idempotent — stale state is cleaned first.
  """

  alias Conductor.Sprite

  @mirror_base "/home/sprite/workspace"
  @safe_input ~r/^[a-zA-Z0-9_\-\.\/]+$/

  @doc "Validate that a string is safe for shell interpolation. Rejects metacharacters, path traversal, absolute paths, and leading dashes."
  @spec validate_input(binary()) :: :ok | {:error, :invalid_input}
  def validate_input(input) do
    cond do
      not Regex.match?(@safe_input, input) -> {:error, :invalid_input}
      String.contains?(input, "..") -> {:error, :invalid_input}
      String.starts_with?(input, "/") -> {:error, :invalid_input}
      String.starts_with?(input, "-") -> {:error, :invalid_input}
      true -> :ok
    end
  end

  @spec prepare(binary(), binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, term()}
  def prepare(sprite, repo, run_id, branch, opts \\ []) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id),
         :ok <- validate_input(branch) do
      do_prepare(sprite, repo, run_id, branch, opts)
    end
  end

  defp do_prepare(sprite, repo, run_id, branch, opts) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    worktree = Path.join([mirror, ".bb", "conductor", run_id, "builder-worktree"])
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      git fetch --all --prune --quiet 2>/dev/null || true
      git worktree prune 2>/dev/null || true
      rm -rf #{worktree} 2>/dev/null || true
      default_branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed "s|refs/remotes/origin/||" || echo master)
      git worktree add -b #{branch} #{worktree} origin/$default_branch --quiet
      #{install_branch_guard_commands(worktree, branch)}
    '
    echo #{worktree}
    """

    case exec_fn.(sprite, commands, timeout: 120_000) do
      {:ok, output} ->
        {:ok, extract_path(output)}

      {:error, msg, code} ->
        {:error, "workspace preparation failed (#{code}): #{msg}"}
    end
  end

  @doc """
  Rebase an existing PR branch on top of the default branch and force-push.

  Uses a temporary worktree to avoid disturbing the mirror's HEAD.
  Returns `:ok` on success, `{:error, reason}` if rebase or push fails.
  """
  @spec rebase(binary(), binary(), binary()) :: :ok | {:error, term()}
  def rebase(sprite, repo, branch) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(branch) do
      do_rebase(sprite, repo, branch)
    end
  end

  defp do_rebase(sprite, repo, branch) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    safe = String.replace(branch, "/", "-")
    tmp = Path.join([mirror, ".bb", "rebase-#{safe}"])

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      git fetch origin --quiet
      git worktree prune 2>/dev/null || true
      rm -rf #{tmp} 2>/dev/null || true
      git worktree add #{tmp} #{branch} --quiet
      cd #{tmp}
      default_branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed "s|refs/remotes/origin/||" || echo master)
      git rebase origin/$default_branch
      git push --force-with-lease origin #{branch}
      cd #{mirror}
      git worktree remove --force #{tmp} 2>/dev/null || true
      git worktree prune 2>/dev/null || true
    '
    """

    case Sprite.exec(sprite, commands, timeout: 120_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @doc """
  Prepare a worktree by checking out an existing remote branch (no -b flag).
  Used when adopting a PR branch from a prior run instead of building fresh.
  """
  @spec adopt_branch(binary(), binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, term()}
  def adopt_branch(sprite, repo, run_id, branch, opts \\ []) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id),
         :ok <- validate_input(branch) do
      do_adopt_branch(sprite, repo, run_id, branch, opts)
    end
  end

  defp do_adopt_branch(sprite, repo, run_id, branch, opts) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    worktree = Path.join([mirror, ".bb", "conductor", run_id, "builder-worktree"])
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      git fetch origin --quiet
      git worktree prune 2>/dev/null || true
      rm -rf #{worktree} 2>/dev/null || true
      git worktree add #{worktree} #{branch} --quiet
      #{install_branch_guard_commands(worktree, branch)}
    '
    echo #{worktree}
    """

    case exec_fn.(sprite, commands, timeout: 120_000) do
      {:ok, output} ->
        {:ok, extract_path(output)}

      {:error, msg, code} ->
        {:error, "branch adoption failed (#{code}): #{msg}"}
    end
  end

  @spec cleanup(binary(), binary(), binary()) :: :ok | {:error, term()}
  def cleanup(sprite, repo, run_id) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id) do
      do_cleanup(sprite, repo, run_id)
    end
  end

  defp do_cleanup(sprite, repo, run_id) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    # Extract branch name from run_id pattern: run-<issue>-<ts> → factory/<issue>-<ts>
    branch = run_id_to_branch(run_id)

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      worktree_dir=".bb/conductor/#{run_id}/builder-worktree"
      if [ -d "$worktree_dir" ]; then
        git worktree remove --force "$worktree_dir" 2>/dev/null || true
      fi
      git worktree prune 2>/dev/null || true
      #{if branch, do: "git branch -D #{branch} 2>/dev/null || true", else: ""}
    '
    """

    case Sprite.exec(sprite, commands, timeout: 60_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  # run-648-1773580938 → factory/648-1773580938
  defp run_id_to_branch("run-" <> rest), do: "factory/#{rest}"
  defp run_id_to_branch(_), do: nil

  defp extract_path(output) do
    output
    |> String.split("\n", trim: true)
    |> List.last()
    |> String.trim()
  end

  defp install_branch_guard_commands(worktree, branch) do
    """
    hook_path=$(git -C #{worktree} rev-parse --git-path hooks/pre-push)
    mkdir -p "$(dirname "$hook_path")"
    cat <<'EOF' > "$hook_path"
    #!/usr/bin/env bash
    set -eu
    expected_branch='#{branch}'
    current_branch=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)

    if [ "$current_branch" != "$expected_branch" ]; then
      echo "Bitterblossom branch guard: refusing push from $current_branch; expected $expected_branch" >&2
      exit 1
    fi

    while read -r local_ref local_sha remote_ref remote_sha; do
      if [ -z "${local_ref:-}" ]; then
        continue
      fi

      case "$local_ref" in
        refs/heads/*)
          local_branch=${local_ref#refs/heads/}
          ;;
        *)
          echo "Bitterblossom branch guard: refusing non-branch push $local_ref" >&2
          exit 1
          ;;
      esac

      if [ "$local_branch" != "$expected_branch" ]; then
        echo "Bitterblossom branch guard: refusing push from $local_branch; expected $expected_branch" >&2
        exit 1
      fi

      if [ "$remote_ref" != "refs/heads/$expected_branch" ]; then
        echo "Bitterblossom branch guard: refusing push to $remote_ref; expected refs/heads/$expected_branch" >&2
        exit 1
      fi
    done
    EOF
    chmod +x "$hook_path"
    """
  end
end
