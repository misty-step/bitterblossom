defmodule Conductor.CLILogsTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{CLI, Store}

  @conductor_dir Path.expand("../..", __DIR__)

  defmodule MockWorker do
    def logs("bb-weaver", opts) do
      send(self(), {:logs_called, "bb-weaver", opts})
      :ok
    end
  end

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "cli_logs_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "cli_logs_test_#{System.unique_integer([:positive])}.jsonl")

    stop_conductor_app()
    stop_process(Store)
    {:ok, _pid} = Store.start_link(db_path: db_path, event_log: event_log)

    orig_worker = Application.get_env(:conductor, :worker_module)
    Application.put_env(:conductor, :worker_module, MockWorker)

    on_exit(fn ->
      stop_process(Store)

      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "logs help prints usage without exiting" do
    output =
      capture_io(fn ->
        CLI.main(["logs", "--help"])
      end)

    assert output =~ "usage: mix conductor logs <sprite>"
    assert output =~ "--follow"
    assert output =~ "--lines"
  end

  test "mix conductor logs rejects negative line counts" do
    {output, status} =
      System.cmd("mix", ["conductor", "logs", "bb-weaver", "--lines", "-1"],
        cd: @conductor_dir,
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "--lines must be >= 0"
  end

  test "logs passes the active builder worktree from Store into the worker" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "misty-step/bitterblossom",
        issue_number: 736,
        issue_title: "remove Go bb transport",
        builder_sprite: "bb-weaver"
      })

    :ok =
      Store.update_run(run_id, %{
        phase: "building",
        status: "building",
        worktree_path: "/tmp/store-worktree"
      })

    capture_io(fn ->
      CLI.main(["logs", "bb-weaver"])
    end)

    assert_received {:logs_called, "bb-weaver", opts}

    assert match?(
             lookup_fn when is_function(lookup_fn, 1),
             Keyword.get(opts, :workspace_lookup_fn)
           )

    assert {:ok, "/tmp/store-worktree"} = Keyword.fetch!(opts, :workspace_lookup_fn).("bb-weaver")
  end
end
