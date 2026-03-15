defmodule Conductor.SelfUpdate do
  @moduledoc """
  Hot-reload the conductor after merging changes to itself.

  When the orchestrator merges a PR that changed files in `conductor/`,
  this module pulls the latest code and recompiles. The BEAM hot-swaps
  module code automatically — GenServers pick up new code on their next
  message. No restart, no state loss.

  This is core OTP: Erlang was designed for telecom switches that upgrade
  without downtime. We're just using it for what it was built for.
  """

  require Logger

  @repo_root Path.expand("../../..", __DIR__)

  @doc """
  Check if a merged PR changed conductor code, and if so, pull + recompile.

  Called by the orchestrator after each successful label-driven merge.
  """
  @spec maybe_reload(binary(), pos_integer()) :: :ok | :noop
  def maybe_reload(repo, pr_number) do
    if self_repo?(repo) do
      case changed_conductor_files?(pr_number, repo) do
        true ->
          Logger.info("[self-update] PR ##{pr_number} changed conductor code, hot-reloading")
          pull_and_recompile()

        false ->
          :noop
      end
    else
      :noop
    end
  end

  @doc """
  Check if origin/master has diverged from HEAD. If so, pull and recompile.

  Called on every poll tick so externally merged changes (human force-merge,
  other conductor instances) are picked up without waiting for a conductor-initiated merge.
  """
  @spec check_for_updates() :: :ok | :noop
  def check_for_updates do
    case Conductor.Shell.cmd("git", ["-C", @repo_root, "fetch", "origin", "master", "--quiet"],
           timeout: 30_000
         ) do
      {:ok, _} ->
        if local_behind_remote?() do
          Logger.info("[self-update] HEAD behind origin/master, pulling")
          pull_and_recompile()
        else
          :noop
        end

      {:error, msg, _} ->
        Logger.debug("[self-update] fetch failed: #{msg}")
        :noop
    end
  end

  # --- Private ---

  defp local_behind_remote? do
    # Counts commits on origin/master not reachable from HEAD.
    # Returns "0\n" when at or ahead, ">0\n" only when truly behind.
    case Conductor.Shell.cmd(
           "git",
           ["-C", @repo_root, "rev-list", "--count", "HEAD..origin/master"],
           timeout: 10_000
         ) do
      {:ok, output} ->
        String.trim(output) != "0"

      _ ->
        false
    end
  end

  defp self_repo?(repo) do
    # The conductor is always working on its own repo when repo matches
    # the git remote of the checkout it's running from.
    case Conductor.Shell.cmd("git", ["-C", @repo_root, "remote", "get-url", "origin"],
           timeout: 10_000
         ) do
      {:ok, url} -> String.contains?(url, repo_name(repo))
      _ -> false
    end
  end

  defp repo_name(repo), do: repo |> String.split("/") |> List.last()

  defp changed_conductor_files?(pr_number, repo) do
    case Conductor.Shell.cmd("gh", [
           "pr",
           "view",
           to_string(pr_number),
           "--repo",
           repo,
           "--json",
           "files",
           "--jq",
           ".files[].path"
         ]) do
      {:ok, output} ->
        output
        |> String.split("\n", trim: true)
        |> Enum.any?(fn f ->
          String.starts_with?(f, "conductor/") or
            String.starts_with?(f, "base/") or
            f == "CLAUDE.md" or
            f == "project.md"
        end)

      _ ->
        # Can't check — assume yes to be safe
        true
    end
  end

  defp pull_and_recompile do
    case Conductor.Shell.cmd("git", ["-C", @repo_root, "pull", "origin", "master"],
           timeout: 30_000
         ) do
      {:ok, output} ->
        Logger.info("[self-update] git pull: #{String.trim(output)}")

        try do
          Mix.Task.rerun("compile", ["--force"])
          Logger.info("[self-update] recompile complete, new code active on next message")
          :ok
        rescue
          e ->
            Logger.warning("[self-update] recompile failed: #{Exception.message(e)}")
            :ok
        end

      {:error, msg, _} ->
        Logger.warning("[self-update] git pull failed: #{msg}")
        :noop
    end
  end
end
