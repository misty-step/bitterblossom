defmodule Conductor.OrchestratorTest do
  use ExUnit.Case, async: false

  alias Conductor.{Orchestrator, Store}

  setup do
    db_path = Path.join(System.tmp_dir!(), "orch_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "orch_test_#{:rand.uniform(999_999)}.jsonl")

    if Process.whereis(Store), do: GenServer.stop(Store)
    if Process.whereis(Orchestrator), do: GenServer.stop(Orchestrator)

    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    {:ok, _} = Orchestrator.start_link()

    on_exit(fn ->
      for name <- [Orchestrator, Store] do
        if pid = Process.whereis(name) do
          try do
            GenServer.stop(pid)
          catch
            :exit, _ -> :ok
          end
        end
      end

      File.rm(db_path)
      File.rm(event_log)
    end)

    %{db_path: db_path}
  end

  describe "start_loop/1" do
    test "returns error when no workers supplied" do
      assert {:error, :no_workers} =
               Orchestrator.start_loop(repo: "test/repo", workers: [])
    end

    test "enters polling mode when workers are provided" do
      assert :ok =
               Orchestrator.start_loop(
                 repo: "test/repo",
                 workers: ["worker-1"],
                 label: "autopilot"
               )
    end
  end

  describe "reconcile — stale run expiry" do
    test "poll expires a stale run and releases its lease" do
      repo = "test/repo"
      issue_number = 99

      # Seed a run that looks abandoned: building phase, heartbeat in the past
      {:ok, run_id} =
        Store.create_run(%{
          repo: repo,
          issue_number: issue_number,
          issue_title: "stale issue",
          builder_sprite: "old-worker"
        })

      Store.update_run(run_id, %{phase: "building"})

      # The lease must be active for the run to be treated as owned
      Store.acquire_lease(repo, issue_number, run_id)

      # Back-date the heartbeat so the run appears stale
      old_ts =
        DateTime.utc_now()
        |> DateTime.add(-600, :second)
        |> DateTime.to_iso8601()

      # Directly update heartbeat via a fresh Store call (reusing the same Store GenServer)
      GenServer.call(
        Store,
        {:update_run, run_id, %{heartbeat_at: old_ts, updated_at: old_ts, picked_at: old_ts}}
      )

      # Configure a zero threshold so every run is stale immediately
      Application.put_env(:conductor, :stale_run_threshold_seconds, 0)
      on_exit(fn -> Application.delete_env(:conductor, :stale_run_threshold_seconds) end)

      :ok =
        Orchestrator.start_loop(
          repo: repo,
          workers: ["worker-1"],
          label: "autopilot"
        )

      # Trigger a poll — reconcile runs synchronously inside handle_info(:poll, ...)
      send(Orchestrator, :poll)

      # Give the GenServer a moment to process the message
      Process.sleep(100)

      {:ok, run} = Store.get_run(run_id)
      assert run["phase"] == "failed", "stale run should be marked failed"
      assert run["status"] == "failed"

      refute Store.leased?(repo, issue_number), "lease should be released after expiry"

      events = Store.list_events(run_id)
      assert Enum.any?(events, fn e -> e["event_type"] == "stale_run_expired" end)
    end

    test "poll does not expire recent runs" do
      repo = "test/repo"
      issue_number = 100

      {:ok, run_id} =
        Store.create_run(%{
          repo: repo,
          issue_number: issue_number,
          issue_title: "fresh issue",
          builder_sprite: "worker-1"
        })

      Store.update_run(run_id, %{phase: "building"})
      Store.acquire_lease(repo, issue_number, run_id)

      # Use a very large threshold so the recent run is not considered stale
      Application.put_env(:conductor, :stale_run_threshold_seconds, 9999)
      on_exit(fn -> Application.delete_env(:conductor, :stale_run_threshold_seconds) end)

      :ok =
        Orchestrator.start_loop(
          repo: repo,
          workers: ["worker-1"],
          label: "autopilot"
        )

      send(Orchestrator, :poll)
      Process.sleep(100)

      {:ok, run} = Store.get_run(run_id)
      assert run["phase"] == "building", "recent run should not be expired"
      assert Store.leased?(repo, issue_number)
    end
  end
end
