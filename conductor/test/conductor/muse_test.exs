defmodule Conductor.MuseTest do
  use ExUnit.Case, async: false
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{Muse, Store}

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

  defmodule MockWorker do
    @behaviour Conductor.Worker
    alias Conductor.MuseTest.MockState

    def exec(_worker, _command, _opts), do: {:ok, ""}

    def dispatch(worker, prompt, _repo, _opts) do
      send(MockState.get(:test_pid, self()), {:dispatched, worker, prompt})

      cond do
        String.contains?(prompt, "# Muse Observe Task") ->
          {:ok,
           Jason.encode!(%{
             summary: "builder hit review friction",
             reflection:
               "# Reflection\n\n- Run showed review churn.\n- Consider stronger reviewer guidance."
           })}

        String.contains?(prompt, "# Muse Synthesis Task") ->
          {:ok,
           Jason.encode!(%{
             summary: "two patterns emerged",
             actions: [
               %{
                 action: "comment_issue",
                 issue_number: 601,
                 title: "Existing pattern",
                 body: "Pattern confirmed by multiple reflections."
               },
               %{
                 action: "create_issue",
                 title: "New synthesis issue",
                 body: "Problem\n\n- A repeated root cause deserves action."
               },
               %{
                 action: "none",
                 title: "Low signal",
                 body: ""
               },
               %{
                 action: "create_issue",
                 title: "Should be trimmed",
                 body: "This action should never execute."
               }
             ]
           })}

        true ->
          {:error, "unknown prompt", 1}
      end
    end

    def cleanup(_worker, _repo, _run_id), do: :ok
    def busy?(_worker, _opts), do: false
  end

  defmodule MockWorkspace do
    alias Conductor.MuseTest.MockState

    def sync_persona(worker, workspace, role, _opts \\ []) do
      send(MockState.get(:test_pid, self()), {:persona_synced, worker, workspace, role})
      :ok
    end
  end

  defmodule MockGitHub do
    @behaviour Conductor.Tracker
    alias Conductor.MuseTest.MockState

    def list_eligible(_repo, _opts), do: []
    def get_issue(_repo, _number), do: {:error, :unsupported}
    def transition(_repo, _id, _state), do: :ok

    def list_issues(repo, _opts \\ []) do
      {:ok, MockState.get({:issues, repo}, [])}
    end

    def comment(repo, issue_number, body) do
      comments = MockState.get({:comments, repo, issue_number}, [])
      MockState.put({:comments, repo, issue_number}, comments ++ [body])
      :ok
    end

    def create_issue_comment(repo, issue_number, body), do: comment(repo, issue_number, body)

    def issue_has_label?(_repo, _issue_number, _label), do: {:ok, false}
    def issue_comments(_repo, _issue_number), do: {:ok, []}
  end

  setup do
    repo_root = Path.join(System.tmp_dir!(), "muse_repo_#{System.unique_integer([:positive])}")
    db_path = Path.join(System.tmp_dir!(), "muse_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "muse_test_#{System.unique_integer([:positive])}.jsonl")

    File.mkdir_p!(repo_root)
    File.write!(Path.join(repo_root, "project.md"), "# Project\n\nMuse test context.\n")
    File.mkdir_p!(Path.join(repo_root, ".groom"))
    File.write!(Path.join(repo_root, ".groom/BACKLOG.md"), "# Backlog\n")

    stop_conductor_app()
    stop_process(Muse)
    stop_process(Store)
    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_tracker = Application.get_env(:conductor, :tracker_module)
    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_repo_root = Application.get_env(:conductor, :repo_root)

    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :workspace_module, MockWorkspace)
    Application.put_env(:conductor, :tracker_module, MockGitHub)
    Application.put_env(:conductor, :code_host_module, MockGitHub)
    Application.put_env(:conductor, :repo_root, repo_root)

    MockState.put(:test_pid, self())
    MockState.put({:issues, "test/repo"}, [])

    on_exit(fn ->
      stop_process(Muse)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Store)
      MockState.cleanup()

      for {key, orig} <- [
            {:worker_module, orig_worker},
            {:workspace_module, orig_workspace},
            {:tracker_module, orig_tracker},
            {:code_host_module, orig_code_host},
            {:repo_root, orig_repo_root}
          ] do
        restore_env(key, orig)
      end

      File.rm_rf(repo_root)
      File.rm(db_path)
      File.rm(event_log)
    end)

    %{repo_root: repo_root}
  end

  test "observe/1 dispatches muse and writes a structured reflection file", %{
    repo_root: repo_root
  } do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 780,
        issue_title: "Muse reflection",
        builder_sprite: "bb-weaver"
      })

    Store.update_run(run_id, %{pr_number: 412, phase: "merged", status: "merged"})
    Store.record_event(run_id, "merged", %{pr_number: 412})

    {:ok, _pid} =
      Muse.start_link(
        repo: "test/repo",
        muse_sprite: "bb-muse",
        synthesis_interval_ms: 60_000
      )

    assert Muse.observe(run_id) == :ok

    assert_receive {:persona_synced, "bb-muse", "/home/sprite/workspace/repo", :muse}, 2_000
    assert_receive {:dispatched, "bb-muse", prompt}, 2_000
    assert prompt =~ "# Muse Observe Task"
    assert prompt =~ run_id

    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")

    reflection_file =
      Path.join(reflection_dir, "#{Date.utc_today()}-#{run_id}.md")

    eventually(fn ->
      assert File.exists?(reflection_file)
      content = File.read!(reflection_file)
      assert content =~ "# Reflection"
      assert content =~ "review churn"
    end)
  end

  test "daily synthesis trims to three actions and avoids duplicate issue creation", %{
    repo_root: repo_root
  } do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-2.md"), "# Reflection 2\n")

    MockState.put(
      {:issues, "test/repo"},
      [
        %Conductor.Issue{
          number: 777,
          title: "[muse] New synthesis issue",
          body: "",
          url: "https://example.test/issues/777"
        }
      ]
    )

    {:ok, _pid} =
      Muse.start_link(
        repo: "test/repo",
        muse_sprite: "bb-muse",
        synthesis_interval_ms: 60_000
      )

    assert Muse.synthesize() == :ok

    assert_receive {:persona_synced, "bb-muse", "/home/sprite/workspace/repo", :muse}, 2_000
    assert_receive {:dispatched, "bb-muse", prompt}, 2_000
    assert prompt =~ "# Muse Synthesis Task"
    assert prompt =~ "Reflection 1"

    eventually(fn ->
      comments = MockState.get({:comments, "test/repo", 601}, [])
      assert comments == ["Pattern confirmed by multiple reflections."]

      duplicate_comments = MockState.get({:comments, "test/repo", 777}, [])
      assert [note] = duplicate_comments
      assert note =~ "matched an existing open issue"

      synthesis_files = Path.wildcard(Path.join(repo_root, ".bb/muse/syntheses/*.md"))
      assert length(synthesis_files) == 1

      [event] =
        Store.list_all_events(limit: 10)
        |> Enum.filter(&(&1["event_type"] == "muse_synthesis_complete"))

      assert event["payload"]["action_count"] == 3
      assert length(event["payload"]["actions_taken"]) == 3
    end)
  end

  test "status/0 exposes muse health and queued work" do
    {:ok, _pid} =
      Muse.start_link(
        repo: "test/repo",
        muse_sprite: "bb-muse",
        synthesis_interval_ms: 60_000
      )

    status = Muse.status()
    assert status.repo == "test/repo"
    assert status.muse_sprite == "bb-muse"
    assert status.health == :healthy
    assert status.failure_count == 0
    assert status.queue_length == 0
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
