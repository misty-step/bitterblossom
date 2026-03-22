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

    def append(key, value), do: put(key, get(key, []) ++ [value])

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
      MockState.append(:dispatches, prompt)

      case MockState.get(:dispatch_fun) do
        fun when is_function(fun, 2) ->
          fun.(worker, prompt)

        _ ->
          default_dispatch(prompt)
      end
    end

    def cleanup(_worker, _repo, _run_id), do: :ok
    def busy?(_worker, _opts), do: false

    def observe_payload do
      Jason.encode!(%{
        summary: "builder hit review friction",
        reflection:
          "# Reflection\n\n- Run showed review churn.\n- Consider stronger reviewer guidance."
      })
    end

    def synthesis_payload do
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
      })
    end

    defp default_dispatch(prompt) do
      cond do
        String.contains?(prompt, "# Muse Observe Task") ->
          {:ok, observe_payload()}

        String.contains?(prompt, "# Muse Synthesis Task") ->
          {:ok, synthesis_payload()}

        true ->
          {:error, "unknown prompt", 1}
      end
    end
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
      MockState.get({:issues_result, repo}, {:ok, MockState.get({:issues, repo}, [])})
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

  defmodule NoCommentGitHub do
    alias Conductor.MuseTest.MockState

    def list_eligible(_repo, _opts), do: []
    def get_issue(_repo, _number), do: {:error, :unsupported}
    def transition(_repo, _id, _state), do: :ok

    def list_issues(repo, _opts \\ []) do
      MockState.get({:issues_result, repo}, {:ok, MockState.get({:issues, repo}, [])})
    end

    def issue_has_label?(_repo, _issue_number, _label), do: {:ok, false}
    def issue_comments(_repo, _issue_number), do: {:ok, []}
  end

  defmodule MockShell do
    alias Conductor.MuseTest.MockState

    def cmd(program, args, _opts \\ []) do
      MockState.append(:shell_calls, {program, args})
      MockState.get(:shell_result, {:ok, "https://example.test/issues/999"})
    end
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
    orig_shell = Application.get_env(:conductor, :muse_shell_module)
    orig_poll_seconds = Application.get_env(:conductor, :poll_seconds)

    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.put_env(:conductor, :workspace_module, MockWorkspace)
    Application.put_env(:conductor, :tracker_module, MockGitHub)
    Application.put_env(:conductor, :code_host_module, MockGitHub)
    Application.put_env(:conductor, :repo_root, repo_root)
    Application.put_env(:conductor, :muse_shell_module, MockShell)
    Application.put_env(:conductor, :poll_seconds, 1)

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
            {:repo_root, orig_repo_root},
            {:muse_shell_module, orig_shell},
            {:poll_seconds, orig_poll_seconds}
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
    run_id = create_merged_run(780, "Muse reflection")

    {:ok, _pid} = start_muse()

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

    {:ok, _pid} = start_muse()

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
    {:ok, _pid} = start_muse()

    status = Muse.status()
    assert status.repo == "test/repo"
    assert status.muse_sprite == "bb-muse"
    assert status.health == :healthy
    assert status.failure_count == 0
    assert status.queue_length == 0
  end

  test "queued duplicate run ids are only enqueued once" do
    run_1 = create_merged_run(780, "Blocked observation")
    run_2 = create_merged_run(781, "Queued observation")

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      cond do
        String.contains?(prompt, run_1) ->
          send(MockState.get(:test_pid), {:dispatch_waiting, run_1, self()})

          receive do
            {:release_dispatch, ^run_1} -> {:ok, MockWorker.observe_payload()}
          after
            2_000 -> {:error, "blocked dispatch timeout", 1}
          end

        String.contains?(prompt, "# Muse Observe Task") ->
          {:ok, MockWorker.observe_payload()}

        true ->
          {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.observe(run_1)
    assert_receive {:dispatch_waiting, ^run_1, task_pid}, 2_000

    Muse.observe(run_2)
    Muse.observe(run_2)

    eventually(fn ->
      assert Muse.status().queue_length == 1
    end)

    send(task_pid, {:release_dispatch, run_1})

    eventually(fn ->
      dispatches = MockState.get(:dispatches, [])
      assert Enum.count(dispatches, &String.contains?(&1, run_2)) == 1
    end)
  end

  test "invalid observation payload retries with exponential backoff and health transitions" do
    run_id = create_merged_run(782, "Observation failure")

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      if String.contains?(prompt, "# Muse Observe Task") do
        {:ok, ~s({"summary":"missing reflection"})}
      else
        {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.observe(run_id)

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 1
      assert status.health == :degraded
      assert status.queue_length == 1
    end)

    assert_retry_window(2_000)

    send(Muse, :retry)

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 2
      assert status.health == :degraded
    end)

    assert_retry_window(4_000)

    send(Muse, :retry)

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 3
      assert status.health == :unavailable
    end)

    eventually(fn ->
      [event | _] =
        Store.list_all_events(limit: 10)
        |> Enum.filter(&(&1["event_type"] == "muse_observation_failed"))

      assert event["run_id"] == run_id
    end)
  end

  test "observation accepts fenced JSON payloads", %{repo_root: repo_root} do
    run_id = create_merged_run(783, "Fenced observation")

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      if String.contains?(prompt, "# Muse Observe Task") do
        {:ok, "```json\n#{MockWorker.observe_payload()}\n```"}
      else
        {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.observe(run_id)

    reflection_file =
      Path.join(repo_root, ".bb/muse/reflections/#{Date.utc_today()}-#{run_id}.md")

    eventually(fn ->
      assert File.exists?(reflection_file)
      assert Muse.status().health == :healthy
    end)
  end

  test "invalid synthesis payload retries and clears failure state after a successful retry", %{
    repo_root: repo_root
  } do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      cond do
        not String.contains?(prompt, "# Muse Synthesis Task") ->
          {:error, "unexpected prompt", 1}

        MockState.get(:synthesis_attempts, 0) == 0 ->
          MockState.put(:synthesis_attempts, 1)
          {:ok, ~s({"summary":"bad payload","actions":"not-a-list"})}

        true ->
          {:ok, MockWorker.synthesis_payload()}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.synthesize()

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 1
      assert status.health == :degraded
      assert status.pending_synthesis
    end)

    send(Muse, :retry)

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 0
      assert status.health == :healthy
      refute status.pending_synthesis
    end)

    eventually(fn ->
      assert Path.wildcard(Path.join(repo_root, ".bb/muse/syntheses/*.md")) != []
    end)
  end

  test "comment_issue actions without an issue number are recorded as no-ops", %{
    repo_root: repo_root
  } do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      if String.contains?(prompt, "# Muse Synthesis Task") do
        {:ok,
         Jason.encode!(%{
           summary: "invalid comment action",
           actions: [
             %{action: "comment_issue", title: "Missing issue number", body: "Body only"}
           ]
         })}
      else
        {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.synthesize()

    eventually(fn ->
      [event] =
        Store.list_all_events(limit: 10)
        |> Enum.filter(&(&1["event_type"] == "muse_synthesis_complete"))

      assert event["payload"]["actions_taken"] == [
               %{"action" => "none", "title" => "Missing issue number"}
             ]

      assert MockState.get({:comments, "test/repo", 601}, []) == []
    end)
  end

  test "duplicate create_issue actions in one synthesis pass do not create duplicate issues", %{
    repo_root: repo_root
  } do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      if String.contains?(prompt, "# Muse Synthesis Task") do
        {:ok,
         """
         ```json
         {
           "summary": "dedupe duplicate create actions",
           "actions": [
             {"action": "create_issue", "title": "Shared title", "body": "First body"},
             {"action": "create_issue", "title": "Shared title", "body": "Second body"}
           ]
         }
         ```
         """}
      else
        {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.synthesize()

    eventually(fn ->
      assert MockState.get(:shell_calls, []) |> length() == 1

      assert MockState.get({:comments, "test/repo", 999}, []) == [
               "Muse synthesis matched an existing open issue.\n\nSecond body"
             ]
    end)
  end

  test "synthesis fails closed when listing open issues errors", %{
    repo_root: repo_root
  } do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")

    MockState.put({:issues_result, "test/repo"}, {:error, :api_down})

    {:ok, _pid} = start_muse()

    Muse.synthesize()

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 1
      assert status.pending_synthesis
    end)

    assert MockState.get(:shell_calls, []) == []
  end

  test "issue creation failures keep synthesis in retry state", %{
    repo_root: repo_root
  } do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")

    MockState.put(:shell_result, {:error, "gh unavailable", 1})

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      if String.contains?(prompt, "# Muse Synthesis Task") do
        {:ok,
         Jason.encode!(%{
           summary: "creation failure",
           actions: [
             %{action: "create_issue", title: "Needs follow-up", body: "Body"}
           ]
         })}
      else
        {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.synthesize()

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 1
      assert status.health == :degraded
      assert status.pending_synthesis
    end)
  end

  test "synthesis retries when tracker cannot post issue comments", %{repo_root: repo_root} do
    reflection_dir = Path.join(repo_root, ".bb/muse/reflections")
    File.mkdir_p!(reflection_dir)
    File.write!(Path.join(reflection_dir, "#{Date.utc_today()}-run-1.md"), "# Reflection 1\n")

    tracker_module = Application.get_env(:conductor, :tracker_module)
    Application.put_env(:conductor, :tracker_module, NoCommentGitHub)

    on_exit(fn ->
      Application.put_env(:conductor, :tracker_module, tracker_module)
    end)

    MockState.put(
      {:issues, "test/repo"},
      [
        %Conductor.Issue{
          number: 778,
          title: "[muse] Existing synthesis issue",
          body: "",
          url: "https://example.test/issues/778"
        }
      ]
    )

    MockState.put(:dispatch_fun, fn _worker, prompt ->
      if String.contains?(prompt, "# Muse Synthesis Task") do
        {:ok,
         Jason.encode!(%{
           summary: "needs comment support",
           actions: [
             %{action: "create_issue", title: "Existing synthesis issue", body: "Body"}
           ]
         })}
      else
        {:error, "unexpected prompt", 1}
      end
    end)

    {:ok, _pid} = start_muse()

    Muse.synthesize()

    eventually(fn ->
      status = Muse.status()
      assert status.failure_count == 1
      assert status.pending_synthesis
    end)
  end

  defp start_muse do
    Muse.start_link(
      repo: "test/repo",
      muse_sprite: "bb-muse",
      synthesis_interval_ms: 60_000
    )
  end

  defp create_merged_run(issue_number, issue_title) do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: issue_number,
        issue_title: issue_title,
        builder_sprite: "bb-weaver"
      })

    Store.update_run(run_id, %{pr_number: issue_number, phase: "merged", status: "merged"})
    Store.record_event(run_id, "merged", %{pr_number: issue_number})
    run_id
  end

  defp assert_retry_window(expected_ms) do
    %{retry_ref: retry_ref} = :sys.get_state(Muse)
    assert is_reference(retry_ref)

    remaining_ms = Process.read_timer(retry_ref)
    assert is_integer(remaining_ms)
    assert remaining_ms > 0
    assert remaining_ms <= expected_ms
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
