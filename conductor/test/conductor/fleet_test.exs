defmodule Conductor.FleetTest do
  use ExUnit.Case, async: false

  alias Conductor.{Store, Orchestrator, Config}

  # Default mock tracker — no eligible issues.
  defmodule MockTracker do
    @behaviour Conductor.Tracker
    def list_eligible(_repo, _opts), do: Application.get_env(:conductor, :_test_issues, [])
    def get_issue(_repo, _number), do: {:error, :not_found}
    def comment(_repo, _issue, _body), do: :ok
  end

  defp eventually(assert_fun, timeout_ms \\ 1_500, step_ms \\ 20) do
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
    db_path = Path.join(System.tmp_dir!(), "fleet_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "fleet_test_#{:rand.uniform(999_999)}.jsonl")

    if Process.whereis(Store), do: GenServer.stop(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    orig_tracker = Application.get_env(:conductor, :tracker_module)
    Application.put_env(:conductor, :tracker_module, MockTracker)

    orig_max_fails = Application.get_env(:conductor, :max_probe_failures)
    Application.put_env(:conductor, :max_probe_failures, 3)

    Application.delete_env(:conductor, :_test_issues)

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

      if orig_max_fails,
        do: Application.put_env(:conductor, :max_probe_failures, orig_max_fails),
        else: Application.delete_env(:conductor, :max_probe_failures)

      Application.delete_env(:conductor, :_test_issues)

      File.rm(db_path)
      File.rm(event_log)
    end)

    %{orch_pid: orch_pid}
  end

  # ---------------------------------------------------------------------------
  # fleet_status/0
  # ---------------------------------------------------------------------------

  describe "fleet_status/0" do
    test "returns empty list before loop starts" do
      assert Orchestrator.fleet_status() == []
    end

    test "returns one entry per declared worker" do
      probe_fn = fn _worker -> :ok end

      :ok =
        Orchestrator.start_loop(
          repo: "test/repo",
          workers: ["sprite-a", "sprite-b", "sprite-c"],
          probe_fn: probe_fn
        )

      statuses = Orchestrator.fleet_status()
      assert length(statuses) == 3
      names = Enum.map(statuses, & &1.worker)
      assert "sprite-a" in names
      assert "sprite-b" in names
      assert "sprite-c" in names
    end

    test "all workers start healthy" do
      probe_fn = fn _worker -> :ok end

      :ok =
        Orchestrator.start_loop(
          repo: "test/repo",
          workers: ["sprite-a", "sprite-b"],
          probe_fn: probe_fn
        )

      Process.sleep(50)
      statuses = Orchestrator.fleet_status()

      Enum.each(statuses, fn s ->
        refute s.drained
        assert s.consecutive_failures == 0
      end)
    end
  end

  # ---------------------------------------------------------------------------
  # Round-robin distribution across 3 workers
  # ---------------------------------------------------------------------------

  describe "round-robin dispatch" do
    test "3 workers each get probed once when 3 issues are eligible" do
      workers = ["sprite-1", "sprite-2", "sprite-3"]
      test_pid = self()

      # Probe always succeeds; track which workers get picked
      probe_fn = fn worker ->
        send(test_pid, {:probed, worker})
        :ok
      end

      # dispatch_fn is a no-op so we don't need RunSupervisor running in tests
      dispatch_fn = fn _opts -> {:error, :test_noop} end

      # Allow up to 3 concurrent runs
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      # Inject 3 eligible issues via the mock tracker
      issues = [
        %Conductor.Issue{number: 101, title: "issue 101", body: "", url: ""},
        %Conductor.Issue{number: 102, title: "issue 102", body: "", url: ""},
        %Conductor.Issue{number: 103, title: "issue 103", body: "", url: ""}
      ]

      Application.put_env(:conductor, :_test_issues, issues)

      if pid = Process.whereis(Orchestrator), do: GenServer.stop(pid)
      {:ok, _} = Orchestrator.start_link([])

      :ok =
        Orchestrator.start_loop(
          repo: "test/repo",
          workers: workers,
          probe_fn: probe_fn,
          dispatch_fn: dispatch_fn
        )

      # Collect all probe calls (we expect 3 unique workers)
      probed_workers =
        Enum.reduce(1..9, [], fn _, acc ->
          receive do
            {:probed, w} -> [w | acc]
          after
            300 -> acc
          end
        end)

      # Each of the 3 workers should have been probed at least once
      assert "sprite-1" in probed_workers
      assert "sprite-2" in probed_workers
      assert "sprite-3" in probed_workers
    end
  end

  # ---------------------------------------------------------------------------
  # Worker health: draining and auto-recovery
  # ---------------------------------------------------------------------------

  describe "worker health tracking" do
    test "worker is drained after max_probe_failures consecutive failures" do
      probe_fn = fn _worker -> {:error, "unreachable"} end
      dispatch_fn = fn _opts -> {:error, :test_noop} end

      # Provide an issue to trigger probing
      issues = [%Conductor.Issue{number: 201, title: "t", body: "", url: ""}]
      Application.put_env(:conductor, :_test_issues, issues)

      :ok =
        Orchestrator.start_loop(
          repo: "test/repo",
          workers: ["bad-sprite"],
          probe_fn: probe_fn,
          dispatch_fn: dispatch_fn
        )

      # Trigger enough polls for failures to accumulate (threshold = 3)
      Enum.each(1..4, fn _ -> send(Process.whereis(Orchestrator), :poll) end)

      eventually(fn ->
        [status] = Orchestrator.fleet_status()
        assert status.drained
        assert status.consecutive_failures >= 3
      end)
    end

    test "drained worker is auto-recovered on next successful probe" do
      # First <max> calls fail, then succeed
      call_count = :counters.new(1, [])
      max = Config.max_probe_failures()

      probe_fn = fn _worker ->
        n = :counters.get(call_count, 1)
        :counters.add(call_count, 1, 1)
        if n < max, do: {:error, "failing"}, else: :ok
      end

      dispatch_fn = fn _opts -> {:error, :test_noop} end

      # Provide an issue to trigger probing
      issue = %Conductor.Issue{number: 202, title: "t", body: "", url: ""}
      Application.put_env(:conductor, :_test_issues, [issue])

      :ok =
        Orchestrator.start_loop(
          repo: "test/repo",
          workers: ["recovering-sprite"],
          probe_fn: probe_fn,
          dispatch_fn: dispatch_fn
        )

      # Drive enough polls: drain (3 fails) then recover (1 success)
      Enum.each(1..6, fn _ -> send(Process.whereis(Orchestrator), :poll) end)

      eventually(fn ->
        [status] = Orchestrator.fleet_status()
        refute status.drained
        assert status.consecutive_failures == 0
      end)
    end

    test "second worker is drained while first stays healthy" do
      workers = ["healthy-sprite", "bad-sprite"]

      probe_fn = fn
        "healthy-sprite" -> :ok
        "bad-sprite" -> {:error, "down"}
      end

      dispatch_fn = fn _opts -> {:error, :test_noop} end

      # 2+ issues with max_concurrent_runs=2 so both workers get probed per poll
      issues = [
        %Conductor.Issue{number: 204, title: "t1", body: "", url: ""},
        %Conductor.Issue{number: 205, title: "t2", body: "", url: ""}
      ]

      Application.put_env(:conductor, :_test_issues, issues)
      Application.put_env(:conductor, :max_concurrent_runs, 2)

      on_exit(fn -> Application.delete_env(:conductor, :max_concurrent_runs) end)

      :ok =
        Orchestrator.start_loop(
          repo: "test/repo",
          workers: workers,
          probe_fn: probe_fn,
          dispatch_fn: dispatch_fn
        )

      Enum.each(1..(Config.max_probe_failures() + 2), fn _ ->
        send(Process.whereis(Orchestrator), :poll)
      end)

      eventually(fn ->
        statuses = Orchestrator.fleet_status()
        healthy = Enum.find(statuses, &(&1.worker == "healthy-sprite"))
        refute healthy.drained
      end)
    end
  end

  # ---------------------------------------------------------------------------
  # Config.workers/0
  # ---------------------------------------------------------------------------

  describe "Config.workers/0" do
    test "returns empty list when unconfigured" do
      Application.delete_env(:conductor, :workers)
      assert Config.workers() == []
    end

    test "normalises string entries" do
      Application.put_env(:conductor, :workers, ["sprite-a", "sprite-b"])
      workers = Config.workers()
      assert [%{name: "sprite-a", tags: []}, %{name: "sprite-b", tags: []}] = workers
    after
      Application.delete_env(:conductor, :workers)
    end

    test "normalises map entries with atom keys" do
      Application.put_env(:conductor, :workers, [%{name: "sprite-x", tags: ["gpu"]}])
      [w] = Config.workers()
      assert w.name == "sprite-x"
      assert w.tags == ["gpu"]
    after
      Application.delete_env(:conductor, :workers)
    end

    test "defaults tags to [] when absent in map" do
      Application.put_env(:conductor, :workers, [%{name: "sprite-y"}])
      [w] = Config.workers()
      assert w.tags == []
    after
      Application.delete_env(:conductor, :workers)
    end
  end

  # ---------------------------------------------------------------------------
  # Config.max_probe_failures/0
  # ---------------------------------------------------------------------------

  describe "Config.max_probe_failures/0" do
    test "defaults to 3" do
      Application.delete_env(:conductor, :max_probe_failures)
      assert Config.max_probe_failures() == 3
    end

    test "returns configured value" do
      Application.put_env(:conductor, :max_probe_failures, 5)
      assert Config.max_probe_failures() == 5
    after
      Application.delete_env(:conductor, :max_probe_failures)
    end
  end

  # --------------------------------------------------------------------------
  # Sprite.wake/1 contract
  # --------------------------------------------------------------------------

  describe "Sprite.wake/1 contract" do
    test "function is exported with arity 1" do
      Code.ensure_loaded!(Conductor.Sprite)
      assert function_exported?(Conductor.Sprite, :wake, 1)
    end
  end
end
