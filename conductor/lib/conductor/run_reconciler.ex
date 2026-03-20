defmodule Conductor.RunReconciler do
  @moduledoc """
  Reconciles durable run state that may outlive the orchestrator process.

  This is used both during startup, before polling begins, and during normal
  poll reconciliation to expire orphaned stale runs.
  """

  require Logger

  alias Conductor.{Config, Store}

  @spec reconcile_stale_runs(binary(), keyword()) :: :ok
  def reconcile_stale_runs(repo, opts \\ [])

  @spec reconcile_stale_runs(nil, keyword()) :: :ok
  def reconcile_stale_runs(nil, _opts), do: :ok

  def reconcile_stale_runs(repo, opts) when is_binary(repo) do
    active_issue_numbers =
      opts
      |> Keyword.get(:active_issue_numbers, [])
      |> MapSet.new()

    cutoff = DateTime.add(DateTime.utc_now(), -Config.stale_run_threshold_minutes() * 60, :second)

    repo
    |> Store.list_active_runs()
    |> Enum.reject(fn run -> MapSet.member?(active_issue_numbers, run["issue_number"]) end)
    |> Enum.filter(fn run -> stale_heartbeat?(run["heartbeat_at"], cutoff) end)
    |> Enum.each(fn run ->
      run_id = run["run_id"]
      issue_number = run["issue_number"]

      Logger.warning("[reconcile] stale run #{run_id} (issue ##{issue_number}), expiring lease")
      Store.expire_stale_run(repo, run_id, issue_number, run["heartbeat_at"])
    end)

    :ok
  end

  defp stale_heartbeat?(nil, _cutoff), do: true

  defp stale_heartbeat?(heartbeat_str, cutoff) do
    case DateTime.from_iso8601(heartbeat_str) do
      {:ok, heartbeat_at, _offset} -> DateTime.compare(heartbeat_at, cutoff) == :lt
      _ -> true
    end
  end
end
