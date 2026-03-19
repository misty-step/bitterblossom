defmodule Conductor.RunServerTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog

  alias Conductor.{Store, RunServer}

  # --- Mock State (shared across processes via persistent_term) ---

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

  # --- Mock Worker (Conductor.Worker behaviour) ---

  defmodule MockWorker do
    @behaviour Conductor.Worker
    alias Conductor.RunServerTest.MockState

    def exec(sprite, cmd, _opts) do
      case Regex.run(~r/^cat '(.+)'$/, cmd, capture: :all_but_first) do
        [path] -> MockState.get({:file_read, sprite, path}, {:error, "not found", 1})
        _ -> {:ok, ""}
      end
    end

    def dispatch(sprite, _prompt, _repo, _opts) do
      MockState.get({:dispatch_result, sprite}, {:ok, ""})
    end

    def cleanup(_sprite, _repo, _run_id), do: :ok
    def kill(_sprite), do: :ok
  end

  defmodule MockCodeHost do
    @behaviour Conductor.CodeHost
    alias Conductor.RunServerTest.MockState

    def checks_green?(_repo, _pr_number), do: true
    def checks_failed?(_repo, _pr_number), do: false
    def ci_status(_repo, _pr_number), do: {:ok, %{state: :green, summary: "mock ci", pending: []}}
    def merge(_repo, _pr_number, _opts), do: :ok
    def labeled_prs(_repo, _label), do: {:ok, []}
    def factory_prs(_repo), do: {:ok, []}
    def pr_review_comments(_repo, _pr_number), do: {:ok, []}
    def pr_ci_failure_logs(_repo, _pr_number), do: {:ok, ""}
    def add_label(_repo, _pr_number, _label), do: :ok
    def close_issue(_repo, _issue_number), do: :ok

    def close_pr(repo, pr_number, opts \\ []) do
      closed = MockState.get(:closed_prs, [])
      MockState.put(:closed_prs, closed ++ [{repo, pr_number, opts}])
      :ok
    end

    def open_prs(_repo), do: {:ok, []}

    def issue_open_prs(repo, issue_number),
      do: MockState.get({:issue_open_prs, repo, issue_number}, {:ok, []})

    def find_open_pr(repo, issue_number, expected_branch \\ nil) do
      result =
        case expected_branch do
          nil ->
            MockState.get({:open_pr, repo, issue_number}, {:error, :not_found})

          branch ->
            MockState.get(
              {:open_pr_force, repo, issue_number, branch},
              MockState.get(
                {:open_pr_exact, repo, issue_number, branch},
                MockState.get({:open_pr, repo, issue_number}, {:error, :not_found})
              )
            )
        end

      case result do
        {:force_ok, %{"headRefName" => _head_ref} = pr} ->
          {:ok, pr}

        {:ok, %{"headRefName" => head_ref} = pr} ->
          if is_nil(expected_branch) or head_ref == expected_branch,
            do: {:ok, pr},
            else: {:error, :not_found}

        {:ok, pr} ->
          case MockState.get({:prepared_branch, repo, issue_number}) do
            nil ->
              {:ok, pr}

            branch ->
              pr = Map.put(pr, "headRefName", branch)

              if is_nil(expected_branch) or branch == expected_branch,
                do: {:ok, pr},
                else: {:error, :not_found}
          end

        other ->
          other
      end
    end

    def pr_state(_repo, _pr_number), do: {:ok, "OPEN"}
  end

  # --- Mock Workspace ---

  defmodule MockWorkspace do
    alias Conductor.RunServerTest.MockState

    def prepare(_sprite, repo, _run_id, branch) do
      remember_branch(repo, branch)
      MockState.get(:workspace_result, {:ok, "/tmp/test-worktree"})
    end

    def adopt_branch(_sprite, repo, _run_id, branch) do
      remember_branch(repo, branch)
      MockState.get(:workspace_result, {:ok, "/tmp/test-worktree"})
    end

    defp remember_branch(repo, branch) do
      case parse_issue_number(branch) do
        {:ok, issue_number} -> MockState.put({:prepared_branch, repo, issue_number}, branch)
        :error -> :ok
      end
    end

    defp parse_issue_number("factory/" <> rest) do
      case String.split(rest, "-", parts: 2) do
        [issue_number, _suffix] ->
          case Integer.parse(issue_number) do
            {number, ""} -> {:ok, number}
            _ -> :error
          end

        _ ->
          :error
      end
    end

    defp parse_issue_number(_branch), do: :error
  end

  # --- Mock Tracker ---

  defmodule MockTracker do
    @behaviour Conductor.Tracker
    alias Conductor.RunServerTest.MockState

    def get_issue(_repo, _number), do: {:error, :not_found}
    def list_eligible(_repo, _opts), do: []
    def issue_has_label?(_repo, _issue, _label), do: {:ok, false}
    def issue_comments(_repo, _issue), do: {:ok, []}

    def comment(_repo, issue_number, body) do
      comments = MockState.get({:comments, issue_number}, [])
      MockState.put({:comments, issue_number}, comments ++ [body])
      :ok
    end
  end

  # --- Slow Worker for operator_block tests ---

  defmodule SlowWorker do
    @behaviour Conductor.Worker

    def exec(_sprite, _cmd, _opts), do: {:ok, ""}

    def dispatch(_sprite, _prompt, _repo, _opts) do
      Process.sleep(30_000)
      {:ok, ""}
    end

    def cleanup(_sprite, _repo, _run_id), do: :ok
    def kill(_sprite), do: :ok
  end

  # --- Crashing Worker for crash tests ---

  defmodule CrashingWorker do
    @behaviour Conductor.Worker

    def exec(_sprite, _cmd, _opts), do: {:ok, ""}

    def dispatch(_sprite, _prompt, _repo, _opts) do
      raise "simulated crash"
    end

    def cleanup(_sprite, _repo, _run_id), do: :ok
  end

  # --- Helpers ---

  defp test_issue(number \\ 42) do
    %Conductor.Issue{
      number: number,
      title: "test issue #{number}",
      body: "## Problem\ntest\n## Acceptance Criteria\ntest",
      url: "https://example.test/issues/#{number}"
    }
  end

  defp start_run_server(opts \\ []) do
    issue = Keyword.get(opts, :issue, test_issue())
    worker = Keyword.get(opts, :worker, "test-sprite")
    repo = Keyword.get(opts, :repo, "test/repo")
    extra = Keyword.drop(opts, [:issue, :worker, :repo])
    RunServer.start_link([repo: repo, issue: issue, worker: worker] ++ extra)
  end

  defp wait_for_exit(pid, timeout \\ 5_000) do
    ref = Process.monitor(pid)
    assert_receive {:DOWN, ^ref, :process, ^pid, _reason}, timeout
  end

  defp find_run(issue_number) do
    Store.list_runs(limit: 50)
    |> Enum.find(&(&1["issue_number"] == issue_number))
  end

  defp event_types(run_id) do
    Store.list_events(run_id)
    |> Enum.map(& &1["event_type"])
  end

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

  # --- Setup ---

  setup do
    db_path = Path.join(System.tmp_dir!(), "rs_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "rs_test_#{:rand.uniform(999_999)}.jsonl")

    if Process.whereis(Store), do: GenServer.stop(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    originals = %{
      worker: Application.get_env(:conductor, :worker_module),
      workspace: Application.get_env(:conductor, :workspace_module),
      tracker: Application.get_env(:conductor, :tracker_module),
      code_host: Application.get_env(:conductor, :code_host_module)
    }

    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :workspace_module, MockWorkspace)
    Application.put_env(:conductor, :tracker_module, MockTracker)
    Application.put_env(:conductor, :code_host_module, MockCodeHost)

    MockState.cleanup()

    on_exit(fn ->
      MockState.cleanup()

      for {key, orig} <- originals do
        config_key = :"#{key}_module"

        if orig,
          do: Application.put_env(:conductor, config_key, orig),
          else: Application.delete_env(:conductor, config_key)
      end

      if pid = Process.whereis(Store) do
        if Process.alive?(pid), do: catch_exit(GenServer.stop(Store))
      end

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  # --- AC1: pending → building → pr_opened lifecycle ---

  describe "AC1: successful lifecycle" do
    setup do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, "build complete"})

      MockState.put(
        {:open_pr, "test/repo", 42},
        {:ok, %{"number" => 123, "url" => "https://github.com/test/repo/pull/123"}}
      )

      :ok
    end

    test "run progresses through all phases to pr_opened" do
      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "pr_opened"
      assert run["pr_number"] == 123
    end

    test "records events at each phase transition" do
      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      types = event_types(run["run_id"])

      assert "lease_acquired" in types
      assert "builder_workspace_prepared" in types
      assert "builder_dispatched" in types
      assert "builder_complete" in types
      assert "builder_pr_detected" in types
      assert "workspace_cleaned" in types
    end

    test "stores the detected PR URL for downstream governance" do
      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert run["pr_url"] == "https://github.com/test/repo/pull/123"
    end

    test "lease is held after pr_opened — released at merge by orchestrator" do
      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      assert Store.leased?("test/repo", 42)
    end

    test "logs Weaver-prefixed lifecycle messages" do
      log =
        capture_log(fn ->
          {:ok, pid} = start_run_server()
          wait_for_exit(pid)
        end)

      assert log =~ "[weaver][run-42-"
      assert log =~ "dispatching Weaver"
      assert log =~ "Weaver opened PR #123"
    end
  end

  # --- AC1 variant: adopt_branch path ---

  describe "AC1: adopt existing branch" do
    setup do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:open_pr, "test/repo", 42},
        {:ok, %{"number" => 999, "url" => "https://github.com/test/repo/pull/999"}}
      )

      :ok
    end

    test "uses adopt_branch instead of prepare when existing_branch given" do
      {:ok, pid} =
        start_run_server(
          existing_branch: "factory/42-1234567890",
          existing_pr_number: 999,
          existing_pr_url: "https://github.com/test/repo/pull/999"
        )

      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "pr_opened"
      assert run["pr_number"] == 999

      types = event_types(run["run_id"])
      assert "lease_acquired" in types
      assert "builder_workspace_prepared" in types
      assert "builder_pr_detected" in types
    end
  end

  # --- AC3: builder dispatch failure ---

  describe "AC3: dispatch error" do
    setup do
      MockState.put({:dispatch_result, "test-sprite"}, {:error, "SEGFAULT", 139})
      :ok
    end

    test "marks run failed" do
      log =
        capture_log(fn ->
          {:ok, pid} = start_run_server()
          wait_for_exit(pid)
        end)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert log =~ "[weaver][run-42-"
      assert log =~ "builder_dispatch_failed: exit 139: SEGFAULT"
    end

    test "lease released" do
      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      refute Store.leased?("test/repo", 42)
    end

    test "records builder_dispatch_failed event" do
      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert "builder_dispatch_failed" in event_types(run["run_id"])
    end
  end

  describe "AC3: dispatch task crash" do
    test "crash kills RunServer via link — stale-run detection handles cleanup" do
      # Task.async links to RunServer. A task crash sends an EXIT signal
      # that kills RunServer before the :DOWN handler can fire. The run
      # stays in "building" until the Orchestrator's stale-run reconciler
      # expires it. This is the actual production behavior.
      Application.put_env(:conductor, :worker_module, CrashingWorker)

      Process.flag(:trap_exit, true)

      {:ok, pid} = start_run_server()

      # RunServer dies from the linked task crash
      assert_receive {:EXIT, ^pid, _reason}, 5_000

      run = find_run(42)
      # Run stays in building — the :DOWN handler never fires
      assert run["phase"] == "building"
      # Lease stays held — stale-run detection cleans this up
      assert Store.leased?("test/repo", 42)
    end
  end

  # --- Additional failure paths ---

  describe "workspace preparation failure" do
    test "workspace error transitions to failed" do
      MockState.put(:workspace_result, {:error, "ssh timeout"})

      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "workspace_preparation_failed" in event_types(run["run_id"])
      refute Store.leased?("test/repo", 42)
    end
  end

  describe "already-leased issue" do
    test "stops immediately without creating a run" do
      :ok = Store.acquire_lease("test/repo", 42, "existing-run")

      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      assert Store.leased?("test/repo", 42)
    end
  end

  describe "missing PR after dispatch" do
    test "marks run failed" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "pr_not_found" in event_types(run["run_id"])
    end

    test "releases the lease when builder exits without opening a PR" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      refute Store.leased?("test/repo", 42)
    end
  end

  describe "blocked run after dispatch" do
    test "marks run blocked when BLOCKED.md exists and no PR is found" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:file_read, "test-sprite", "/tmp/test-worktree/BLOCKED.md"},
        {:ok, "need operator input"}
      )

      log =
        capture_log(fn ->
          {:ok, pid} = start_run_server()
          wait_for_exit(pid)
        end)

      run = find_run(42)
      assert run["phase"] == "blocked"
      assert "run_blocked" in event_types(run["run_id"])
      assert log =~ "[weaver][run-42-"
      assert log =~ "blocked: need operator input"

      assert MockState.get({:comments, 42}) == [
               "Bitterblossom blocked `#{run["run_id"]}`: need operator input"
             ]

      refute Store.leased?("test/repo", 42)
    end

    test "fails truthfully when BLOCKED.md cannot be read" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:file_read, "test-sprite", "/tmp/test-worktree/BLOCKED.md"},
        {:error, "permission denied", 126}
      )

      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "workspace_read_error" in event_types(run["run_id"])
    end
  end

  describe "stale PR branch after dispatch" do
    test "marks run failed when lookup finds a different factory branch" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:open_pr_force, "test/repo", 42, "factory/42-1773840329"},
        {:force_ok,
         %{
           "number" => 123,
           "url" => "https://github.com/test/repo/pull/123",
           "headRefName" => "factory/42-older-run"
         }}
      )

      {:ok, pid} = start_run_server(existing_branch: "factory/42-1773840329")
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "pr_branch_mismatch" in event_types(run["run_id"])
    end

    test "finds the current run branch when an older factory PR also exists" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:open_pr, "test/repo", 42},
        {:ok,
         %{
           "number" => 456,
           "url" => "https://github.com/test/repo/pull/456",
           "headRefName" => "factory/42-older-run"
         }}
      )

      MockState.put(
        {:open_pr_exact, "test/repo", 42, "factory/42-1234567890"},
        {:ok,
         %{
           "number" => 999,
           "url" => "https://github.com/test/repo/pull/999",
           "headRefName" => "factory/42-1234567890"
         }}
      )

      {:ok, pid} =
        start_run_server(
          existing_branch: "factory/42-1234567890",
          existing_pr_number: 999,
          existing_pr_url: "https://github.com/test/repo/pull/999"
        )

      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "pr_opened"
      assert run["pr_number"] == 999
      assert "builder_pr_detected" in event_types(run["run_id"])
    end
  end

  describe "duplicate non-factory PR after dispatch" do
    test "closes the foreign PR and fails the run truthfully" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:open_pr_exact, "test/repo", 42, "factory/42-1234567890"},
        {:ok,
         %{
           "number" => 999,
           "url" => "https://github.com/test/repo/pull/999",
           "headRefName" => "factory/42-1234567890"
         }}
      )

      MockState.put(
        {:issue_open_prs, "test/repo", 42},
        {:ok,
         [
           %{
             "number" => 999,
             "url" => "https://github.com/test/repo/pull/999",
             "headRefName" => "factory/42-1234567890"
           },
           %{
             "number" => 1000,
             "url" => "https://github.com/test/repo/pull/1000",
             "headRefName" => "cx/issue-42-shadow"
           }
         ]}
      )

      {:ok, pid} = start_run_server(existing_branch: "factory/42-1234567890")
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "unexpected_issue_prs" in event_types(run["run_id"])

      assert MockState.get(:closed_prs) == [
               {"test/repo", 1000,
                [
                  comment:
                    "Bitterblossom closed this PR because issue #42 is leased to `factory/42-1234567890` and duplicate foreign-branch PRs are not governable."
                ]}
             ]
    end
  end

  describe "PR lookup failure" do
    test "lookup error marks run failed" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})
      MockState.put({:open_pr, "test/repo", 42}, {:error, :api_down})

      {:ok, pid} = start_run_server()
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "pr_detection_failed" in event_types(run["run_id"])
    end
  end

  describe "PR lookup returns incomplete data" do
    test "fails when PR is missing url or number" do
      MockState.put({:dispatch_result, "test-sprite"}, {:ok, ""})

      MockState.put(
        {:open_pr_exact, "test/repo", 42, "factory/42-1234567890"},
        {:ok, %{"headRefName" => "factory/42-1234567890"}}
      )

      {:ok, pid} = start_run_server(existing_branch: "factory/42-1234567890")
      wait_for_exit(pid)

      run = find_run(42)
      assert run["phase"] == "failed"
      assert "pr_detection_failed" in event_types(run["run_id"])
    end
  end

  describe "operator_block/2" do
    test "blocks active run and records reason" do
      Application.put_env(:conductor, :worker_module, SlowWorker)

      {:ok, pid} = start_run_server()

      eventually(fn ->
        status = RunServer.status(pid)
        assert status.phase == :building
      end)

      :ok = RunServer.operator_block(pid, "operator cancelled")

      # operator_block stops the GenServer synchronously via handle_call
      # so the process is already dead after the call returns
      eventually(fn ->
        refute Process.alive?(pid)
      end)

      run = find_run(42)
      assert run["phase"] == "blocked"
      assert "run_blocked" in event_types(run["run_id"])
      refute Store.leased?("test/repo", 42)
    end
  end
end
