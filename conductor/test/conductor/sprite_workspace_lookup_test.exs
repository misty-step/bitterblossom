defmodule Conductor.SpriteWorkspaceLookupTest do
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{Sprite, Store}

  setup do
    stop_conductor_app()

    db_path =
      Path.join(
        System.tmp_dir!(),
        "sprite_workspace_lookup_#{System.unique_integer([:positive])}.db"
      )

    event_log =
      Path.join(
        System.tmp_dir!(),
        "sprite_workspace_lookup_#{System.unique_integer([:positive])}.jsonl"
      )

    stop_process(Store)
    {:ok, _pid} = Store.start_link(db_path: db_path, event_log: event_log)

    on_exit(fn ->
      stop_process(Store)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "logs uses the active worktree recorded in Store before remote discovery" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "misty-step/bitterblossom",
        issue_number: 736,
        issue_title: "remove Go bb transport",
        builder_sprite: "bb-weaver"
      })

    :ok =
      Store.update_run(run_id, %{
        status: "building",
        worktree_path: "/tmp/store-worktree"
      })

    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "meta=$(ls -dt") ->
          flunk("workspace discovery should not run when Store has an active worktree")

        String.contains?(command, "test -s '/tmp/store-worktree/ralph.log'") ->
          {:ok, ""}

        true ->
          {:error, "", 1}
      end
    end

    runner_fn = fn _sprite, command, _opts ->
      send(test_pid, {:runner_called, command})
      {:ok, ""}
    end

    assert :ok = Sprite.logs("bb-weaver", exec_fn: exec_fn, runner_fn: runner_fn)
    assert_received {:exec_called, "test -s '/tmp/store-worktree/ralph.log'"}

    assert_received {:runner_called,
                     "touch '/tmp/store-worktree/ralph.log' && cat '/tmp/store-worktree/ralph.log'"}
  end

  test "logs discovery searches nested worktrees when Store has no active run" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "shopt -s globstar nullglob") ->
          assert command =~ "/home/sprite/workspace/**/.bb/workspace.json"
          assert command =~ "/home/sprite/workspace/**/PROMPT.md"
          assert command =~ "/home/sprite/workspace/**/ralph.log"
          {:ok, "/tmp/repo/.bb/conductor/run-1/builder-worktree\n"}

        String.contains?(
          command,
          "test -s '/tmp/repo/.bb/conductor/run-1/builder-worktree/ralph.log'"
        ) ->
          {:ok, ""}

        true ->
          {:error, "", 1}
      end
    end

    runner_fn = fn _sprite, command, _opts ->
      send(test_pid, {:runner_called, command})
      {:ok, ""}
    end

    assert :ok = Sprite.logs("bb-weaver", exec_fn: exec_fn, runner_fn: runner_fn)

    assert_received {:runner_called,
                     "touch '/tmp/repo/.bb/conductor/run-1/builder-worktree/ralph.log' && cat '/tmp/repo/.bb/conductor/run-1/builder-worktree/ralph.log'"}
  end
end
