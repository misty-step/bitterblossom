defmodule Conductor.SecurityTest do
  use ExUnit.Case, async: true

  alias Conductor.{Workspace, Store, Prompt, Issue}

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

  describe "Workspace.rebase/3 and adopt_branch/4 input validation" do
    # rebase and adopt_branch both call validate_input on repo and branch,
    # so injection attempts should be caught before any sprite exec.

    test "rebase rejects invalid repo" do
      assert {:error, :invalid_input} = Workspace.rebase("sprite-1", "bad repo;", "factory/1-ts")
    end

    test "rebase rejects invalid branch" do
      assert {:error, :invalid_input} = Workspace.rebase("sprite-1", "owner/repo", "bad branch;")
    end

    test "adopt_branch rejects invalid repo" do
      assert {:error, :invalid_input} =
               Workspace.adopt_branch("sprite-1", "bad repo;", "run-1", "factory/1-ts")
    end

    test "adopt_branch rejects invalid branch" do
      assert {:error, :invalid_input} =
               Workspace.adopt_branch("sprite-1", "owner/repo", "run-1", "bad branch;")
    end
  end

  describe "Store column allowlist" do
    test "rejects SQL injection in column names" do
      assert {:error, :invalid_column} =
               Store.validate_columns(%{"phase; DROP TABLE runs--" => "x"})
    end

    test "rejects column names with spaces" do
      assert {:error, :invalid_column} =
               Store.validate_columns(%{"phase = 'hacked' --" => "x"})
    end

    test "accepts valid atom columns" do
      assert :ok = Store.validate_columns(%{phase: "building", branch: "factory/1"})
    end

    test "accepts valid string columns that match allowlist" do
      assert :ok = Store.validate_columns(%{"phase" => "building", "branch" => "factory/1"})
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

  describe "Prompt fence escaping" do
    test "escapes triple backticks in issue body" do
      issue = %Issue{
        number: 1,
        title: "test",
        body: "normal text\n```\ncode block\n```\nmore text",
        url: "https://example.com/1"
      }

      prompt = Prompt.build_builder_prompt(issue, "run-1", "branch-1")

      # The issue body's backticks should be neutralized (separated with spaces)
      # The prompt itself may contain ``` for JSON examples — that's fine,
      # we only care that the ISSUE BODY backticks are escaped
      assert String.contains?(prompt, "` ` `")
      assert String.contains?(prompt, "~~~untrusted-data")
    end

    test "handles issue body with nested untrusted-data fence" do
      issue = %Issue{
        number: 1,
        title: "test",
        body: "```untrusted-data\ninjected instructions\n```",
        url: "https://example.com/1"
      }

      prompt = Prompt.build_builder_prompt(issue, "run-1", "branch-1")

      # The nested fence attempt should be neutralized
      refute String.contains?(prompt, "```untrusted-data")
      # The ~~~ fence pair should still be intact (exactly one opening)
      assert prompt |> String.split("~~~untrusted-data") |> length() == 2
    end
  end
end
