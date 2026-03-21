defmodule Conductor.FixerTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{Store, Fixer}

  # Shared state for mocks (accessible across processes)
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

  # Mock code host: configurable factory PRs and CI status
  defmodule MockCodeHost do
    @behaviour Conductor.CodeHost
    alias Conductor.FixerTest.MockState

    def checks_green?(_repo, pr_number) do
      case MockState.get(:checks_green) do
        fun when is_function(fun) -> fun.(pr_number)
        _ -> false
      end
    end

    def checks_failed?(_repo, pr_number) do
      case MockState.get(:checks_failed) do
        fun when is_function(fun) -> fun.(pr_number)
        _ -> MockState.get(:checks_failed_default, true)
      end
    end

    def ci_status(_repo, pr_number) do
      state =
        cond do
          checks_failed?("", pr_number) -> :failed
          checks_green?("", pr_number) -> :green
          true -> :pending
        end

      {:ok, %{state: state, summary: "mock ci", pending: []}}
    end

    def merge(_repo, _pr, _opts), do: :ok
    def labeled_prs(_repo, _label), do: {:ok, []}

    def open_prs(_repo), do: MockState.get(:open_prs, {:ok, []})

    def pr_ci_failure_logs(_repo, _pr_number) do
      MockState.get(:ci_failure_logs, {:ok, "Build failed: test_foo.ex:42 assertion error"})
    end

    def pr_review_comments(_repo, _pr_number), do: {:ok, []}
    def add_label(_repo, _pr_number, _label), do: :ok
    def close_issue(_repo, _issue_number), do: :ok
    def close_pr(_repo, _pr_number, _opts \\ []), do: :ok
    def find_open_pr(_repo, _issue_number, _expected_branch \\ nil), do: {:error, :not_found}
    def issue_open_prs(_repo, _issue_number), do: {:ok, []}
    def pr_state(_repo, _pr_number), do: {:ok, "OPEN"}

    def get_pr_checks(_repo, _pr_number) do
      {:ok, [%{"name" => "ci", "conclusion" => "FAILURE", "status" => "COMPLETED"}]}
    end
  end

  # Mock worker: records dispatches
  defmodule MockWorker do
    @behaviour Conductor.Worker
    alias Conductor.FixerTest.MockState

    def exec(_worker, _cmd, _opts), do: {:ok, ""}

    def dispatch(worker, prompt, _repo, _opts) do
      delay = MockState.get(:dispatch_delay_ms, 0)
      if delay > 0, do: Process.sleep(delay)
      send(MockState.get(:test_pid, self()), {:dispatched, worker, prompt})
      {:ok, "done"}
    end

    def cleanup(_worker, _repo, _run_id), do: :ok
    def busy?(_worker, _opts), do: false
  end

  defmodule MockWorkspace do
    alias Conductor.FixerTest.MockState

    def sync_persona(worker, workspace, role, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:persona_synced, worker, workspace, role})
      :ok
    end
  end

  # Mock tracker
  defmodule MockTracker do
    @behaviour Conductor.Tracker
    def list_eligible(_repo, _opts), do: []

    def get_issue(_repo, number) do
      {:ok,
       %Conductor.Issue{
         number: number,
         title: "Test issue #{number}",
         body: "Test body",
         url: "https://github.com/test/repo/issues/#{number}"
       }}
    end

    def comment(_repo, _issue, _body), do: :ok
    def issue_has_label?(_repo, _issue, _label), do: {:ok, false}
    def issue_comments(_repo, _issue), do: {:ok, []}
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "fixer_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "fixer_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(Fixer)
    stop_process(Store)
    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    # Inject mocks
    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_tracker = Application.get_env(:conductor, :tracker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)

    Application.put_env(:conductor, :code_host_module, MockCodeHost)
    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :tracker_module, MockTracker)
    Application.put_env(:conductor, :workspace_module, MockWorkspace)

    MockState.put(:test_pid, self())

    on_exit(fn ->
      stop_process(Fixer)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Store)
      MockState.cleanup()

      for {key, orig} <- [
            {:code_host_module, orig_code_host},
            {:worker_module, orig_worker},
            {:tracker_module, orig_tracker},
            {:workspace_module, orig_workspace}
          ] do
        if orig,
          do: Application.put_env(:conductor, key, orig),
          else: Application.delete_env(:conductor, key)
      end

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  describe "poll triggers fixer dispatch" do
    test "dispatches fixer sprite when factory PR has failed CI" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      log =
        capture_log(fn ->
          {:ok, _pid} =
            Fixer.start_link(
              repo: "test/repo",
              fixer_sprite: "bb-thorn",
              poll_ms: 50
            )

          assert_receive {:persona_synced, "bb-thorn", "/home/sprite/workspace/repo", :thorn},
                         2_000

          assert_receive {:dispatched, "bb-thorn", prompt}, 2_000
          assert prompt =~ "Repository Root: /home/sprite/workspace/repo"
          assert prompt =~ "CI"
        end)

      assert log =~ "[thorn] PR #42 has red CI, dispatching Thorn"
    end

    test "skips PRs when CI has not failed (green or pending)" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      {:ok, _pid} =
        Fixer.start_link(
          repo: "test/repo",
          fixer_sprite: "bb-thorn",
          poll_ms: 50
        )

      refute_receive {:dispatched, _, _}, 300
    end

    test "dispatches fixer for non-factory PRs with failed CI" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "fix/cerberus-permissions",
             "title" => "fix: cerberus permissions",
             "body" => "Fixes permissions",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      {:ok, _pid} =
        Fixer.start_link(
          repo: "test/repo",
          fixer_sprite: "bb-thorn",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-thorn", _prompt}, 2_000
    end

    test "does not dispatch when fixer is already working on a PR" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      # Slow dispatch so in-flight tracking is testable
      MockState.put(:dispatch_delay_ms, 500)

      {:ok, _pid} =
        Fixer.start_link(
          repo: "test/repo",
          fixer_sprite: "bb-thorn",
          poll_ms: 50
        )

      # First dispatch should happen
      assert_receive {:dispatched, "bb-thorn", _}, 2_000
      # Second dispatch for same PR should not happen while first is in-flight
      refute_receive {:dispatched, "bb-thorn", _}, 300
    end
  end

  describe "status/0" do
    test "returns current fixer state with health" do
      {:ok, _pid} =
        Fixer.start_link(
          repo: "test/repo",
          fixer_sprite: "bb-thorn",
          poll_ms: 60_000
        )

      status = Fixer.status()
      assert status.repo == "test/repo"
      assert status.fixer_sprite == "bb-thorn"
      assert is_map(status.in_flight)
      assert status.health == :healthy
      assert status.failure_count == 0
    end
  end

  describe "dispatch failure backoff" do
    defmodule FailingWorker do
      @behaviour Conductor.Worker
      alias Conductor.FixerTest.MockState

      def exec(_worker, _cmd, _opts), do: {:ok, ""}

      def dispatch(_worker, _prompt, _repo, _opts) do
        send(MockState.get(:test_pid, self()), :dispatch_attempted)
        {:error, "sprite unreachable", 1}
      end

      def cleanup(_worker, _repo, _run_id), do: :ok
      def busy?(_worker, _opts), do: false
    end

    test "backs off after dispatch failure" do
      Application.put_env(:conductor, :worker_module, FailingWorker)

      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 100,
             "headRefName" => "factory/100-12345",
             "title" => "test",
             "body" => "",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      {:ok, pid} =
        Fixer.start_link(
          repo: "test/repo",
          fixer_sprite: "bb-thorn",
          poll_ms: 50
        )

      assert_receive :dispatch_attempted, 2_000

      status = Fixer.status()
      assert status.health == :degraded
      assert status.failure_count == 1
      assert Process.alive?(pid)
    end

    test "survives task crash (async_nolink)" do
      defmodule CrashingWorker do
        @behaviour Conductor.Worker
        alias Conductor.FixerTest.MockState

        def exec(_worker, _cmd, _opts), do: {:ok, ""}

        def dispatch(_worker, _prompt, _repo, _opts) do
          send(MockState.get(:test_pid, self()), :crash_dispatch)
          raise "boom"
        end

        def cleanup(_worker, _repo, _run_id), do: :ok
        def busy?(_worker, _opts), do: false
      end

      Application.put_env(:conductor, :worker_module, CrashingWorker)

      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 101,
             "headRefName" => "factory/101-12345",
             "title" => "test",
             "body" => "",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      {:ok, pid} =
        Fixer.start_link(
          repo: "test/repo",
          fixer_sprite: "bb-thorn",
          poll_ms: 50
        )

      assert_receive :crash_dispatch, 2_000
      Process.sleep(100)

      assert Process.alive?(pid)
    end
  end
end
