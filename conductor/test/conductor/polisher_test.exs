defmodule Conductor.PolisherTest do
  use ExUnit.Case, async: false

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

    def pr_ci_failure_logs(_repo, _pr_number), do: {:ok, ""}
    def add_label(_repo, _pr_number, _label), do: :ok
    def find_open_pr(_repo, _issue_number), do: {:error, :not_found}
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

    def read_artifact(_worker, _path, _opts), do: {:ok, %{"status" => "ready"}}
    def cleanup(_worker, _repo, _run_id), do: :ok
    def busy?(_worker, _opts), do: false
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

  defp stop_process(name) do
    try do
      GenServer.stop(name)
    catch
      :exit, _reason -> :ok
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "polisher_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "polisher_test_#{:rand.uniform(999_999)}.jsonl")

    if Process.whereis(Store), do: GenServer.stop(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_tracker = Application.get_env(:conductor, :tracker_module)

    Application.put_env(:conductor, :code_host_module, MockCodeHost)
    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :tracker_module, MockTracker)

    MockState.put(:test_pid, self())

    on_exit(fn ->
      if pid = Process.whereis(Polisher),
        do: if(Process.alive?(pid), do: stop_process(pid))

      if pid = Process.whereis(Store), do: if(Process.alive?(pid), do: stop_process(pid))
      MockState.cleanup()

      for {key, orig} <- [
            {:code_host_module, orig_code_host},
            {:worker_module, orig_worker},
            {:tracker_module, orig_tracker}
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

      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-polisher",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-polisher", prompt}, 2_000
      assert prompt =~ "review"
    end

    test "dispatches polisher for non-factory PR with green CI" do
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
          polisher_sprite: "bb-polisher",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-polisher", _prompt}, 2_000
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
          polisher_sprite: "bb-polisher",
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
          polisher_sprite: "bb-polisher",
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
          polisher_sprite: "bb-polisher",
          poll_ms: 50
        )

      assert_receive {:dispatched, "bb-polisher", _}, 2_000
      refute_receive {:dispatched, "bb-polisher", _}, 300
    end
  end

  describe "status/0" do
    test "returns current polisher state" do
      {:ok, _pid} =
        Polisher.start_link(
          repo: "test/repo",
          polisher_sprite: "bb-polisher",
          poll_ms: 60_000
        )

      status = Polisher.status()
      assert status.repo == "test/repo"
      assert status.polisher_sprite == "bb-polisher"
      assert is_map(status.in_flight)
    end
  end
end
