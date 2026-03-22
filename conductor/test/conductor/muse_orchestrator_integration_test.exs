defmodule Conductor.MuseOrchestratorIntegrationTest do
  use ExUnit.Case, async: false
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{Orchestrator, Store}

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
      for {{__MODULE__, _} = key, _} <- :persistent_term.get() do
        :persistent_term.erase(key)
      end
    end
  end

  defmodule MockTracker do
    @behaviour Conductor.Tracker
    alias Conductor.MuseOrchestratorIntegrationTest.MockState

    def list_eligible(_repo, _opts), do: []
    def get_issue(_repo, _number), do: {:error, :not_found}
    def issue_has_label?(_repo, _issue_number, _label), do: {:ok, false}
    def issue_comments(_repo, _issue_number), do: {:ok, []}

    def comment(repo, issue_number, body) do
      comments = MockState.get({:comments, repo, issue_number}, [])
      MockState.put({:comments, repo, issue_number}, comments ++ [body])
      :ok
    end
  end

  defmodule MockCodeHost do
    @behaviour Conductor.CodeHost
    alias Conductor.MuseOrchestratorIntegrationTest.MockState

    def checks_green?(_repo, _pr_number), do: true
    def checks_failed?(_repo, _pr_number), do: false
    def open_prs(_repo), do: {:ok, []}
    def pr_review_comments(_repo, _pr_number), do: {:ok, []}
    def pr_ci_failure_logs(_repo, _pr_number), do: {:ok, ""}
    def add_label(_repo, _pr_number, _label), do: :ok
    def close_pr(_repo, _pr_number, _opts \\ []), do: :ok
    def find_open_pr(_repo, _issue_number, _expected_branch \\ nil), do: {:error, :not_found}
    def issue_open_prs(_repo, _issue_number), do: {:ok, []}
    def pr_state(_repo, _pr_number), do: {:ok, "OPEN"}
    def get_pr_checks(_repo, _pr_number), do: {:ok, []}
    def ci_status(_repo, _pr_number), do: {:ok, %{state: :green, summary: "green", pending: []}}

    def labeled_prs(repo, label), do: {:ok, MockState.get({:labeled_prs, repo, label}, [])}

    def merge(_repo, pr_number, _opts) do
      merges = MockState.get(:merge_calls, [])
      MockState.put(:merge_calls, merges ++ [pr_number])
      :ok
    end

    def close_issue(repo, issue_number) do
      closed = MockState.get(:closed_issues, [])
      MockState.put(:closed_issues, closed ++ [{repo, issue_number}])
      :ok
    end
  end

  defmodule MockWorker do
    def probe(_worker, _opts \\ []), do: {:ok, %{reachable: true}}
    def busy?(_worker, _opts \\ []), do: false
  end

  defmodule MockRunLauncher do
    def start(_opts) do
      {:ok, spawn(fn -> Process.sleep(5_000) end)}
    end
  end

  defmodule MockRunControl do
    def operator_block(pid, _reason) do
      Process.exit(pid, :shutdown)
      :ok
    end
  end

  defmodule MockMuse do
    alias Conductor.MuseOrchestratorIntegrationTest.MockState

    def observe(run_id) do
      calls = MockState.get(:observe_calls, [])
      MockState.put(:observe_calls, calls ++ [run_id])
      :ok
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "muse_orch_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "muse_orch_#{System.unique_integer([:positive])}.jsonl")

    stop_conductor_app()
    stop_process(Orchestrator)
    stop_process(Store)
    stop_process(Conductor.TaskSupervisor)
    stop_process(Conductor.RunSupervisor)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)
    {:ok, _} = DynamicSupervisor.start_link(strategy: :one_for_one, name: Conductor.RunSupervisor)
    {:ok, _} = Orchestrator.start_link()

    originals = %{
      tracker_module: Application.get_env(:conductor, :tracker_module),
      code_host_module: Application.get_env(:conductor, :code_host_module),
      worker_module: Application.get_env(:conductor, :worker_module),
      run_launcher_module: Application.get_env(:conductor, :run_launcher_module),
      run_control_module: Application.get_env(:conductor, :run_control_module),
      muse_module: Application.get_env(:conductor, :muse_module)
    }

    Application.put_env(:conductor, :tracker_module, MockTracker)
    Application.put_env(:conductor, :code_host_module, MockCodeHost)
    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :run_launcher_module, MockRunLauncher)
    Application.put_env(:conductor, :run_control_module, MockRunControl)
    Application.put_env(:conductor, :muse_module, MockMuse)

    on_exit(fn ->
      stop_process(Orchestrator)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Conductor.RunSupervisor)
      stop_process(Store)
      MockState.cleanup()

      Enum.each(originals, fn {key, value} -> restore_env(key, value) end)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "merged runs trigger muse observation and ci_timeout runs do not" do
    {:ok, merged_run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 780,
        issue_title: "muse merge",
        builder_sprite: "bb-weaver"
      })

    Store.update_run(merged_run_id, %{pr_number: 780, phase: "pr_opened", status: "pr_opened"})
    Store.acquire_lease("test/repo", 780, merged_run_id)

    MockState.put(
      {:labeled_prs, "test/repo", "lgtm"},
      [%{"number" => 780, "headRefName" => "factory/780-123"}]
    )

    :ok = Orchestrator.configure_polling(repo: "test/repo", workers: ["bb-weaver"])

    eventually(fn ->
      assert MockState.get(:observe_calls, []) == [merged_run_id]
      {:ok, run} = Store.get_run(merged_run_id)
      assert run["phase"] == "merged"
    end)

    {:ok, timeout_run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 781,
        issue_title: "muse timeout",
        builder_sprite: "bb-weaver"
      })

    Store.update_run(timeout_run_id, %{
      pr_number: 781,
      phase: "ci_timeout",
      status: "ci_timeout",
      completed_at: DateTime.utc_now() |> DateTime.to_iso8601()
    })

    assert MockState.get(:observe_calls, []) == [merged_run_id]
  end

  defp eventually(assert_fun, timeout_ms \\ 1_500, step_ms \\ 25) do
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
end
