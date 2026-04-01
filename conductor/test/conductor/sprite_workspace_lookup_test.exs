defmodule Conductor.SpriteWorkspaceLookupTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite

  test "logs discovery searches nested worktrees when no workspace is provided" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "shopt -s globstar nullglob") ->
          assert command =~ "/home/sprite/workspace/**/.bb/workspace.json"
          assert command =~ "/home/sprite/workspace/**/PROMPT.md"
          assert command =~ "/home/sprite/workspace/**/ralph.log"
          refute command =~ "/home/sprite/workspace/**/.git"
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

  test "logs discovery fails when no workspace markers exist" do
    exec_fn = fn _sprite, command, _opts ->
      if String.contains?(command, "shopt -s globstar nullglob"),
        do: {:ok, "\n"},
        else: {:error, "", 1}
    end

    assert {:error, reason} = Sprite.logs("bb-weaver", exec_fn: exec_fn)
    assert reason =~ ~s(sprite "bb-weaver" has no workspace repo)
  end
end
