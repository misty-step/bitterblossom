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
  @remote_ref "origin/master"
  @warning_interval_ms 60_000

  @doc """
  Check if a merged PR changed conductor code, and if so, sync + recompile.

  Called by the orchestrator after each successful label-driven merge.
  """
  @spec maybe_reload(binary(), pos_integer()) :: :ok | :noop | {:error, :recompile_failed}
  def maybe_reload(repo, pr_number) do
    if self_repo?(repo) do
      case changed_conductor_files?(pr_number, repo) do
        true ->
          Logger.info("[self-update] PR ##{pr_number} changed conductor code, hot-reloading")
          reset_and_recompile()

        false ->
          :noop
      end
    else
      :noop
    end
  end

  @doc """
  Check if origin/master has diverged from HEAD. If so, sync and recompile.

  Called on every poll tick so externally merged changes (human force-merge,
  other conductor instances) are picked up without waiting for a conductor-initiated merge.
  """
  @spec check_for_updates() :: :ok | :noop | {:error, :recompile_failed}
  def check_for_updates do
    if active_worktrees?() do
      Logger.debug("[self-update] active worktrees present, skipping")
      :noop
    else
      case shell_module().cmd("git", ["-C", @repo_root, "fetch", "origin", "master", "--quiet"],
             timeout: 30_000
           ) do
        {:ok, _} ->
          if local_behind_remote?() do
            Logger.info("[self-update] HEAD behind #{@remote_ref}, resetting")
            reset_and_recompile()
          else
            :noop
          end

        {:error, msg, _} ->
          Logger.debug("[self-update] fetch failed: #{msg}")
          :noop
      end
    end
  end

  @spec shell_module() :: module()
  defp shell_module do
    Application.get_env(:conductor, :self_update_shell_module, Conductor.Shell)
  end

  @spec compiler_module() :: module()
  defp compiler_module do
    Application.get_env(:conductor, :self_update_compiler_module, Conductor.SelfUpdate.Compiler)
  end

  @spec clock_module() :: module()
  defp clock_module do
    Application.get_env(:conductor, :self_update_clock_module, System)
  end

  defp active_worktrees? do
    case shell_module().cmd("git", ["-C", @repo_root, "worktree", "list", "--porcelain"],
           timeout: 10_000
         ) do
      {:ok, output} ->
        output
        |> String.split("\n", trim: true)
        |> Enum.filter(&String.starts_with?(&1, "worktree "))
        |> Enum.map(&String.replace_prefix(&1, "worktree ", ""))
        |> Enum.any?(fn path -> Path.expand(path) != @repo_root end)

      {:error, msg, _} ->
        Logger.debug("[self-update] worktree inspection failed: #{msg}")
        false
    end
  end

  # --- Private ---

  defp local_behind_remote? do
    # Counts commits on origin/master not reachable from HEAD.
    # Returns "0\n" when at or ahead, ">0\n" only when truly behind.
    case shell_module().cmd(
           "git",
           ["-C", @repo_root, "rev-list", "--count", "HEAD..#{@remote_ref}"],
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
    case shell_module().cmd("git", ["-C", @repo_root, "remote", "get-url", "origin"],
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

  defp reset_and_recompile do
    if active_worktrees?() do
      Logger.debug("[self-update] active worktrees present, skipping")
      :noop
    else
      case shell_module().cmd("git", ["-C", @repo_root, "reset", "--hard", @remote_ref],
             timeout: 30_000
           ) do
        {:ok, output} ->
          Logger.info("[self-update] git reset --hard #{@remote_ref}: #{String.trim(output)}")

          try do
            case compiler_module().recompile() do
              :ok ->
                Logger.info("[self-update] recompile complete, new code active on next message")
                :ok

              {:error, reason} ->
                rate_limited_warning("[self-update] recompile failed: #{inspect(reason)}")
                {:error, :recompile_failed}
            end
          rescue
            e ->
              rate_limited_warning("[self-update] recompile failed: #{Exception.message(e)}")
              {:error, :recompile_failed}
          end

        {:error, msg, _} ->
          rate_limited_warning("[self-update] git reset failed: #{msg}")
          :noop
      end
    end
  end

  defp rate_limited_warning(message) do
    now_ms = clock_module().system_time(:millisecond)
    last_logged_ms = Process.get({__MODULE__, :last_warning_ms})

    if is_nil(last_logged_ms) or now_ms - last_logged_ms >= @warning_interval_ms do
      Logger.warning(message)
      Process.put({__MODULE__, :last_warning_ms}, now_ms)
    end
  end
end

defmodule Conductor.SelfUpdate.Compiler do
  @moduledoc false

  def recompile do
    Mix.Task.rerun("compile", ["--force"])
    :ok
  end
end
