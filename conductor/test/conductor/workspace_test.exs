defmodule Conductor.WorkspaceTest do
  use ExUnit.Case, async: true

  alias Conductor.Workspace

  describe "prepare/5" do
    test "installs a branch guard hook for the prepared branch" do
      parent = self()

      exec_fn = fn _sprite, command, _opts ->
        send(parent, {:prepare_command, command})
        {:ok, "/tmp/test-worktree\n"}
      end

      assert {:ok, "/tmp/test-worktree"} =
               Workspace.prepare(
                 "bb-weaver",
                 "misty-step/bitterblossom",
                 "run-42-1773867376",
                 "factory/42-1773867376",
                 exec_fn: exec_fn
               )

      assert_received {:prepare_command, command}
      assert command =~ "config extensions.worktreeConfig true"
      assert command =~ "config --worktree core.hooksPath .bb-hooks"
      assert command =~ "hook_path=\"$hook_dir/pre-push\""
      assert command =~ "expected_branch=\"factory/42-1773867376\""
      assert command =~ "refs/heads/$expected_branch"
      assert command =~ "refusing push from"
    end
  end

  describe "adopt_branch/5" do
    test "installs the same branch guard when adopting an existing branch" do
      parent = self()

      exec_fn = fn _sprite, command, _opts ->
        send(parent, {:adopt_command, command})
        {:ok, "/tmp/test-worktree\n"}
      end

      assert {:ok, "/tmp/test-worktree"} =
               Workspace.adopt_branch(
                 "bb-weaver",
                 "misty-step/bitterblossom",
                 "run-42-1773867376",
                 "factory/42-1773867376",
                 exec_fn: exec_fn
               )

      assert_received {:adopt_command, command}
      assert command =~ "config --worktree core.hooksPath .bb-hooks"
      assert command =~ "expected_branch=\"factory/42-1773867376\""
    end
  end
end
