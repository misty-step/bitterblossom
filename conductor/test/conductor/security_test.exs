defmodule Conductor.SecurityTest do
  use ExUnit.Case, async: true

  alias Conductor.Workspace

  describe "Workspace input validation" do
    test "rejects repo name with shell metacharacters" do
      assert {:error, :invalid_input} =
               Workspace.validate_input("owner/repo; rm -rf /")
    end

    test "rejects run_id with shell metacharacters" do
      assert {:error, :invalid_input} =
               Workspace.validate_input("run-1-123 && cat /etc/passwd")
    end

    test "rejects branch with shell metacharacters" do
      assert {:error, :invalid_input} =
               Workspace.validate_input("factory/1-123; echo pwned")
    end

    test "accepts valid repo name" do
      assert :ok = Workspace.validate_input("misty-step/bitterblossom")
    end

    test "accepts valid run_id" do
      assert :ok = Workspace.validate_input("run-625-1773502623")
    end

    test "accepts valid branch with slashes and dots" do
      assert :ok = Workspace.validate_input("factory/625-1773502623")
    end

    test "rejects path traversal" do
      assert {:error, :invalid_input} = Workspace.validate_input("../../etc/passwd")
    end

    test "rejects absolute path traversal" do
      assert {:error, :invalid_input} = Workspace.validate_input("foo/../../../tmp")
    end

    test "rejects absolute paths (leading slash)" do
      assert {:error, :invalid_input} = Workspace.validate_input("/etc/passwd")
    end

    test "rejects leading dash (git argument injection)" do
      assert {:error, :invalid_input} = Workspace.validate_input("--force")
    end
  end

  describe "governance: sprite merge lockout" do
    test "Sprite.kill_and_revoke/2 kills agents and revokes gh auth" do
      commands_run = :ets.new(:kill_revoke_cmds, [:bag, :public])

      exec_fn = fn _sprite, command, _opts ->
        :ets.insert(commands_run, {:cmd, command})
        {:ok, ""}
      end

      Conductor.Sprite.kill_and_revoke("bb-weaver", exec_fn: exec_fn)

      cmds = :ets.tab2list(commands_run) |> Enum.map(fn {:cmd, c} -> c end)
      assert Enum.any?(cmds, &String.contains?(&1, "pkill"))
      assert Enum.any?(cmds, &String.contains?(&1, "gh auth logout"))
      :ets.delete(commands_run)
    end

    test "Sprite.kill_and_revoke/2 tolerates exec failures" do
      exec_fn = fn _sprite, _command, _opts -> {:error, "unreachable", 1} end
      assert :ok = Conductor.Sprite.kill_and_revoke("bb-weaver", exec_fn: exec_fn)
    end
  end
end
