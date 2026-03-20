defmodule Conductor.CLIPauseTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{CLI, Store}

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "pause_cli_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "pause_cli_test_#{System.unique_integer([:positive])}.jsonl")

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)

    Application.stop(:conductor)
    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.ensure_all_started(:conductor)

    on_exit(fn ->
      Application.stop(:conductor)
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)
      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "pause and resume commands toggle dispatch state" do
    pause_output =
      capture_io(fn ->
        CLI.main(["pause"])
      end)

    assert pause_output =~ "paused"
    assert Store.dispatch_paused?()

    resume_output =
      capture_io(fn ->
        CLI.main(["resume"])
      end)

    assert resume_output =~ "resumed"
    refute Store.dispatch_paused?()
  end
end
