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

  # --- Private ---

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
    # 1. Pull latest from master
    case Conductor.Shell.cmd("git", ["-C", @repo_root, "pull", "origin", "master"],
           timeout: 30_000
         ) do
      {:ok, output} ->
        Logger.info("[self-update] git pull: #{String.trim(output)}")

      {:error, msg, _} ->
        Logger.warning("[self-update] git pull failed: #{msg}")
        return_noop()
    end

    # 2. Recompile — BEAM hot-swaps modules automatically
    try do
      Mix.Task.rerun("compile", ["--force"])
      Logger.info("[self-update] recompile complete, new code active on next message")
      :ok
    rescue
      e ->
        Logger.warning("[self-update] recompile failed: #{Exception.message(e)}")
        :ok
    end
  end

  defp return_noop, do: :noop
end
