defmodule Conductor.Workspace do
  @moduledoc """
  Worktree lifecycle on sprites.

  Each run gets an isolated git worktree under the warm mirror.
  Preparation and cleanup are idempotent — stale state is cleaned first.
  """

  alias Conductor.Sprite

  @mirror_base "/home/sprite/workspace"
  @safe_input ~r/^[a-zA-Z0-9_\-\.\/]+$/

  @doc "Validate that a string is safe for shell interpolation. Rejects metacharacters and path traversal."
  @spec validate_input(binary()) :: :ok | {:error, :invalid_input}
  def validate_input(input) do
    cond do
      not Regex.match?(@safe_input, input) -> {:error, :invalid_input}
      String.contains?(input, "..") -> {:error, :invalid_input}
      true -> :ok
    end
  end

  @spec prepare(binary(), binary(), binary(), binary()) :: {:ok, binary()} | {:error, term()}
  def prepare(sprite, repo, run_id, branch) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id),
         :ok <- validate_input(branch) do
      do_prepare(sprite, repo, run_id, branch)
    end
  end

  defp do_prepare(sprite, repo, run_id, branch) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    worktree = Path.join([mirror, ".bb", "conductor", run_id, "builder-worktree"])

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      git fetch --all --prune --quiet 2>/dev/null || true
      git worktree prune 2>/dev/null || true
      rm -rf #{worktree} 2>/dev/null || true
      default_branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed "s|refs/remotes/origin/||" || echo master)
      git worktree add -b #{branch} #{worktree} origin/$default_branch --quiet
    '
    echo #{worktree}
    """

    case Sprite.exec(sprite, commands, timeout: 120_000) do
      {:ok, output} ->
        path = output |> String.split("\n") |> List.last() |> String.trim()
        {:ok, path}

      {:error, msg, code} ->
        {:error, "workspace preparation failed (#{code}): #{msg}"}
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

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      worktree_dir=".bb/conductor/#{run_id}/builder-worktree"
      if [ -d "$worktree_dir" ]; then
        git worktree remove --force "$worktree_dir" 2>/dev/null || true
      fi
      git worktree prune 2>/dev/null || true
    '
    """

    case Sprite.exec(sprite, commands, timeout: 60_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec artifact_path(binary(), binary()) :: binary()
  def artifact_path(repo, run_id) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    Path.join([mirror, ".bb", "conductor", run_id, "builder-result.json"])
  end
end
