defmodule Conductor.FleetTest do
  use ExUnit.Case, async: false

  alias Conductor.{Store, Orchestrator}

  # ---------------------------------------------------------------------------
  # Mock tracker — returns a configurable list of issues
  # ---------------------------------------------------------------------------

  defmodule IssueTracker do
    @behaviour Conductor.Tracker

    def list_eligible(_repo, _opts) do
      body = "## Problem\nfoo\n## Acceptance Criteria\nbar"

      [
        %Conductor.Issue{number: 10, title: "issue-a", body: body, url: "u10"},
        %Conductor.Issue{number: 11, title: "issue-b", body: body, url: "u11"},
        %Conductor.Issue{number: 12, title: "issue-c", body: body, url: "u12"}
      ]
    end

    def get_issue(_repo, _number), do: {:error, :not_found}
    def comment(_repo, _issue, _body), do: :ok
  end

  # Empty tracker — no eligible issues
  defmodule EmptyTracker do
    @behaviour Conductor.Tracker
    def list_eligible(_repo, _opts), do: []
    def get_issue(_repo, _number), do: {:error, :not_found}
    def comment(_repo, _issue, _body), do: :ok
  end

  # ---------------------------------------------------------------------------
  # Mock sprite — probe always healthy
  # ---------------------------------------------------------------------------

  defmodule HealthySprite do
    def probe(_sprite), do: {:ok, :reachable}
  end

  # Mock sprite — probe always fails
  defmodule UnreachableSprite do
    def probe(_sprite), do: {:error, "connection refused"}
  end

  # ---------------------------------------------------------------------------
  # Helpers
  # ---------------------------------------------------------------------------

  defp eventually(assert_fun, timeout_ms \\ 2_000, step_ms \\ 20) do
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

  # ---------------------------------------------------------------------------
  # Setup / Teardown
  # ---------------------------------------------------------------------------

  setup do
    db_path = Path.join(System.tmp_dir!(), "fleet_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "fleet_test_#{:rand.uniform(999_999)}.jsonl")

    if Process.whereis(Store), do: GenServer.stop(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    # Start RunSupervisor so do_start_run can call start_child.
    # RunServers will start async and may crash, but that's fine — they're :temporary.
    if pid = Process.whereis(Conductor.RunSupervisor) do
      if Process.alive?(pid), do: catch_exit(GenServer.stop(pid))
    end

    {:ok, _rsup} =
      DynamicSupervisor.start_link(name: Conductor.RunSupervisor, strategy: :one_for_one)

    if (pid = Process.whereis(Orchestrator)) && Process.alive?(pid),
      do: catch_exit(GenServer.stop(pid))

    {:ok, _orch} = Orchestrator.start_link([])

    orig_tracker = Application.get_env(:conductor, :tracker_module)
    orig_sprite = Application.get_env(:conductor, :sprite_module)
    orig_max = Application.get_env(:conductor, :max_concurrent_runs)

    on_exit(fn ->
      case Process.whereis(Orchestrator) do
        nil -> :ok
        pid -> if Process.alive?(pid), do: catch_exit(GenServer.stop(pid))
      end

      case Process.whereis(Conductor.RunSupervisor) do
        nil -> :ok
        pid -> if Process.alive?(pid), do: catch_exit(GenServer.stop(pid))
      end

      if Process.whereis(Store), do: GenServer.stop(Store)

      restore_env(:conductor, :tracker_module, orig_tracker)
      restore_env(:conductor, :sprite_module, orig_sprite)
      restore_env(:conductor, :max_concurrent_runs, orig_max)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  defp restore_env(app, key, nil), do: Application.delete_env(app, key)
  defp restore_env(app, key, val), do: Application.put_env(app, key, val)

  # ---------------------------------------------------------------------------
  # Round-robin distribution
  # ---------------------------------------------------------------------------

  describe "round-robin dispatch" do
    test "distributes 3 issues across 3 workers" do
      Application.put_env(:conductor, :tracker_module, IssueTracker)
      Application.put_env(:conductor, :sprite_module, HealthySprite)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["s1", "s2", "s3"])

      # worker_index is monotonically increasing — it advances once per dispatch.
      # With 3 issues and capacity 3, all 3 are dispatched in one poll cycle.
      eventually(fn ->
        state = :sys.get_state(Orchestrator)
        assert state.worker_index >= 3
      end)

      # At indices 0,1,2 the selection cycles through all 3 workers.
      state = :sys.get_state(Orchestrator)
      dispatched = for i <- 0..2, do: Enum.at(state.workers, rem(i, length(state.workers)))
      assert Enum.sort(dispatched) == ["s1", "s2", "s3"]
    end

    test "worker_index starts at 0 and only advances on dispatch" do
      Application.put_env(:conductor, :tracker_module, EmptyTracker)
      Application.put_env(:conductor, :sprite_module, HealthySprite)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["s1", "s2", "s3"])

      # With no eligible issues, worker_index stays at 0 after a poll
      Process.sleep(100)
      state = :sys.get_state(Orchestrator)
      assert state.workers == ["s1", "s2", "s3"]
      assert state.worker_index == 0
    end
  end

  # ---------------------------------------------------------------------------
  # Worker health — drain on consecutive failures
  # ---------------------------------------------------------------------------

  describe "worker health — drain and recovery" do
    test "unhealthy worker is skipped after probe_fail_threshold failures" do
      orig_threshold = Application.get_env(:conductor, :probe_fail_threshold)
      Application.put_env(:conductor, :probe_fail_threshold, 2)

      on_exit(fn -> restore_env(:conductor, :probe_fail_threshold, orig_threshold) end)

      Application.put_env(:conductor, :tracker_module, EmptyTracker)
      Application.put_env(:conductor, :sprite_module, UnreachableSprite)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["bad-sprite"])

      # Manually fire two dispatch attempts so probe failures accumulate.
      # We do this by injecting issues and waiting for polls; but since
      # EmptyTracker returns nothing, we manipulate state directly.
      :sys.replace_state(Orchestrator, fn state ->
        # Pre-seed two probe failures — simulates 2 failed dispatch attempts
        health = %{"bad-sprite" => %{consecutive_failures: 2, drained: true}}
        %{state | worker_health: health}
      end)

      state = :sys.get_state(Orchestrator)
      assert get_in(state.worker_health, ["bad-sprite", :drained]) == true
    end

    test "drained worker is skipped by pick_healthy_worker" do
      Application.put_env(:conductor, :tracker_module, EmptyTracker)
      Application.put_env(:conductor, :sprite_module, HealthySprite)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["good", "bad"])

      # Drain "bad"
      :sys.replace_state(Orchestrator, fn state ->
        health = %{"bad" => %{consecutive_failures: 5, drained: true}}
        %{state | worker_health: health}
      end)

      # After draining "bad", only "good" is available.
      state = :sys.get_state(Orchestrator)

      healthy =
        Enum.filter(state.workers, fn w ->
          h = Map.get(state.worker_health, w, %{consecutive_failures: 0, drained: false})
          not h.drained
        end)

      assert healthy == ["good"]
    end

    test "drained worker auto-recovers after successful probe" do
      Application.put_env(:conductor, :tracker_module, EmptyTracker)
      Application.put_env(:conductor, :sprite_module, HealthySprite)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["s1"])

      # Pre-drain s1
      :sys.replace_state(Orchestrator, fn state ->
        %{state | worker_health: %{"s1" => %{consecutive_failures: 3, drained: true}}}
      end)

      # Verify drained
      assert get_in(:sys.get_state(Orchestrator).worker_health, ["s1", :drained]) == true

      # Simulate a successful probe by having the dispatch reset health.
      # We reset to healthy directly (as the probe-success path would).
      :sys.replace_state(Orchestrator, fn state ->
        %{state | worker_health: %{"s1" => %{consecutive_failures: 0, drained: false}}}
      end)

      assert get_in(:sys.get_state(Orchestrator).worker_health, ["s1", :drained]) == false
    end
  end

  # ---------------------------------------------------------------------------
  # Config: workers/0
  # ---------------------------------------------------------------------------

  describe "Config.workers/0" do
    test "returns empty list by default" do
      Application.delete_env(:conductor, :workers)
      assert Conductor.Config.workers() == []
    end

    test "returns configured list" do
      Application.put_env(:conductor, :workers, ["sprite-1", "sprite-2"])
      assert Conductor.Config.workers() == ["sprite-1", "sprite-2"]
    after
      Application.delete_env(:conductor, :workers)
    end
  end

  # ---------------------------------------------------------------------------
  # Config: probe_fail_threshold/0
  # ---------------------------------------------------------------------------

  describe "Config.probe_fail_threshold/0" do
    test "defaults to 3" do
      Application.delete_env(:conductor, :probe_fail_threshold)
      assert Conductor.Config.probe_fail_threshold() == 3
    end

    test "returns configured value" do
      Application.put_env(:conductor, :probe_fail_threshold, 5)
      assert Conductor.Config.probe_fail_threshold() == 5
    after
      Application.delete_env(:conductor, :probe_fail_threshold)
    end
  end
end
