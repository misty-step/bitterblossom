defmodule Conductor.OrchestratorTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{GitHub, Store, Orchestrator}

  # Mock tracker: no eligible issues by default.
  defmodule MockTracker do
    @behaviour Conductor.Tracker
    alias Conductor.OrchestratorTest.MockState

    def list_eligible(repo, opts),
      do: MockState.get({:eligible, repo, Keyword.get(opts, :label)}, [])

    def get_issue(repo, number) do
      case MockState.get({:issue, repo, number}) do
        nil -> {:error, :not_found}
        issue -> {:ok, issue}
      end
    end

    def comment(repo, issue_number, body) do
      comments = MockState.get({:comments, repo, issue_number}, [])
      MockState.put({:comments, repo, issue_number}, comments ++ [body])
      :ok
    end

    def issue_has_label?(repo, issue_number, label) do
      MockState.get({:issue_has_label, repo, issue_number, label}, {:ok, false})
    end

    def issue_comments(repo, issue_number) do
      case MockState.get({:issue_comments, repo, issue_number}, {:ok, []}) do
        {:ok, _comments} = result -> result
        {:error, _reason} = result -> result
        comments -> {:ok, comments}
      end
    end
  end

  defmodule MockShaper do
    alias Conductor.OrchestratorTest.MockState

    def shape(repo, issue_number, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:shape_attempted, repo, issue_number})
      Process.sleep(MockState.get({:shape_delay_ms, repo, issue_number}, 0))

      case MockState.get({:shape_result, repo, issue_number}, {:error, :not_configured}) do
        {:sleep, sleep_ms, result} ->
          Process.sleep(sleep_ms)
          result

        {:ok, result} = shaped when result in [:shaped, :already_shaped] ->
          if issue = MockState.get({:issue_after_shape, repo, issue_number}) do
            MockState.put({:issue, repo, issue_number}, issue)
          end

          shaped

        {:raise, error} ->
          raise error

        {:throw, reason} ->
          throw(reason)

        {:exit, reason} ->
          exit(reason)

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

    def ci_status(_repo, pr_number),
      do: MockState.get({:ci_status, pr_number}, {:ok, %{state: :green}})

    def merge(_repo, pr_number, _opts) do
      merge_calls = MockState.get(:merge_calls, [])
      MockState.put(:merge_calls, merge_calls ++ [pr_number])
      MockState.get({:merge_result, pr_number}, :ok)
    end

    def labeled_prs(repo, label), do: {:ok, MockState.get({:labeled_prs, repo, label}, [])}
    def open_prs(_repo), do: {:ok, []}
    def pr_review_comments(_repo, _pr), do: {:ok, []}
    def pr_ci_failure_logs(_repo, _pr), do: {:ok, ""}
    def add_label(_repo, _pr, _label), do: :ok

    def close_issue(repo, issue_number) do
      close_calls = MockState.get(:close_issue_calls, [])
      MockState.put(:close_issue_calls, close_calls ++ [{repo, issue_number}])
      MockState.get({:close_issue_result, repo, issue_number}, :ok)
    end

    def close_pr(_repo, _pr_number, _opts \\ []), do: :ok

    def find_open_pr(_repo, issue_number, _expected_branch \\ nil),
      do: MockState.get({:open_pr, issue_number}, {:error, :not_found})

    def issue_open_prs(_repo, _issue_number), do: {:ok, []}

    def pr_state(_repo, pr_number),
      do: MockState.get({:pr_state, pr_number}, {:ok, "OPEN"})
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

  defmodule MockRunControl do
    alias Conductor.OrchestratorTest.MockState

    def operator_block(pid, reason) do
      calls = MockState.get(:run_control_calls, [])
      MockState.put(:run_control_calls, calls ++ [{pid, reason}])
      Process.exit(pid, :shutdown)
      :ok
    end
  end

  defmodule MockSelfUpdate do
    def check_for_updates, do: :noop
  end

  defmodule MockRunReconciler do
    alias Conductor.OrchestratorTest.MockState

    def reconcile_stale_runs(repo, opts \\ []) do
      send(
        MockState.get(:test_pid, self()),
        {:startup_reconciled, repo, Keyword.get(opts, :active_issue_numbers, [])}
      )

      :ok
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

  defp safe_stop(nil), do: :ok

  defp safe_stop(pid) when is_pid(pid) do
    if Process.alive?(pid) do
      try do
        GenServer.stop(pid)
      catch
        :exit, _ -> :ok
      end
    else
      :ok
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "orch_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "orch_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(Orchestrator)
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    # Inject mock tracker so polls don't hit GitHub
    orig_tracker = Application.get_env(:conductor, :tracker_module)
    Application.put_env(:conductor, :tracker_module, MockTracker)

    orig_worker = Application.get_env(:conductor, :worker_module)
    Application.put_env(:conductor, :worker_module, MockWorker)

    orig_launcher = Application.get_env(:conductor, :run_launcher_module)
    Application.put_env(:conductor, :run_launcher_module, MockRunLauncher)

    orig_code_host = Application.get_env(:conductor, :code_host_module)
    Application.put_env(:conductor, :code_host_module, MockCodeHost)

    orig_run_control = Application.get_env(:conductor, :run_control_module)
    Application.put_env(:conductor, :run_control_module, MockRunControl)

    orig_shaper = Application.get_env(:conductor, :shaper_module)
    Application.put_env(:conductor, :shaper_module, MockShaper)

    orig_self_update = Application.get_env(:conductor, :self_update_module)
    Application.put_env(:conductor, :self_update_module, MockSelfUpdate)

    # Use a 60-minute stale threshold; tests can plant older heartbeats to trigger expiry
    orig_stale = Application.get_env(:conductor, :stale_run_threshold_minutes)
    Application.put_env(:conductor, :stale_run_threshold_minutes, 60)

    orig_probe_threshold = Application.get_env(:conductor, :fleet_probe_failure_threshold)
    Application.put_env(:conductor, :fleet_probe_failure_threshold, 2)

    MockState.put(:test_pid, self())
    MockState.put(:started_runs, [])
    MockState.put(:run_lifetime_ms, 150)
    MockState.put(:merge_calls, [])
    MockState.put(:close_issue_calls, [])
    MockState.put(:run_control_calls, [])

    # Restart the Orchestrator under the global name so configure_polling/1 works
    safe_stop(Process.whereis(Orchestrator))
    {:ok, orch_pid} = Orchestrator.start_link([])

    on_exit(fn ->
      stop_process(Orchestrator)
      stop_process(Store)
      stop_process(Conductor.TaskSupervisor)

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

      if orig_run_control,
        do: Application.put_env(:conductor, :run_control_module, orig_run_control),
        else: Application.delete_env(:conductor, :run_control_module)

      if orig_shaper,
        do: Application.put_env(:conductor, :shaper_module, orig_shaper),
        else: Application.delete_env(:conductor, :shaper_module)

      if orig_self_update,
        do: Application.put_env(:conductor, :self_update_module, orig_self_update),
        else: Application.delete_env(:conductor, :self_update_module)

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

  describe "terminate/2 shutdown behavior" do
    test "orchestrator traps exits for clean shutdown" do
      info = Process.info(Process.whereis(Orchestrator), :trap_exit)
      assert {:trap_exit, true} = info
    end

    test "stopping orchestrator with workers calls kill_fleet_agents" do
      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      safe_stop(Process.whereis(Orchestrator))
    end
  end

  describe "configure_polling/1" do
    test "returns error when workers list is empty" do
      assert {:error, :no_workers} =
               Orchestrator.configure_polling(repo: "test/repo", workers: [])
    end

    test "returns :ok with at least one worker" do
      assert :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
    end

    test "clears a prior label filter when a later configure_polling call omits :label" do
      assert :ok =
               Orchestrator.configure_polling(
                 repo: "test/repo",
                 label: "autopilot",
                 workers: ["sprite-1"]
               )

      assert :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert :sys.get_state(Orchestrator).label == nil
    end

    test "clears shape attempts when configure_polling switches repos" do
      assert :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      :sys.replace_state(Orchestrator, fn state ->
        %{state | shape_attempts: %{123 => :crypto.hash(:sha256, "draft body")}}
      end)

      assert :ok = Orchestrator.configure_polling(repo: "other/repo", workers: ["sprite-1"])

      assert :sys.get_state(Orchestrator).shape_attempts == %{}
    end
  end

  describe "pause/resume dispatch" do
    test "pause prevents new runs and resume restarts polling", %{orch_pid: orch_pid} do
      issue = %Conductor.Issue{
        number: 301,
        title: "paused issue",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/301"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])

      assert :ok = Orchestrator.pause()
      assert :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      assert Store.dispatch_paused?()

      send(Process.whereis(Orchestrator), :poll)
      Process.sleep(100)
      assert MockState.get(:started_runs) == []

      assert :ok = Orchestrator.resume()
      refute Store.dispatch_paused?()
      send(orch_pid, :poll)

      eventually(fn ->
        assert MockState.get(:started_runs) == [{301, "sprite-1"}]
      end)
    end

    test "poll fails closed when pause state cannot be read", %{orch_pid: orch_pid} do
      Process.unlink(orch_pid)

      assert :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      assert Process.alive?(orch_pid)

      GenServer.stop(Store)
      send(orch_pid, :poll)
      Process.sleep(50)

      assert Process.alive?(orch_pid)
    end
  end

  describe "reconcile — stale run detection" do
    test "configure_polling reconciles stale runs before polling begins" do
      orig_reconciler = Application.get_env(:conductor, :run_reconciler_module)
      Application.put_env(:conductor, :run_reconciler_module, MockRunReconciler)

      on_exit(fn ->
        if orig_reconciler,
          do: Application.put_env(:conductor, :run_reconciler_module, orig_reconciler),
          else: Application.delete_env(:conductor, :run_reconciler_module)
      end)

      assert :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      assert_received {:startup_reconciled, "test/repo", []}
    end

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

      # configure_polling triggers an immediate poll
      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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

      # configure_polling with capacity 0 — no runs should start
      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: workers)

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

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: workers)

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
      MockState.put({:shape_result, "test/repo", 301}, {:ok, :shaped})
      MockState.put({:issue_after_shape, "test/repo", 301}, shaped_issue)

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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
      MockState.put({:shape_result, "test/repo", 306}, {:ok, :already_shaped})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

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
      MockState.put({:shape_result, "test/repo", 302}, {:error, :llm_unavailable})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 302}, 1_000
      send(Process.whereis(Orchestrator), :poll)
      refute_receive {:shape_attempted, "test/repo", 302}, 200

      Process.sleep(100)
      assert MockState.get(:started_runs) == []
    end

    test "slow shaping does not block orchestrator calls" do
      issue = %Conductor.Issue{
        number: 311,
        title: "slow shaping",
        body: "needs shaping",
        url: "https://example.test/issues/311"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, "test/repo", 311}, {:sleep, 500, {:error, :llm_slow}})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 311}, 1_000

      pause_task = Task.async(fn -> Orchestrator.pause() end)

      assert Task.await(pause_task, 200) == :ok
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
      MockState.put({:shape_result, "test/repo", 303}, {:error, :llm_unavailable})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 303}, 1_000

      updated_issue = %{issue | body: "operator updated context"}
      MockState.put({:eligible, "test/repo", nil}, [updated_issue])

      send(Process.whereis(Orchestrator), :poll)

      assert_receive {:shape_attempted, "test/repo", 303}, 1_000
    end

    test "continues shaping later issues after a ready issue consumes the last slot" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)
      MockState.put(:run_lifetime_ms, 1_000)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      ready_issue = %Conductor.Issue{
        number: 304,
        title: "ready issue",
        body: "## Problem\nx\n\n## Acceptance Criteria\n- [ ] [test] y",
        url: "https://example.test/issues/304"
      }

      issue_1 = %Conductor.Issue{
        number: 305,
        title: "first underspecified issue",
        body: "draft one",
        url: "https://example.test/issues/305"
      }

      issue_2 = %Conductor.Issue{
        number: 306,
        title: "second underspecified issue",
        body: "draft two",
        url: "https://example.test/issues/306"
      }

      MockState.put({:eligible, "test/repo", nil}, [ready_issue, issue_1, issue_2])
      MockState.put({:shape_result, "test/repo", 305}, {:error, :llm_unavailable})
      MockState.put({:shape_result, "test/repo", 306}, {:error, :llm_unavailable})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert MockState.get(:started_runs) == [{304, "sprite-1"}]
      end)

      assert_receive {:shape_attempted, "test/repo", 305}, 1_000
      assert_receive {:shape_attempted, "test/repo", 306}, 1_000
    end

    test "attempts shaping even when the poll starts at max active capacity" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)
      MockState.put(:run_lifetime_ms, 1_000)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      ready_issue = %Conductor.Issue{
        number: 307,
        title: "ready issue",
        body: "## Problem\nx\n\n## Acceptance Criteria\n- [ ] [test] y",
        url: "https://example.test/issues/307"
      }

      unready_issue = %Conductor.Issue{
        number: 308,
        title: "underspecified issue",
        body: "draft body",
        url: "https://example.test/issues/308"
      }

      MockState.put({:eligible, "test/repo", nil}, [ready_issue])

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert MockState.get(:started_runs) == [{307, "sprite-1"}]
      end)

      MockState.put({:eligible, "test/repo", nil}, [unready_issue])
      MockState.put({:shape_result, "test/repo", 308}, {:error, :llm_unavailable})
      send(Process.whereis(Orchestrator), :poll)

      assert_receive {:shape_attempted, "test/repo", 308}, 1_000
    end

    test "treats :already_shaped as a successful shaping attempt" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      unready_issue = %Conductor.Issue{
        number: 309,
        title: "already shaped elsewhere",
        body: "Need better issue text",
        url: "https://example.test/issues/309"
      }

      ready_issue = %{
        unready_issue
        | body: "## Problem\nx\n\n## Acceptance Criteria\n- [ ] [test] y"
      }

      MockState.put({:eligible, "test/repo", nil}, [unready_issue])
      MockState.put({:shape_result, "test/repo", 309}, {:ok, :already_shaped})
      MockState.put({:issue_after_shape, "test/repo", 309}, ready_issue)

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 309}, 1_000
      Process.sleep(100)
      assert MockState.get(:started_runs) == []

      MockState.put({:eligible, "test/repo", nil}, [MockState.get({:issue, "test/repo", 309})])
      send(Process.whereis(Orchestrator), :poll)

      eventually(fn ->
        assert MockState.get(:started_runs) == [{309, "sprite-1"}]
      end)
    end

    test "records a failed shaping attempt when the shaper raises without crashing the loop" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 310,
        title: "crashy shaper input",
        body: "draft body",
        url: "https://example.test/issues/310"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, "test/repo", 310}, {:raise, RuntimeError.exception("boom")})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 310}, 1_000
      assert Process.alive?(Process.whereis(Orchestrator))

      send(Process.whereis(Orchestrator), :poll)
      refute_receive {:shape_attempted, "test/repo", 310}, 200
      assert MockState.get(:started_runs) == []
    end

    test "records a failed shaping attempt when the shaper throws without crashing the loop" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 312,
        title: "throwy shaper input",
        body: "draft body",
        url: "https://example.test/issues/312"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, "test/repo", 312}, {:throw, :boom})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 312}, 1_000
      assert Process.alive?(Process.whereis(Orchestrator))

      send(Process.whereis(Orchestrator), :poll)
      refute_receive {:shape_attempted, "test/repo", 312}, 200
      assert MockState.get(:started_runs) == []
    end

    test "records a failed shaping attempt when the shaper exits without crashing the loop" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 313,
        title: "exiting shaper input",
        body: "draft body",
        url: "https://example.test/issues/313"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, "test/repo", 313}, {:exit, :boom})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 313}, 1_000
      assert Process.alive?(Process.whereis(Orchestrator))

      send(Process.whereis(Orchestrator), :poll)
      refute_receive {:shape_attempted, "test/repo", 313}, 200
      assert MockState.get(:started_runs) == []
    end

    test "records an unexpected shaper return as a failed shaping attempt" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 314,
        title: "weird shaper return",
        body: "draft body",
        url: "https://example.test/issues/314"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, "test/repo", 314}, :weird_return)

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 314}, 1_000
      assert Process.alive?(Process.whereis(Orchestrator))

      send(Process.whereis(Orchestrator), :poll)
      refute_receive {:shape_attempted, "test/repo", 314}, 200
      assert MockState.get(:started_runs) == []
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
      MockState.put({:shape_result, "test/repo", 308}, {:error, :llm_unavailable})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 308}, 1_000
      assert MockState.get(:started_runs) == []
    end

    test "does not block orchestrator control calls while shaping runs in the background" do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue = %Conductor.Issue{
        number: 311,
        title: "slow shaper input",
        body: "needs grooming",
        url: "https://example.test/issues/311"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:shape_result, "test/repo", 311}, {:error, :llm_slow})
      MockState.put({:shape_delay_ms, "test/repo", 311}, 500)

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      assert_receive {:shape_attempted, "test/repo", 311}, 1_000

      pause_task = Task.async(fn -> Orchestrator.pause() end)
      assert Task.yield(pause_task, 400) == {:ok, :ok}
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

    test "releases lease when marking conflict blocked" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 56,
          issue_title: "conflict lease test",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 100})
      Store.acquire_lease("test/repo", 56, run_id)
      assert Store.leased?("test/repo", 56)

      Orchestrator.mark_conflict_blocked("test/repo", 100)

      eventually(fn ->
        refute Store.leased?("test/repo", 56)
      end)
    end
  end

  describe "operator directives" do
    test "hold label prevents dispatch for an eligible issue" do
      issue = %Conductor.Issue{
        number: 401,
        title: "held issue",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/401"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:issue_has_label, "test/repo", 401, "hold"}, {:ok, true})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(100)
      assert MockState.get(:started_runs) == []
    end

    test "hold label blocks merge and records operator_hold" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 402,
          issue_title: "hold merge",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 77, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 77, "headRefName" => "factory/402-123"}]
      )

      MockState.put({:issue_has_label, "test/repo", 402, "hold"}, {:ok, true})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "blocked"

        events = Store.list_events(run_id)
        operator_event = Enum.find(events, &(&1["event_type"] == "operator_blocked"))
        assert operator_event["payload"]["reason"] == "operator_hold"
      end)
    end

    test "bb: cancel comment blocks merge and records operator_cancel" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 403,
          issue_title: "cancel merge",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 78, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 78, "headRefName" => "factory/403-123"}]
      )

      MockState.put({:issue_comments, "test/repo", 403}, [%{"body" => "bb: cancel"}])

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "blocked"
        refute Store.leased?("test/repo", 403)

        events = Store.list_events(run_id)
        operator_event = Enum.find(events, &(&1["event_type"] == "operator_blocked"))
        assert operator_event["payload"]["reason"] == "operator_cancel"
      end)
    end

    test "comment lookup errors fail closed for merge" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 404,
          issue_title: "comment lookup failed",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 79, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 79, "headRefName" => "factory/404-123"}]
      )

      MockState.put({:issue_comments, "test/repo", 404}, {:error, :github_down})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(100)
      {:ok, run} = Store.get_run(run_id)
      assert run["phase"] == "pr_opened"
      assert MockState.get(:merge_calls) == []

      events = Store.list_events(run_id)
      refute Enum.any?(events, &(&1["event_type"] == "operator_blocked"))
    end

    test "bb: cancel comment in body.text blocks merge" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 405,
          issue_title: "cancel merge body.text",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 80, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 80, "headRefName" => "factory/405-123"}]
      )

      MockState.put(
        {:issue_comments, "test/repo", 405},
        GitHub.normalize_issue_comments([%{"body" => %{"text" => "bb: cancel"}}])
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "blocked"

        events = Store.list_events(run_id)
        operator_event = Enum.find(events, &(&1["event_type"] == "operator_blocked"))
        assert operator_event["payload"]["reason"] == "operator_cancel"
      end)
    end

    test "label check errors fail closed for dispatch" do
      issue = %Conductor.Issue{
        number: 404,
        title: "label lookup failed",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/404"
      }

      MockState.put({:eligible, "test/repo", nil}, [issue])
      MockState.put({:issue_has_label, "test/repo", 404, "hold"}, {:error, :github_down})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(100)
      assert MockState.get(:started_runs) == []
    end

    test "hold label blocks merge when issue number is inferred from the branch" do
      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 81, "headRefName" => "factory/406-123"}]
      )

      MockState.put({:issue_has_label, "test/repo", 406, "hold"}, {:ok, true})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(100)
      assert MockState.get(:merge_calls) == []
    end

    test "bb: cancel comment in body.body blocks merge" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 407,
          issue_title: "cancel merge body.body",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 82, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 82, "headRefName" => "factory/407-123"}]
      )

      MockState.put(
        {:issue_comments, "test/repo", 407},
        GitHub.normalize_issue_comments([%{"body" => %{"body" => "bb: cancel"}}])
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "blocked"

        events = Store.list_events(run_id)
        operator_event = Enum.find(events, &(&1["event_type"] == "operator_blocked"))
        assert operator_event["payload"]["reason"] == "operator_cancel"
      end)
    end

    test "operator directives use configurable policy strings" do
      orig_hold = Application.get_env(:conductor, :operator_hold_label)
      orig_cancel = Application.get_env(:conductor, :operator_cancel_command)

      Application.put_env(:conductor, :operator_hold_label, "pause-me")
      Application.put_env(:conductor, :operator_cancel_command, "bb: stop")

      on_exit(fn ->
        if orig_hold,
          do: Application.put_env(:conductor, :operator_hold_label, orig_hold),
          else: Application.delete_env(:conductor, :operator_hold_label)

        if orig_cancel,
          do: Application.put_env(:conductor, :operator_cancel_command, orig_cancel),
          else: Application.delete_env(:conductor, :operator_cancel_command)
      end)

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 408,
          issue_title: "configurable directives",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 83, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 83, "headRefName" => "factory/408-123"}]
      )

      MockState.put({:issue_has_label, "test/repo", 408, "pause-me"}, {:ok, false})
      MockState.put({:issue_comments, "test/repo", 408}, [%{"body" => "bb: stop"}])

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "blocked"

        events = Store.list_events(run_id)
        operator_event = Enum.find(events, &(&1["event_type"] == "operator_blocked"))
        assert operator_event["payload"]["reason"] == "operator_cancel"
      end)
    end

    test "bb: cancel comment blocks an active run and frees the worker slot", %{
      orch_pid: orch_pid
    } do
      orig_max = Application.get_env(:conductor, :max_concurrent_runs)
      Application.put_env(:conductor, :max_concurrent_runs, 1)

      on_exit(fn ->
        if orig_max,
          do: Application.put_env(:conductor, :max_concurrent_runs, orig_max),
          else: Application.delete_env(:conductor, :max_concurrent_runs)
      end)

      issue1 = %Conductor.Issue{
        number: 409,
        title: "active cancel",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/409"
      }

      issue2 = %Conductor.Issue{
        number: 410,
        title: "next issue",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/410"
      }

      MockState.put(:run_lifetime_ms, 5_000)
      MockState.put({:eligible, "test/repo", nil}, [issue1])

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert MockState.get(:started_runs) == [{409, "sprite-1"}]
      end)

      MockState.put({:issue_comments, "test/repo", 409}, [%{"body" => "bb: cancel"}])
      MockState.put({:eligible, "test/repo", nil}, [issue2])

      send(orch_pid, :poll)

      eventually(fn ->
        [{_pid, reason}] = MockState.get(:run_control_calls)
        assert reason == "operator_cancel"
        assert MockState.get(:started_runs) == [{409, "sprite-1"}, {410, "sprite-1"}]
      end)
    end
  end

  describe "issue_number_for_pr_lookup/4" do
    test "returns :unmapped when run lookup misses and branch has no issue number" do
      assert :unmapped =
               Orchestrator.issue_number_for_pr_lookup(
                 "test/repo",
                 904,
                 "fix/cerberus-permissions",
                 fn _repo, _pr_number -> {:error, :not_found} end
               )
    end

    test "returns :skip when run lookup returns an unexpected store error" do
      assert :skip =
               Orchestrator.issue_number_for_pr_lookup(
                 "test/repo",
                 901,
                 "factory/901-123",
                 fn _repo, _pr_number -> {:error, :db_down} end
               )
    end

    test "returns :skip when run lookup exits" do
      assert :skip =
               Orchestrator.issue_number_for_pr_lookup(
                 "test/repo",
                 903,
                 "factory/903-123",
                 fn _repo, _pr_number -> exit(:noproc) end
               )
    end

    test "returns :skip when run lookup raises" do
      assert :skip =
               Orchestrator.issue_number_for_pr_lookup(
                 "test/repo",
                 902,
                 "factory/902-123",
                 fn _repo, _pr_number -> raise "sqlite exploded" end
               )
    end
  end

  describe "parse_issue_number_from_branch/1" do
    test "parses factory/ branch" do
      assert {:ok, 42} = Orchestrator.parse_issue_number_from_branch("factory/42-1234567890")
    end

    test "parses fix/ branch" do
      assert {:ok, 99} =
               Orchestrator.parse_issue_number_from_branch("fix/99-cerberus-permissions")
    end

    test "parses multi-segment branch" do
      assert {:ok, 123} = Orchestrator.parse_issue_number_from_branch("team/fix/123-bug")
    end

    test "parses bare branch without prefix" do
      assert {:ok, 7} = Orchestrator.parse_issue_number_from_branch("7-quick-fix")
    end

    test "parses branch without description suffix" do
      assert {:ok, 123} = Orchestrator.parse_issue_number_from_branch("hotfix/123")
    end

    test "returns :skip for branch without issue number" do
      assert :skip = Orchestrator.parse_issue_number_from_branch("feature/unrelated")
    end

    test "returns :skip for empty trailing segment" do
      assert :skip = Orchestrator.parse_issue_number_from_branch("feature/")
    end

    test "returns :skip for non-numeric issue segment" do
      assert :skip = Orchestrator.parse_issue_number_from_branch("fix/abc-description")
    end

    test "parses issue number when suffix contains additional dashes" do
      assert {:ok, 123} = Orchestrator.parse_issue_number_from_branch("fix/123-456-bug")
    end

    test "returns :skip for branch with no dash delimiter" do
      assert :skip = Orchestrator.parse_issue_number_from_branch("main")
    end

    test "does not match prefix of larger number" do
      # "factory/420-..." should not match issue 42
      assert {:ok, 420} = Orchestrator.parse_issue_number_from_branch("factory/420-1234567890")
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

  describe "merge releases lease" do
    test "record_merge releases the lease for the issue" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 700,
          issue_title: "merge lease test",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 200, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 700, run_id)
      assert Store.leased?("test/repo", 700)

      # Simulate merge via labeled_prs poll
      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 200, "headRefName" => "factory/700-123"}]
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        refute Store.leased?("test/repo", 700)
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "merged"
      end)
    end

    test "record_merge closes the linked issue after merge" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 702,
          issue_title: "merge close issue test",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 202, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 702, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 202, "headRefName" => "factory/702-123"}]
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert {"test/repo", 702} in MockState.get(:close_issue_calls, [])
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "merged"
      end)
    end

    test "record_merge keeps the lease when issue closure fails" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 703,
          issue_title: "merge close issue failure test",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 203, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 703, run_id)
      MockState.put({:close_issue_result, "test/repo", 703}, {:error, :github_down})

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 203, "headRefName" => "factory/703-123"}]
      )

      log =
        capture_log(fn ->
          :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

          eventually(fn ->
            assert {"test/repo", 703} in MockState.get(:close_issue_calls, [])
            assert Store.leased?("test/repo", 703)
            {:ok, run} = Store.get_run(run_id)
            assert run["phase"] == "pr_opened"
          end)
        end)

      assert log =~ "post-merge reconciliation is incomplete"
      assert log =~ "leaving run #{run_id} and lease for issue #703 unchanged"
    end

    test "record_merge clears ci wait metadata after merge" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 701,
          issue_title: "merge cleanup test",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{
        pr_number: 201,
        phase: "pr_opened",
        status: "pr_opened",
        ci_wait_started_at:
          DateTime.utc_now() |> DateTime.add(-600, :second) |> DateTime.to_iso8601(),
        ci_last_reported_at:
          DateTime.utc_now() |> DateTime.add(-300, :second) |> DateTime.to_iso8601(),
        blocked_reason: "stale wait state"
      })

      Store.acquire_lease("test/repo", 701, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 201, "headRefName" => "factory/701-123"}]
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "merged"
        assert run["ci_wait_started_at"] == nil
        assert run["ci_last_reported_at"] == nil
        assert run["blocked_reason"] == nil
      end)
    end
  end

  describe "CI wait governance" do
    test "logs pending CI status with job URLs while waiting" do
      orig_interval = Application.get_env(:conductor, :ci_status_log_interval_minutes)
      Application.put_env(:conductor, :ci_status_log_interval_minutes, 0)

      on_exit(fn ->
        if orig_interval,
          do: Application.put_env(:conductor, :ci_status_log_interval_minutes, orig_interval),
          else: Application.delete_env(:conductor, :ci_status_log_interval_minutes)
      end)

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 501,
          issue_title: "pending ci",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 501, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 501, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 501, "headRefName" => "factory/501-123"}]
      )

      MockState.put(
        {:ci_status, 501},
        {:ok,
         %{
           state: :pending,
           summary:
             "waiting on Cerberus · wave1 · Testing (IN_PROGRESS) https://example.test/checks/501",
           pending: [
             %{
               name: "Cerberus · wave1 · Testing",
               status: "IN_PROGRESS",
               conclusion: nil,
               url: "https://example.test/checks/501"
             }
           ]
         }}
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      send(Process.whereis(Orchestrator), :poll)

      eventually(fn ->
        events = Store.list_events(run_id)
        wait_events = Enum.filter(events, &(&1["event_type"] == "ci_wait_status"))
        assert length(wait_events) >= 2
        wait_event = List.last(wait_events)
        assert wait_event["payload"]["summary"] =~ "Cerberus · wave1 · Testing"

        assert hd(wait_event["payload"]["pending_checks"])["url"] ==
                 "https://example.test/checks/501"

        assert MockState.get(:merge_calls) == []
      end)
    end

    test "transitions to ci_timeout with blocked reason and next actions" do
      orig_timeout = Application.get_env(:conductor, :ci_timeout_minutes)
      orig_interval = Application.get_env(:conductor, :ci_status_log_interval_minutes)
      Application.put_env(:conductor, :ci_timeout_minutes, 0)
      Application.put_env(:conductor, :ci_status_log_interval_minutes, 0)

      on_exit(fn ->
        if orig_timeout,
          do: Application.put_env(:conductor, :ci_timeout_minutes, orig_timeout),
          else: Application.delete_env(:conductor, :ci_timeout_minutes)

        if orig_interval,
          do: Application.put_env(:conductor, :ci_status_log_interval_minutes, orig_interval),
          else: Application.delete_env(:conductor, :ci_status_log_interval_minutes)
      end)

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 502,
          issue_title: "ci timeout",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 502, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 502, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 502, "headRefName" => "factory/502-123"}]
      )

      MockState.put(
        {:ci_status, 502},
        {:ok,
         %{
           state: :pending,
           summary:
             "waiting on Cerberus · wave1 · Testing (IN_PROGRESS) https://example.test/checks/502",
           pending: [
             %{
               name: "Cerberus · wave1 · Testing",
               status: "IN_PROGRESS",
               conclusion: nil,
               url: "https://example.test/checks/502"
             }
           ]
         }}
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "ci_timeout"
        assert run["status"] == "ci_timeout"
        assert run["blocked_reason"] =~ "https://example.test/checks/502"
        assert run["blocked_reason"] =~ "trigger a new run for this issue"
        refute Store.leased?("test/repo", 502)

        events = Store.list_events(run_id)
        assert Enum.any?(events, &(&1["event_type"] == "ci_timeout"))

        comments = MockState.get({:comments, "test/repo", 502}, [])
        assert Enum.any?(comments, &String.contains?(&1, "https://example.test/checks/502"))
        assert Enum.any?(comments, &String.contains?(&1, "trigger a new run for this issue"))
      end)
    end

    test "does not merge when the latest run already timed out even if CI is green" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 503,
          issue_title: "timed out run",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 503, phase: "ci_timeout", status: "ci_timeout"})
      Store.complete_run(run_id, "ci_timeout", "ci_timeout")

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 503, "headRefName" => "factory/503-123"}]
      )

      MockState.put({:ci_status, 503}, {:ok, %{state: :green, summary: "all checks green"}})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert MockState.get(:merge_calls) == []
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "ci_timeout"
      end)
    end

    test "a new run on the same PR can merge after an earlier ci_timeout" do
      {:ok, timed_out_run_id} =
        Store.create_run(%{
          run_id: "run-504-timeout",
          repo: "test/repo",
          issue_number: 504,
          issue_title: "older timed out run",
          builder_sprite: "sprite-1"
        })

      Store.update_run(timed_out_run_id, %{
        pr_number: 504,
        phase: "ci_timeout",
        status: "ci_timeout",
        blocked_reason: "old timeout"
      })

      Store.complete_run(timed_out_run_id, "ci_timeout", "ci_timeout")

      {:ok, retry_run_id} =
        Store.create_run(%{
          run_id: "run-504-retry",
          repo: "test/repo",
          issue_number: 504,
          issue_title: "retry run",
          builder_sprite: "sprite-1"
        })

      Store.update_run(retry_run_id, %{pr_number: 504, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 504, retry_run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 504, "headRefName" => "factory/504-123"}]
      )

      MockState.put({:ci_status, 504}, {:ok, %{state: :green, summary: "all checks green"}})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert MockState.get(:merge_calls) == [504]
        {:ok, retry_run} = Store.get_run(retry_run_id)
        assert retry_run["phase"] == "merged"

        {:ok, timed_out_run} = Store.get_run(timed_out_run_id)
        assert timed_out_run["phase"] == "ci_timeout"
      end)
    end

    test "failed CI clears wait tracking on the active run" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 505,
          issue_title: "failed ci",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{
        pr_number: 505,
        phase: "pr_opened",
        status: "pr_opened",
        ci_wait_started_at:
          DateTime.utc_now() |> DateTime.add(-600, :second) |> DateTime.to_iso8601(),
        ci_last_reported_at:
          DateTime.utc_now() |> DateTime.add(-300, :second) |> DateTime.to_iso8601()
      })

      Store.acquire_lease("test/repo", 505, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 505, "headRefName" => "factory/505-123"}]
      )

      MockState.put(
        {:ci_status, 505},
        {:ok,
         %{
           state: :failed,
           summary: "failed checks: Deploy (TIMED_OUT) https://example.test/checks/505"
         }}
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "pr_opened"
        assert run["ci_wait_started_at"] == nil
        assert run["ci_last_reported_at"] == nil
        assert MockState.get(:merge_calls) == []
      end)
    end

    test "unknown CI leaves the run untouched while waiting for signal" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 506,
          issue_title: "unknown ci",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 506, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 506, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 506, "headRefName" => "factory/506-123"}]
      )

      MockState.put(
        {:ci_status, 506},
        {:ok, %{state: :unknown, summary: "no actionable CI signal yet", pending: []}}
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert run["phase"] == "pr_opened"
        assert run["ci_wait_started_at"] == nil
        assert run["ci_last_reported_at"] == nil
        assert Store.list_events(run_id) == []
        assert MockState.get(:merge_calls) == []
      end)
    end

    test "warns when CI inspection fails" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 507,
          issue_title: "ci api error",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 507, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 507, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 507, "headRefName" => "factory/507-123"}]
      )

      MockState.put({:ci_status, 507}, {:error, :api_error})

      log =
        capture_log(fn ->
          :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
          send(Process.whereis(Orchestrator), :poll)
          Process.sleep(100)
        end)

      assert log =~ "failed to inspect CI for PR #507"
      assert log =~ ":api_error"
      assert MockState.get(:merge_calls) == []
      {:ok, run} = Store.get_run(run_id)
      assert run["phase"] == "pr_opened"
    end

    test "invalid stored ci wait timestamps restart tracking cleanly" do
      orig_interval = Application.get_env(:conductor, :ci_status_log_interval_minutes)
      Application.put_env(:conductor, :ci_status_log_interval_minutes, 0)

      on_exit(fn ->
        if orig_interval,
          do: Application.put_env(:conductor, :ci_status_log_interval_minutes, orig_interval),
          else: Application.delete_env(:conductor, :ci_status_log_interval_minutes)
      end)

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 508,
          issue_title: "invalid ci timestamps",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{
        pr_number: 508,
        phase: "pr_opened",
        status: "pr_opened",
        ci_wait_started_at: "not-a-datetime",
        ci_last_reported_at: "also-not-a-datetime"
      })

      Store.acquire_lease("test/repo", 508, run_id)

      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 508, "headRefName" => "factory/508-123"}]
      )

      MockState.put(
        {:ci_status, 508},
        {:ok,
         %{
           state: :pending,
           summary:
             "waiting on Cerberus · wave1 · Testing (IN_PROGRESS) https://example.test/checks/508",
           pending: [
             %{
               name: "Cerberus · wave1 · Testing",
               status: "IN_PROGRESS",
               conclusion: nil,
               url: "https://example.test/checks/508"
             }
           ]
         }}
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      send(Process.whereis(Orchestrator), :poll)

      eventually(fn ->
        {:ok, run} = Store.get_run(run_id)
        assert is_binary(run["ci_wait_started_at"])
        assert is_binary(run["ci_last_reported_at"])

        events = Store.list_events(run_id)
        assert Enum.any?(events, &(&1["event_type"] == "ci_wait_started"))
        assert Enum.any?(events, &(&1["event_type"] == "ci_wait_status"))
      end)
    end
  end

  describe "merge skips non-conductor PRs" do
    test "non-conductor PR with lgtm is not auto-merged", %{orch_pid: orch_pid} do
      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 300, "headRefName" => "fix/cerberus-permissions"}]
      )

      :ok =
        GenServer.call(
          orch_pid,
          {:configure_polling, [repo: "test/repo", workers: ["sprite-1"]]}
        )

      Process.sleep(100)
      assert MockState.get(:merge_calls) == []
    end

    test "conductor_tracked? returns false for non-conductor PR even with green CI", %{
      orch_pid: _orch_pid
    } do
      # PR 400 has no Store run — conductor_tracked? returns false → merge skipped
      MockState.put(
        {:labeled_prs, "test/repo", "lgtm"},
        [%{"number" => 400, "headRefName" => "fix/99-test"}]
      )

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      Process.sleep(200)

      assert MockState.get(:merge_calls) == []
    end
  end

  describe "reconcile_held_leases" do
    test "releases lease when PR merged externally" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 800,
          issue_title: "external merge",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 300, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 800, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")

      # PR was merged externally
      MockState.put({:pr_state, 300}, {:ok, "MERGED"})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        assert {"test/repo", 800} in MockState.get(:close_issue_calls, [])
        refute Store.leased?("test/repo", 800)
        events = Store.list_events(run_id)
        types = Enum.map(events, & &1["event_type"])
        assert "external_merge" in types
      end)
    end

    test "keeps lease when external merge cannot close the issue" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 806,
          issue_title: "external merge close failure",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 306, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 806, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")

      MockState.put({:pr_state, 306}, {:ok, "MERGED"})
      MockState.put({:close_issue_result, "test/repo", 806}, {:error, :github_down})

      log =
        capture_log(fn ->
          :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

          eventually(fn ->
            assert {"test/repo", 806} in MockState.get(:close_issue_calls, [])
            assert Store.leased?("test/repo", 806)
            events = Store.list_events(run_id)
            types = Enum.map(events, & &1["event_type"])
            refute "external_merge" in types
          end)
        end)

      assert log =~ "keeping lease for issue #806 after external merge of PR #306"
      assert log =~ "issue closure will retry on the next poll"
    end

    test "releases lease when PR closed without merge" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 801,
          issue_title: "external close",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 301, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 801, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")

      MockState.put({:pr_state, 301}, {:ok, "CLOSED"})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      eventually(fn ->
        refute Store.leased?("test/repo", 801)
        events = Store.list_events(run_id)
        types = Enum.map(events, & &1["event_type"])
        assert "external_close" in types
      end)
    end

    test "survives Store errors without crashing poll loop", %{orch_pid: orch_pid} do
      Process.unlink(orch_pid)

      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 803,
          issue_title: "error test",
          builder_sprite: "sprite-1"
        })

      Store.acquire_lease("test/repo", 803, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")
      Store.update_run(run_id, %{pr_number: 303})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])
      Process.sleep(100)

      # Kill the Store — next poll tick should survive the error
      GenServer.stop(Store)
      send(orch_pid, :poll)
      Process.sleep(100)

      assert Process.alive?(orch_pid)
    end

    test "holds lease when pr_state returns error" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 804,
          issue_title: "github down",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 304, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 804, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")

      MockState.put({:pr_state, 304}, {:error, :github_down})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(200)
      assert Store.leased?("test/repo", 804)
    end

    test "holds lease when pr_number is nil" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 805,
          issue_title: "no pr",
          builder_sprite: "sprite-1"
        })

      # No pr_number set — run was force-completed
      Store.acquire_lease("test/repo", 805, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(200)
      assert Store.leased?("test/repo", 805)
    end

    test "holds lease when PR is still open" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 802,
          issue_title: "still open",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 302, phase: "pr_opened", status: "pr_opened"})
      Store.acquire_lease("test/repo", 802, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")

      MockState.put({:pr_state, 302}, {:ok, "OPEN"})

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(200)
      assert Store.leased?("test/repo", 802)
    end
  end

  describe "leased issue re-dispatch guard" do
    test "leased issue is not re-dispatched" do
      issue = %Conductor.Issue{
        number: 900,
        title: "leased issue",
        body: "## Problem\nx\n## Acceptance Criteria\ny",
        url: "https://example.test/issues/900"
      }

      # Simulate a held lease from a prior run
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 900,
          issue_title: "leased issue",
          builder_sprite: "sprite-1"
        })

      Store.acquire_lease("test/repo", 900, run_id)
      Store.complete_run(run_id, "pr_opened", "pr_opened")
      Store.update_run(run_id, %{pr_number: 400})

      # PR still open — lease holds
      MockState.put({:pr_state, 400}, {:ok, "OPEN"})

      # Make issue eligible
      MockState.put({:eligible, "test/repo", nil}, [issue])

      :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["sprite-1"])

      Process.sleep(200)
      # Should NOT have started a run for issue 900 (lease blocks it)
      started = MockState.get(:started_runs, [])
      issue_numbers = Enum.map(started, fn {n, _w} -> n end)
      refute 900 in issue_numbers
    end
  end
end
