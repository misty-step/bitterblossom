defmodule Conductor.OrchestratorTest do
  use ExUnit.Case, async: false

  alias Conductor.{Store, Orchestrator}

  # Mock tracker: no eligible issues by default.
  defmodule MockTracker do
    @behaviour Conductor.Tracker
    def list_eligible(_repo, _opts), do: []
    def get_issue(_repo, _number), do: {:error, :not_found}
    def comment(_repo, _issue, _body), do: :ok
  end

  # Retry an assertion block until it passes or timeout elapses.
  defp eventually(assert_fun, timeout_ms \\ 1_000, step_ms \\ 20) do
    deadline = System.monotonic_time(:millisecond) + timeout_ms
    do_eventually(assert_fun, deadline, step_ms)
  end

  defp do_eventually(assert_fun, deadline, step_ms) do
    assert_fun.()
  rescue
    _ ->
      if System.monotonic_time(:millisecond) < deadline do
        Process.sleep(step_ms)
        do_eventually(assert_fun, deadline, step_ms)
      else
        assert_fun.()
      end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "orch_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "orch_test_#{:rand.uniform(999_999)}.jsonl")

    if Process.whereis(Store), do: GenServer.stop(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    # Inject mock tracker so polls don't hit GitHub
    orig_tracker = Application.get_env(:conductor, :tracker_module)
    Application.put_env(:conductor, :tracker_module, MockTracker)

    # Use a 60-minute stale threshold; tests can plant older heartbeats to trigger expiry
    orig_stale = Application.get_env(:conductor, :stale_run_threshold_minutes)
    Application.put_env(:conductor, :stale_run_threshold_minutes, 60)

    # Restart the Orchestrator under the global name so start_loop/1 works
    if pid = Process.whereis(Orchestrator), do: GenServer.stop(pid)
    {:ok, orch_pid} = Orchestrator.start_link([])

    on_exit(fn ->
      case Process.whereis(Orchestrator) do
        nil -> :ok
        pid -> if Process.alive?(pid), do: catch_exit(GenServer.stop(pid))
      end

      if Process.whereis(Store), do: GenServer.stop(Store)

      if orig_tracker,
        do: Application.put_env(:conductor, :tracker_module, orig_tracker),
        else: Application.delete_env(:conductor, :tracker_module)

      if orig_stale,
        do: Application.put_env(:conductor, :stale_run_threshold_minutes, orig_stale),
        else: Application.delete_env(:conductor, :stale_run_threshold_minutes)

      File.rm(db_path)
      File.rm(event_log)
    end)

    %{orch_pid: orch_pid}
  end

  describe "start_loop/1" do
    test "returns error when workers list is empty" do
      assert {:error, :no_workers} =
               Orchestrator.start_loop(repo: "test/repo", workers: [])
    end

    test "returns :ok with at least one worker" do
      assert :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])
    end
  end

  describe "reconcile — stale run detection" do
    test "expires lease and marks failed for run with old heartbeat" do
      # Create a run with a heartbeat 2 hours old (> 60-minute threshold)
      old_heartbeat =
        DateTime.utc_now() |> DateTime.add(-7200, :second) |> DateTime.to_iso8601()

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 99,
          issue_title: "stale issue",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{heartbeat_at: old_heartbeat})
      :ok = Store.acquire_lease("test/repo", 99, run_id)

      # start_loop triggers an immediate poll
      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "failed"
        assert run["completed_at"] != nil
        refute Store.leased?("test/repo", 99)
      end)
    end

    test "stale detection records stale_run_detected event" do
      old_heartbeat =
        DateTime.utc_now() |> DateTime.add(-7200, :second) |> DateTime.to_iso8601()

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 98,
          issue_title: "stale event check",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{heartbeat_at: old_heartbeat})
      :ok = Store.acquire_lease("test/repo", 98, run_id)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        events = Store.list_events(run_id)
        event_types = Enum.map(events, & &1["event_type"])
        assert "stale_run_detected" in event_types
      end)
    end

    test "does not expire runs with recent heartbeat" do
      # Create a run with a heartbeat from 1 minute ago (well within threshold)
      recent_heartbeat =
        DateTime.utc_now() |> DateTime.add(-60, :second) |> DateTime.to_iso8601()

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 100,
          issue_title: "healthy issue",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{heartbeat_at: recent_heartbeat})
      :ok = Store.acquire_lease("test/repo", 100, run_id)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      # Give the poll a chance to fire, then assert the run is still active
      Process.sleep(100)
      {:ok, run} = Store.get_run(run_id)
      assert run["completed_at"] == nil
      assert Store.leased?("test/repo", 100)
    end

    test "treats nil heartbeat as stale" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 97,
          issue_title: "nil heartbeat",
          builder_sprite: "sprite-1"
        })

      # Force heartbeat_at to nil
      Store.update_run(run_id, %{heartbeat_at: nil})
      :ok = Store.acquire_lease("test/repo", 97, run_id)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "failed"
        refute Store.leased?("test/repo", 97)
      end)
    end
  end

  describe "concurrent run management" do
    test "does not start more runs than max_concurrent_runs allows" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 0)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      # start_loop with capacity 0 — no runs should start
      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      # Give the poll a chance to fire, then assert nothing was dispatched
      Process.sleep(100)
      runs = Store.list_runs()
      assert runs == []
    end
  end

  describe "Store.list_active_runs/1" do
    test "returns only non-terminal runs for the given repo" do
      {:ok, run_a} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 10,
          issue_title: "a",
          builder_sprite: "s"
        })

      {:ok, run_b} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 11,
          issue_title: "b",
          builder_sprite: "s"
        })

      # Complete run_b
      Store.complete_run(run_b, "merged", "merged")

      active = Store.list_active_runs("test/repo")
      ids = Enum.map(active, & &1["run_id"])
      assert run_a in ids
      refute run_b in ids
    end

    test "ignores runs from a different repo" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "other/repo",
          issue_number: 20,
          issue_title: "other",
          builder_sprite: "s"
        })

      active = Store.list_active_runs("test/repo")
      ids = Enum.map(active, & &1["run_id"])
      refute run_id in ids
    end
  end
end
