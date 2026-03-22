defmodule Conductor.PhaseWorkerTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{PhaseWorker, Store}
  alias Conductor.PhaseWorker.Roles

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

  defmodule MockCodeHost do
    @behaviour Conductor.CodeHost
    alias Conductor.PhaseWorkerTest.MockState

    def checks_green?(_repo, _pr_number), do: false
    def checks_failed?(_repo, _pr_number), do: false
    def ci_status(_repo, _pr_number), do: {:ok, %{state: :green, summary: "mock ci", pending: []}}
    def merge(_repo, _pr, _opts), do: :ok
    def labeled_prs(_repo, _label), do: {:ok, []}
    def open_prs(_repo), do: MockState.get(:open_prs, {:ok, []})

    def pr_review_comments(_repo, _pr_number) do
      MockState.get(:review_comments, {:ok, []})
    end

    def pr_ci_failure_logs(_repo, _pr_number) do
      MockState.get(:ci_failure_logs, {:ok, "Build failed: test_foo.ex:42 assertion error"})
    end

    def add_label(_repo, _pr_number, _label), do: :ok
    def close_issue(_repo, _issue_number), do: :ok
    def close_pr(_repo, _pr_number, _opts \\ []), do: :ok
    def find_open_pr(_repo, _issue_number, _expected_branch \\ nil), do: {:error, :not_found}
    def issue_open_prs(_repo, _issue_number), do: {:ok, []}
    def pr_state(_repo, _pr_number), do: {:ok, "OPEN"}

    def get_pr_checks(_repo, _pr_number) do
      {:ok, [%{"name" => "ci", "conclusion" => "SUCCESS", "status" => "COMPLETED"}]}
    end
  end

  defmodule MockWorker do
    @behaviour Conductor.Worker
    alias Conductor.PhaseWorkerTest.MockState

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
    alias Conductor.PhaseWorkerTest.MockState

    def sync_persona(worker, workspace, role, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:persona_synced, worker, workspace, role})
      :ok
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "phase_worker_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "phase_worker_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(Store)
    stop_process(Conductor.TaskSupervisor)
    stop_process(Conductor.PhaseWorkerRegistry)

    {:ok, _} = Registry.start_link(keys: :unique, name: Conductor.PhaseWorkerRegistry)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
    orig_phase_worker_sprites = Application.get_env(:conductor, :phase_worker_sprites)

    Application.put_env(:conductor, :code_host_module, MockCodeHost)
    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :workspace_module, MockWorkspace)

    MockState.put(:test_pid, self())

    on_exit(fn ->
      stop_process(Conductor.TaskSupervisor)
      stop_process(Store)
      stop_process(Conductor.PhaseWorkerRegistry)
      MockState.cleanup()

      restore_env(:code_host_module, orig_code_host)
      restore_env(:worker_module, orig_worker)
      restore_env(:workspace_module, orig_workspace)
      restore_env(:phase_worker_supervisor, orig_phase_worker_supervisor)
      restore_env(:phase_worker_sprites, orig_phase_worker_sprites)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  defp start_phase_worker(role_module, sprites, opts \\ []) do
    {:ok, _pid} =
      PhaseWorker.start_link(
        repo: "test/repo",
        role_module: role_module,
        sprites: sprites,
        poll_ms: Keyword.get(opts, :poll_ms, 50)
      )
  end

  defp create_run_for_pr(pr_number) do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: pr_number,
        issue_title: "tracked issue #{pr_number}",
        builder_sprite: "sprite-1"
      })

    Store.update_run(run_id, %{pr_number: pr_number, phase: "pr_opened", status: "pr_opened"})
    run_id
  end

  defp event_types(run_id) do
    Store.list_events(run_id)
    |> Enum.map(& &1["event_type"])
  end

  describe "child_spec/1" do
    test "returns the role-keyed child spec used by the supervisor" do
      opts = [repo: "test/repo", role_module: Roles.Fixer, sprites: ["bb-thorn"]]

      assert %{
               id: {PhaseWorker, Roles.Fixer},
               start: {PhaseWorker, :start_link, [^opts]}
             } = PhaseWorker.child_spec(opts)
    end
  end

  describe "fixer role" do
    test "dispatches thorn when a PR has failed CI" do
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
          start_phase_worker(Roles.Fixer, ["bb-thorn"])

          assert_receive {:persona_synced, "bb-thorn", "/home/sprite/workspace/repo", :thorn},
                         2_000

          assert_receive {:dispatched, "bb-thorn", prompt}, 2_000
          assert prompt =~ "Repository Root: /home/sprite/workspace/repo"
          assert prompt =~ "CI"
        end)

      assert log =~ "[thorn] PR #42 has red CI, dispatching Thorn"
    end

    test "records fixer events on the tracked run instead of the role prefix bucket" do
      run_id = create_run_for_pr(42)

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

      start_phase_worker(Roles.Fixer, ["bb-thorn"])

      assert_receive {:dispatched, "bb-thorn", _prompt}, 2_000
      Process.sleep(100)

      assert "fixer_dispatched" in event_types(run_id)
      assert "fixer_complete" in event_types(run_id)
      assert Store.list_events("fixer") == []
    end

    test "skips PRs when CI has not failed" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "green",
             "body" => "",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
             ]
           },
           %{
             "number" => 43,
             "headRefName" => "factory/100-12345",
             "title" => "pending",
             "body" => "",
             "statusCheckRollup" => [%{"name" => "CI", "conclusion" => nil, "status" => "QUEUED"}]
           }
         ]}
      )

      start_phase_worker(Roles.Fixer, ["bb-thorn"])

      refute_receive {:dispatched, _, _}, 300
    end
  end

  describe "polisher role" do
    @green_checks [%{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}]

    test "dispatches fern when a conductor-managed PR is green and unlabeled" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 99,
          issue_title: "tracked issue",
          builder_sprite: "sprite-1"
        })

      Store.update_run(run_id, %{pr_number: 42, phase: "pr_opened", status: "pr_opened"})

      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "labels" => [],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      MockState.put(
        :review_comments,
        {:ok, [%{"author" => "reviewer", "body" => "Please rename this variable"}]}
      )

      log =
        capture_log(fn ->
          start_phase_worker(Roles.Polisher, ["bb-fern"])

          assert_receive {:persona_synced, "bb-fern", "/home/sprite/workspace/repo", :fern},
                         2_000

          assert_receive {:dispatched, "bb-fern", prompt}, 2_000
          assert prompt =~ "Repository Root: /home/sprite/workspace/repo"
          assert prompt =~ "gh pr edit --add-label lgtm"
        end)

      assert log =~ "[fern] PR #42 is green, dispatching Fern"
    end

    test "skips PRs that already have lgtm label" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "labels" => [%{"name" => "lgtm"}],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      start_phase_worker(Roles.Polisher, ["bb-fern"])

      refute_receive {:dispatched, _, _}, 300
    end

    test "skips PRs with LGTM label case-insensitively" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "labels" => [%{"name" => "LGTM"}],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      start_phase_worker(Roles.Polisher, ["bb-fern"])

      refute_receive {:dispatched, _, _}, 300
    end

    test "skips PRs with red CI" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "labels" => [],
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      start_phase_worker(Roles.Polisher, ["bb-fern"])

      refute_receive {:dispatched, _, _}, 300
    end
  end

  describe "status/1" do
    test "reports current state for a role worker" do
      start_phase_worker(Roles.Fixer, ["bb-thorn"], poll_ms: 60_000)

      status = PhaseWorker.status(Roles.Fixer)
      assert status.repo == "test/repo"
      assert status.sprites == ["bb-thorn"]
      assert status.role == :fixer
      assert is_map(status.in_flight)
      assert status.health == :healthy
      assert status.failure_count == 0
    end
  end

  describe "statuses/0" do
    test "returns all running role workers" do
      start_phase_worker(Roles.Fixer, ["bb-thorn"], poll_ms: 60_000)
      start_phase_worker(Roles.Polisher, ["bb-fern"], poll_ms: 60_000)

      statuses =
        PhaseWorker.statuses()
        |> Enum.sort_by(& &1.role)

      assert Enum.map(statuses, & &1.role) == [:fixer, :polisher]
      assert Enum.map(statuses, & &1.sprites) == [["bb-thorn"], ["bb-fern"]]
    end
  end

  describe "shared worker behavior" do
    defmodule NoStoredSpritesSupervisor do
    end

    test "uses provided sprites when the configured supervisor does not expose stored pools" do
      orig_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
      Application.put_env(:conductor, :phase_worker_supervisor, NoStoredSpritesSupervisor)

      on_exit(fn -> restore_env(:phase_worker_supervisor, orig_supervisor) end)

      start_phase_worker(Roles.Fixer, ["bb-thorn"], poll_ms: 60_000)

      assert PhaseWorker.status(Roles.Fixer).sprites == ["bb-thorn"]
    end

    test "does not redispatch work while the only sprite is busy" do
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

      MockState.put(:dispatch_delay_ms, 500)

      start_phase_worker(Roles.Fixer, ["bb-thorn"])

      assert_receive {:dispatched, "bb-thorn", _prompt}, 2_000
      refute_receive {:dispatched, "bb-thorn", _prompt}, 300
    end

    defmodule FailingWorker do
      @behaviour Conductor.Worker
      alias Conductor.PhaseWorkerTest.MockState

      def exec(_worker, _cmd, _opts), do: {:ok, ""}

      def dispatch(_worker, _prompt, _repo, _opts) do
        send(MockState.get(:test_pid, self()), :dispatch_attempted)
        {:error, "sprite unreachable", 1}
      end

      def cleanup(_worker, _repo, _run_id), do: :ok
      def busy?(_worker, _opts), do: false
    end

    defmodule CrashingWorker do
      @behaviour Conductor.Worker
      alias Conductor.PhaseWorkerTest.MockState

      def exec(_worker, _cmd, _opts), do: {:ok, ""}

      def dispatch(_worker, _prompt, _repo, _opts) do
        send(MockState.get(:test_pid, self()), :crash_dispatch)
        exit(:boom)
      end

      def cleanup(_worker, _repo, _run_id), do: :ok
      def busy?(_worker, _opts), do: false
    end

    test "backs off after dispatch failure" do
      orig_worker = Application.get_env(:conductor, :worker_module)
      Application.put_env(:conductor, :worker_module, FailingWorker)

      on_exit(fn -> restore_env(:worker_module, orig_worker) end)

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

      start_phase_worker(Roles.Fixer, ["bb-thorn"])

      assert_receive :dispatch_attempted, 2_000

      status = PhaseWorker.status(Roles.Fixer)
      assert status.health == :degraded
      assert status.failure_count == 1
    end

    test "marks the worker unavailable after repeated dispatch failures" do
      orig_worker = Application.get_env(:conductor, :worker_module)
      Application.put_env(:conductor, :worker_module, FailingWorker)

      on_exit(fn -> restore_env(:worker_module, orig_worker) end)

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

      {:ok, pid} = start_phase_worker(Roles.Fixer, ["bb-thorn"], poll_ms: 10)

      assert_receive :dispatch_attempted, 2_000
      assert_receive :dispatch_attempted, 2_000
      assert_receive :dispatch_attempted, 2_000

      Process.sleep(50)

      status = PhaseWorker.status(Roles.Fixer)
      assert status.health == :unavailable
      assert status.failure_count >= 3
      assert Process.alive?(pid)
    end

    test "survives task crashes from async_nolink dispatch tasks" do
      orig_worker = Application.get_env(:conductor, :worker_module)
      Application.put_env(:conductor, :worker_module, CrashingWorker)

      on_exit(fn -> restore_env(:worker_module, orig_worker) end)

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

      {:ok, pid} = start_phase_worker(Roles.Fixer, ["bb-thorn"])

      assert_receive :crash_dispatch, 2_000
      Process.sleep(100)

      status = PhaseWorker.status(Roles.Fixer)
      assert status.health == :degraded
      assert status.failure_count >= 1
      assert Process.alive?(pid)
    end
  end

  describe "multi-sprite dispatch" do
    test "dispatches two eligible PRs in parallel when the role has two idle sprites" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 101,
             "headRefName" => "factory/101-12345",
             "title" => "first",
             "body" => "",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           },
           %{
             "number" => 102,
             "headRefName" => "factory/102-12345",
             "title" => "second",
             "body" => "",
             "statusCheckRollup" => [
               %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}
             ]
           }
         ]}
      )

      MockState.put(:dispatch_delay_ms, 500)

      start_phase_worker(Roles.Fixer, ["bb-thorn-1", "bb-thorn-2"])

      assert_receive {:dispatched, first_worker, _prompt}, 2_000
      assert_receive {:dispatched, second_worker, _prompt}, 2_000

      assert Enum.sort([first_worker, second_worker]) == ["bb-thorn-1", "bb-thorn-2"]
    end
  end
end
