defmodule Conductor.SpriteWorkspaceLookupTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite

  test "logs uses an injected workspace lookup before remote discovery" do
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

    assert :ok =
             Sprite.logs("bb-weaver",
               exec_fn: exec_fn,
               runner_fn: runner_fn,
               workspace_lookup_fn: fn "bb-weaver" -> {:ok, "/tmp/store-worktree"} end
             )

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
