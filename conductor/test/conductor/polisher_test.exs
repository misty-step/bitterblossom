defmodule Conductor.PolisherTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{Store, Polisher}

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

  # Mock code host
  defmodule MockCodeHost do
    @behaviour Conductor.CodeHost
    alias Conductor.PolisherTest.MockState

    def checks_green?(_repo, pr_number) do
      case MockState.get(:checks_green) do
        fun when is_function(fun) -> fun.(pr_number)
        _ -> true
      end
    end

    def checks_failed?(_repo, _pr_number), do: false
    def ci_status(_repo, _pr_number), do: {:ok, %{state: :green, summary: "mock ci", pending: []}}

    def merge(_repo, _pr, _opts), do: :ok

    def labeled_prs(_repo, label) do
      prs = MockState.get(:labeled_prs, %{})
      {:ok, Map.get(prs, label, [])}
    end

    def open_prs(_repo), do: MockState.get(:open_prs, {:ok, []})

    def pr_review_comments(_repo, _pr_number) do
      MockState.get(:review_comments, {:ok, []})
    end

    def pr_substantive_change_at(_repo, pr_number) do
      MockState.get({:substantive_change_at, pr_number}, {:error, :not_found})
    end

    def pr_ci_failure_logs(_repo, _pr_number), do: {:ok, ""}
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

  # Mock worker
  defmodule MockWorker do
    @behaviour Conductor.Worker
    alias Conductor.PolisherTest.MockState

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
    alias Conductor.PolisherTest.MockState

    def sync_persona(worker, workspace, role, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:persona_synced, worker, workspace, role})
      :ok
    end
  end

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

  defp wait_for_pr_state(repo, pr_number, attempts \\ 20)

  defp wait_for_pr_state(_repo, _pr_number, 0) do
    flunk("timed out waiting for persisted PR state")
  end

  defp wait_for_pr_state(repo, pr_number, attempts) do
    case Store.get_pr_state(repo, pr_number) do
      {:ok, %{"polished_at" => polished_at} = pr} when is_binary(polished_at) ->
        pr

      _ ->
        Process.sleep(25)
        wait_for_pr_state(repo, pr_number, attempts - 1)
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "polisher_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "polisher_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(Polisher)
    stop_process(Store)
    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

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
      stop_process(Polisher)
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

  describe "poll triggers polisher dispatch" do
    @green_checks [%{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}]
    @red_checks [%{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"}]

    test "dispatches polisher when factory PR has green CI and no lgtm" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 42,
             "headRefName" => "factory/99-12345",
             "title" => "feat: implement feature",
             "body" => "Closes #99",
             "labels" => [%{"name" => "feature"}],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      MockState.put(
        :review_comments,
        {:ok,
         [
           %{"author" => "reviewer", "body" => "Please rename this variable"}
         ]}
      )

      log =
        capture_log(fn ->
          {:ok, _pid} =
            Polisher.start_link(
              repo: "test/repo",
              polisher_sprite: "bb-fern",
              poll_ms: 50
            )

          assert_receive {:persona_synced, "bb-fern", "/home/sprite/workspace/repo", :fern},
                         2_000

          assert_receive {:dispatched, "bb-fern", prompt}, 2_000
          assert prompt =~ "Repository Root: /home/sprite/workspace/repo"
          assert prompt =~ "review"
        end)

      assert log =~ "[fern] PR #42 is green, dispatching Fern"
    end

    test "conductor-tracked PR gets lgtm authority in prompt" do
      # Create a store run so conductor_managed? returns true
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

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-fern", prompt}, 2_000
      assert prompt =~ "gh pr edit --add-label lgtm"
      refute prompt =~ "Do NOT add the `lgtm` label"
    end

    test "dispatches polisher for non-factory PR without lgtm permission" do
      MockState.put(
        :open_prs,
        {:ok,
         [
           %{
             "number" => 55,
             "headRefName" => "fix/cerberus-permissions",
             "title" => "fix: cerberus permissions",
             "body" => "Fixes permissions",
             "labels" => [],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-fern", prompt}, 2_000
      # Non-conductor PRs get review but NOT lgtm labeling authority
      assert prompt =~ "Do NOT add the `lgtm` label"
      refute prompt =~ "gh pr edit --add-label lgtm"
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
             "statusCheckRollup" => @red_checks
           }
         ]}
      )

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      refute_receive {:dispatched, _, _}, 300
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

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      refute_receive {:dispatched, _, _}, 300
    end

    test "skips PRs with LGTM label (case-insensitive match)" do
      MockState.put(
        :factory_prs,
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

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      refute_receive {:dispatched, _, _}, 300
    end

    test "does not dispatch when polisher already working on a PR" do
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

      MockState.put(:dispatch_delay_ms, 500)

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-fern", _}, 2_000
      refute_receive {:dispatched, "bb-fern", _}, 300
    end

    test "does not redispatch an already-polished PR until substantive activity changes" do
      initial_change_at =
        DateTime.utc_now()
        |> DateTime.add(-60, :second)
        |> DateTime.to_iso8601()

      next_change_at =
        DateTime.utc_now()
        |> DateTime.add(60, :second)
        |> DateTime.to_iso8601()

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

      MockState.put({:substantive_change_at, 42}, {:ok, initial_change_at})

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-fern", _prompt}, 2_000

      pr = wait_for_pr_state("test/repo", 42)
      assert pr["last_substantive_change_at"] == initial_change_at
      assert is_binary(pr["polished_at"])

      stop_process(Polisher)

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      refute_receive {:dispatched, "bb-fern", _prompt}, 300

      MockState.put({:substantive_change_at, 42}, {:ok, next_change_at})

      assert_receive {:dispatched, "bb-fern", _prompt}, 2_000
    end
  end

  describe "status/0" do
    test "returns current polisher state with health" do
      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 60_000
        )

      status = Polisher.status()
      assert status.repo == "test/repo"
      assert status.polisher_sprite == "bb-fern"
      assert is_map(status.in_flight)
      assert status.health == :healthy
      assert status.failure_count == 0
    end
  end

  describe "dispatch failure backoff" do
    @green_checks [%{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}]

    defmodule FailingWorker do
      @behaviour Conductor.Worker
      alias Conductor.PolisherTest.MockState

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
             "labels" => [],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      {:ok, pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      # First dispatch attempt
      assert_receive :dispatch_attempted, 2_000

      # Check status shows degraded
      status = Polisher.status()
      assert status.health == :degraded
      assert status.failure_count == 1

      assert Process.alive?(pid)
    end

    test "survives task crash (async_nolink)" do
      defmodule CrashingWorker do
        @behaviour Conductor.Worker
        alias Conductor.PolisherTest.MockState

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
             "labels" => [],
             "statusCheckRollup" => @green_checks
           }
         ]}
      )

      {:ok, pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-fern",
          poll_ms: 50
        )

      assert_receive :crash_dispatch, 2_000
      Process.sleep(100)

      # GenServer should still be alive after task crash
      assert Process.alive?(pid)
    end
  end
end
