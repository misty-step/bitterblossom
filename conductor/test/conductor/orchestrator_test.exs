defmodule Conductor.OrchestratorTest do
  use ExUnit.Case, async: false

  alias Conductor.{Store, Orchestrator}

  # Mock tracker: no eligible issues by default.
  defmodule MockTracker do
    @behaviour Conductor.Tracker
    alias Conductor.OrchestratorTest.MockState

    def list_eligible(repo, opts),
      do: MockState.get({:eligible, repo, Keyword.get(opts, :label)}, [])

    def get_issue(_repo, _number), do: {:error, :not_found}
    def comment(_repo, _issue, _body), do: :ok
  end

  defmodule MockShaper do
    alias Conductor.OrchestratorTest.MockState

    def shape(repo, issue_number, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:shape_attempted, repo, issue_number})

      case MockState.get({:shape_result, issue_number}, {:error, :not_configured}) do
        {:ok, :shaped} = shaped ->
          if issue = MockState.get({:issue_after_shape, issue_number}) do
            MockState.put({:issue, repo, issue_number}, issue)
          end

          shaped

        {:raise, error} ->
          raise error

        other ->
          other
      end
    end
  end

  # Shared persistent state for mock coordination across processes.
  defmodule MockState do
    def put(key, value), do: :persistent_term.put({__MODULE__, key}, value)

    def get(key, default \\ nil) do
      try do
        :persistent_term.get({__MODULE__, key})
      rescue
        ArgumentError -> default
      end
    end

    def cleanup do
      for {{__MODULE__, _} = k, _} <- :persistent_term.get(), do: :persistent_term.erase(k)
    end
  end

  # Mock code host for orchestrator tests. Configurable merge results and PR lookups.
  defmodule MockCodeHost do
    @behaviour Conductor.CodeHost
    alias Conductor.OrchestratorTest.MockState

    def checks_green?(_repo, _pr), do: true
    def checks_failed?(_repo, _pr), do: false
    def merge(_repo, pr_number, _opts), do: MockState.get({:merge_result, pr_number}, :ok)
    def labeled_prs(_repo, _label), do: {:ok, []}
    def factory_prs(_repo), do: {:ok, []}
    def pr_review_comments(_repo, _pr), do: {:ok, []}
    def pr_ci_failure_logs(_repo, _pr), do: {:ok, ""}
    def add_label(_repo, _pr, _label), do: :ok

    def find_open_pr(_repo, issue_number),
      do: MockState.get({:open_pr, issue_number}, {:error, :not_found})
  end

  defmodule MockWorker do
    alias Conductor.OrchestratorTest.MockState

    def probe(worker, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:probed, worker})

      case MockState.get({:probe_result, worker}, {:ok, %{sprite: worker, reachable: true}}) do
        {:error, reason} -> {:error, reason}
        other -> other
      end
    end

    def busy?(worker, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:busy_checked, worker})
      MockState.get({:busy, worker}, false)
    end
  end

  defmodule MockRunLauncher do
    alias Conductor.OrchestratorTest.MockState

    def start(opts) do
      started = MockState.get(:started_runs, [])
      MockState.put(:started_runs, started ++ [{opts[:issue].number, opts[:worker]}])

      lifetime_ms = MockState.get(:run_lifetime_ms, 150)
      pid = spawn(fn -> Process.sleep(lifetime_ms) end)
      {:ok, pid}
    end
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

    orig_worker = Application.get_env(:conductor, :worker_module)
    Application.put_env(:conductor, :worker_module, MockWorker)

    orig_launcher = Application.get_env(:conductor, :run_launcher_module)
    Application.put_env(:conductor, :run_launcher_module, MockRunLauncher)

    orig_code_host = Application.get_env(:conductor, :code_host_module)
    Application.put_env(:conductor, :code_host_module, MockCodeHost)

    orig_shaper = Application.get_env(:conductor, :shaper_module)
    Application.put_env(:conductor, :shaper_module, MockShaper)

    # Use a 60-minute stale threshold; tests can plant older heartbeats to trigger expiry
    orig_stale = Application.get_env(:conductor, :stale_run_threshold_minutes)
    Application.put_env(:conductor, :stale_run_threshold_minutes, 60)

    orig_probe_threshold = Application.get_env(:conductor, :fleet_probe_failure_threshold)
    Application.put_env(:conductor, :fleet_probe_failure_threshold, 2)

    MockState.put(:test_pid, self())
    MockState.put(:started_runs, [])
    MockState.put(:run_lifetime_ms, 150)

    # Restart the Orchestrator under the global name so start_loop/1 works
    if pid = Process.whereis(Orchestrator), do: GenServer.stop(pid)
    {:ok, orch_pid} = Orchestrator.start_link([])

    on_exit(fn ->
      case Process.whereis(Orchestrator) do
        nil -> :ok
        pid -> if Process.alive?(pid), do: catch_exit(GenServer.stop(pid))
      end

      if pid = Process.whereis(Store),
        do: if(Process.alive?(pid), do: catch_exit(GenServer.stop(Store)))

      if orig_tracker,
        do: Application.put_env(:conductor, :tracker_module, orig_tracker),
        else: Application.delete_env(:conductor, :tracker_module)

      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)

      if orig_launcher,
        do: Application.put_env(:conductor, :run_launcher_module, orig_launcher),
        else: Application.delete_env(:conductor, :run_launcher_module)

      if orig_code_host,
        do: Application.put_env(:conductor, :code_host_module, orig_code_host),
        else: Application.delete_env(:conductor, :code_host_module)

      if orig_shaper,
        do: Application.put_env(:conductor, :shaper_module, orig_shaper),
        else: Application.delete_env(:conductor, :shaper_module)

      if orig_stale,
        do: Application.put_env(:conductor, :stale_run_threshold_minutes, orig_stale),
        else: Application.delete_env(:conductor, :stale_run_threshold_minutes)

      if orig_probe_threshold,
        do: Application.put_env(:conductor, :fleet_probe_failure_threshold, orig_probe_threshold),
        else: Application.delete_env(:conductor, :fleet_probe_failure_threshold)

      MockState.cleanup()

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

    test "round-robins work across three healthy workers" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 3)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issues =
        Enum.map(1..3, fn number ->
          %Conductor.Issue{
            number: number,
            title: "issue #{number}",
            body: "## Problem\nx\n## Acceptance Criteria\ny",
            url: "https://example.test/issues/#{number}"
          }
        end)

      MockState.put({:eligible, "test/repo", nil}, issues)

      workers = [
        %{name: "sprite-1", capability_tags: []},
        %{name: "sprite-2", capability_tags: ["elixir"]},
        %{name: "sprite-3", capability_tags: ["ci"]}
      ]

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: workers)

      eventually(fn ->
        assert MockState.get(:started_runs) == [{1, "sprite-1"}, {2, "sprite-2"}, {3, "sprite-3"}]
      end)
    end

    test "drains unhealthy workers after consecutive probe failures and recovers on success",
         %{orch_pid: orch_pid} do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue1 = %Conductor.Issue{
        number: 201,
        title: "first issue",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/201"
      }

      issue2 = %Conductor.Issue{
        number: 202,
        title: "second issue",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/202"
      }

      workers = [
        %{name: "sprite-1", capability_tags: []},
        %{name: "sprite-2", capability_tags: []}
      ]

      MockState.put({:probe_result, "sprite-1"}, {:error, "timeout"})
      MockState.put({:eligible, "test/repo", nil}, [issue1])

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: workers)

      eventually(fn ->
        assert MockState.get(:started_runs) == [{201, "sprite-2"}]
      end)

      Process.sleep(200)
      send(orch_pid, :poll)

      eventually(fn ->
        assert MockState.get(:started_runs) == [{201, "sprite-2"}, {201, "sprite-2"}]
      end)

      eventually(fn ->
        [drained | _] = Orchestrator.fleet_status()
        assert drained.name == "sprite-1"
        assert drained.drained == true
        assert drained.consecutive_failures == 2
      end)

      MockState.put({:probe_result, "sprite-1"}, {:ok, %{sprite: "sprite-1", reachable: true}})
      MockState.put({:eligible, "test/repo", nil}, [issue2])

      Process.sleep(200)
      send(orch_pid, :poll)

      eventually(fn ->
        assert MockState.get(:started_runs) == [
                 {201, "sprite-2"},
                 {201, "sprite-2"},
                 {202, "sprite-1"}
               ]
      end)

      eventually(fn ->
        [recovered | _] = Orchestrator.fleet_status()
        assert recovered.name == "sprite-1"
        assert recovered.drained == false
        assert recovered.consecutive_failures == 0
        assert recovered.healthy == true
      end)
    end
  end

  describe "shape-before-skip" do
    test "attempts shaping once for an unready issue and dispatches it on the next poll when shaping succeeds" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      unready_issue = %Conductor.Issue{
        number: 301,
        title: "underspecified issue",
        body: "Need better issue text",
        url: "https://example.test/issues/301"
      }

      shaped_issue = %{
        unready_issue
        | body: "## Problem\nx\n\n## Acceptance Criteria\n- [ ] [test] y"
      }

      MockState.put({:eligible, "test/repo", nil}, [unready_issue])
      MockState.put({:shape_result, 301}, {:ok, :shaped})
      MockState.put({:issue_after_shape, 301}, shaped_issue)

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 301}, 1_000
      Process.sleep(100)
      assert MockState.get(:started_runs) == []

      MockState.put({:eligible, "test/repo", nil}, [MockState.get({:issue, "test/repo", 301})])
      send(Process.whereis(Orchestrator), :poll)

      eventually(fn ->
        assert MockState.get(:started_runs) == [{301, "sprite-1"}]
      end)
    end

    test "treats :already_shaped as a successful defer-until-next-poll outcome" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 306,
        title: "already shaped elsewhere",
        body: "still stale in this poll",
        url: "https://example.test/issues/306"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, 306}, {:ok, :already_shaped})

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 306}, 1_000
      Process.sleep(100)
      assert MockState.get(:started_runs) == []
    end

    test "does not retry shaping on every poll when the issue body is unchanged" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 302,
        title: "still underspecified",
        body: "no sections here",
        url: "https://example.test/issues/302"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, 302}, {:error, :llm_unavailable})

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 302}, 1_000
      send(Process.whereis(Orchestrator), :poll)
      refute_receive {:shape_attempted, "test/repo", 302}, 200

      Process.sleep(100)
      assert MockState.get(:started_runs) == []
    end

    test "retries shaping after the issue body changes" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 303,
        title: "groom me later",
        body: "first draft",
        url: "https://example.test/issues/303"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, 303}, {:error, :llm_unavailable})

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 303}, 1_000

      updated_issue = %{issue | body: "operator updated context"}
      MockState.put({:eligible, "test/repo", nil}, [updated_issue])

      send(Process.whereis(Orchestrator), :poll)

      assert_receive {:shape_attempted, "test/repo", 303}, 1_000
    end

    test "converts shaper exceptions into skipped shaping attempts" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 307,
        title: "shaper crash",
        body: "needs shaping",
        url: "https://example.test/issues/307"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, 307}, {:raise, RuntimeError.exception("boom")})

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 307}, 1_000
      Process.sleep(100)
      assert MockState.get(:started_runs) == []
    end

    test "limits shaping work to the available slots in a single poll" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue_1 = %Conductor.Issue{
        number: 304,
        title: "first underspecified issue",
        body: "draft one",
        url: "https://example.test/issues/304"
      }

      issue_2 = %Conductor.Issue{
        number: 305,
        title: "second underspecified issue",
        body: "draft two",
        url: "https://example.test/issues/305"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue_1, issue_2])
      MockState.put({:shape_result, 304}, {:error, :llm_unavailable})
      MockState.put({:shape_result, 305}, {:error, :llm_unavailable})

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 304}, 1_000
      refute_receive {:shape_attempted, "test/repo", 305}, 200
    end

    test "still performs one shaping attempt when run slots are exhausted" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 0)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 308,
        title: "shape during saturation",
        body: "needs shaping",
        url: "https://example.test/issues/308"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, 308}, {:error, :llm_unavailable})

      :ok = Orchestrator.start_loop(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 308}, 1_000
      assert MockState.get(:started_runs) == []
    end
  end

  describe "merge_conflict?/1" do
    test "detects 'not mergeable'" do
      assert Orchestrator.merge_conflict?("Pull Request is not mergeable")
    end

    test "detects 'cannot be cleanly created'" do
      assert Orchestrator.merge_conflict?("Merge cannot be cleanly created")
    end

    test "ignores other errors" do
      refute Orchestrator.merge_conflict?("rate limit exceeded")
      refute Orchestrator.merge_conflict?("authentication failed")
      refute Orchestrator.merge_conflict?("")
    end
  end

  describe "merge conflict: mark run blocked" do
    setup do
      orig_code_host = Application.get_env(:conductor, :code_host_module)
      Application.put_env(:conductor, :code_host_module, MockCodeHost)

      on_exit(fn ->
        MockState.cleanup()

        if orig_code_host,
          do: Application.put_env(:conductor, :code_host_module, orig_code_host),
          else: Application.delete_env(:conductor, :code_host_module)
      end)

      :ok
    end

    test "marks run as blocked when rebase fails on a conflict PR" do
      # Create a run with a known PR number
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 55,
          issue_title: "conflict issue",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 99})

      # Simulate: merge fails with conflict, and worker_module.rebase also fails
      # mark_conflict_blocked is called after a failed rebase — test it directly
      Orchestrator.mark_conflict_blocked("test/repo", 99)

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "blocked"
        events = Store.list_events(run_id)
        types = Enum.map(events, & &1["event_type"])
        assert "merge_conflict_blocked" in types
      end)
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
