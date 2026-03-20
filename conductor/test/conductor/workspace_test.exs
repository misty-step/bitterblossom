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

  describe "cleanup_conflicting_branch/4" do
    test "quotes derived paths and only deletes conductor factory branches" do
      parent = self()

      exec_fn = fn _sprite, command, _opts ->
        send(parent, {:cleanup_command, command})
        {:ok, ""}
      end

      assert :ok =
               Workspace.cleanup_conflicting_branch(
                 "bb-weaver",
                 "misty-step/bitterblossom",
                 "factory/42-1773867376",
                 exec_fn: exec_fn
               )

      assert_received {:cleanup_command, command}
      assert command =~ "cd '/home/sprite/workspace/bitterblossom'"
      assert command =~ "target_branch='factory/42-1773867376'"
      assert command =~ "conductor_root='/home/sprite/workspace/bitterblossom/.bb/conductor'"
      assert command =~ "git branch -D \"$target_branch\""
    end

    test "does not delete non-factory branches during conflict cleanup" do
      parent = self()

      exec_fn = fn _sprite, command, _opts ->
        send(parent, {:cleanup_command, command})
        {:ok, ""}
      end

      assert :ok =
               Workspace.cleanup_conflicting_branch(
                 "bb-weaver",
                 "misty-step/bitterblossom",
                 "feature/42-1773867376",
                 exec_fn: exec_fn
               )

      assert_received {:cleanup_command, command}
      refute command =~ "git branch -D \"$target_branch\""
    end
  end

  describe "sync_persona/4" do
    test "materializes a role-specific launch dir from deterministic local persona sources" do
      workspace =
        Path.join(System.tmp_dir!(), "workspace-test-#{System.unique_integer([:positive])}")

      source_root =
        Path.join(System.tmp_dir!(), "persona-source-#{System.unique_integer([:positive])}")

      File.mkdir_p!(workspace)

      on_exit(fn ->
        File.rm_rf(workspace)
        File.rm_rf(source_root)
      end)

      write_workspace_file(source_root, "shared/CLAUDE.md", "shared claude\n")
      write_workspace_file(source_root, "shared/AGENTS.md", "shared agents\n")

      write_workspace_file(
        source_root,
        "shared/skills/gather-pr-context/SKILL.md",
        "shared skill\n"
      )

      write_workspace_file(source_root, "thorn/CLAUDE.md", "thorn claude\n")
      write_workspace_file(source_root, "thorn/AGENTS.md", "thorn agents\n")

      write_workspace_file(
        source_root,
        "thorn/skills/diagnose-ci/SKILL.md",
        "thorn skill\n"
      )

      write_workspace_file(
        workspace,
        ".claude/skills/bb-persona-thorn-stale/SKILL.md",
        "stale claude skill\n"
      )

      write_workspace_file(
        workspace,
        ".agents/skills/bb-persona-thorn-stale/SKILL.md",
        "stale agents skill\n"
      )

      write_workspace_file(
        workspace,
        ".claude/skills/bb-persona-fern-keep/SKILL.md",
        "fern claude keep\n"
      )

      write_workspace_file(
        workspace,
        ".agents/skills/bb-persona-fern-keep/SKILL.md",
        "fern agents keep\n"
      )

      assert :ok =
               Workspace.sync_persona("local", workspace, :thorn,
                 exec_fn: &local_exec/3,
                 source_root: source_root
               )

      launch_dir = Workspace.persona_launch_dir(workspace, :thorn)

      assert File.read!(Path.join(launch_dir, "CLAUDE.md")) == "shared claude\nthorn claude\n"
      assert File.read!(Path.join(launch_dir, "AGENTS.md")) == "shared agents\nthorn agents\n"

      assert File.read!(Path.join(launch_dir, ".claude/skills/gather-pr-context/SKILL.md")) ==
               "shared skill\n"

      assert File.read!(Path.join(launch_dir, ".claude/skills/diagnose-ci/SKILL.md")) ==
               "thorn skill\n"

      assert File.read!(Path.join(launch_dir, ".agents/skills/gather-pr-context/SKILL.md")) ==
               "shared skill\n"

      assert File.read!(Path.join(launch_dir, ".agents/skills/diagnose-ci/SKILL.md")) ==
               "thorn skill\n"

      assert File.read!(Path.join(workspace, ".claude/CLAUDE.md")) ==
               "shared claude\nthorn claude\n"

      assert File.read!(
               Path.join(workspace, ".claude/skills/bb-persona-thorn-diagnose-ci/SKILL.md")
             ) ==
               "thorn skill\n"

      assert File.read!(
               Path.join(workspace, ".agents/skills/bb-persona-thorn-gather-pr-context/SKILL.md")
             ) ==
               "shared skill\n"

      refute File.exists?(Path.join(workspace, ".claude/skills/bb-persona-thorn-stale"))
      refute File.exists?(Path.join(workspace, ".agents/skills/bb-persona-thorn-stale"))
      assert File.exists?(Path.join(workspace, ".claude/skills/bb-persona-fern-keep"))
      assert File.exists?(Path.join(workspace, ".agents/skills/bb-persona-fern-keep"))
    end

    test "rejects unknown persona roles" do
      assert {:error, :invalid_role} =
               Workspace.sync_persona("local", "/tmp/ws", :unknown, exec_fn: &local_exec/3)
    end

    test "returns missing persona source errors before consulting config when source_root is provided" do
      source_root =
        Path.join(System.tmp_dir!(), "persona-source-#{System.unique_integer([:positive])}")

      File.mkdir_p!(source_root)
      Application.delete_env(:conductor, :persona_source_root)

      on_exit(fn -> File.rm_rf(source_root) end)

      assert {:error, message} =
               Workspace.sync_persona("local", "/tmp/ws", :thorn,
                 exec_fn: &local_exec/3,
                 source_root: source_root
               )

      assert message =~ "missing persona source"
      assert message =~ Path.join(source_root, "shared/CLAUDE.md")
    end

    test "propagates prepare command failures" do
      workspace =
        Path.join(System.tmp_dir!(), "workspace-test-#{System.unique_integer([:positive])}")

      source_root = minimal_persona_source_root(:thorn)
      File.mkdir_p!(workspace)

      on_exit(fn ->
        File.rm_rf(workspace)
        File.rm_rf(source_root)
      end)

      assert {:error, "persona sync failed (73): permission denied"} =
               Workspace.sync_persona("local", workspace, :thorn,
                 source_root: source_root,
                 exec_fn: fn _sprite, command, _opts ->
                   if String.contains?(command, "mkdir -p"),
                     do: {:error, "permission denied", 73},
                     else: {:ok, ""}
                 end
               )
    end

    test "propagates upload failures" do
      workspace =
        Path.join(System.tmp_dir!(), "workspace-test-#{System.unique_integer([:positive])}")

      source_root = minimal_persona_source_root(:thorn)
      File.mkdir_p!(workspace)

      on_exit(fn ->
        File.rm_rf(workspace)
        File.rm_rf(source_root)
      end)

      assert {:error, "persona sync failed (75): upload failed"} =
               Workspace.sync_persona("local", workspace, :thorn,
                 source_root: source_root,
                 exec_fn: fn _sprite, command, opts ->
                   if command == "true" and Keyword.has_key?(opts, :files),
                     do: {:error, "upload failed", 75},
                     else: {:ok, ""}
                 end
               )
    end

    test "propagates link command failures" do
      workspace =
        Path.join(System.tmp_dir!(), "workspace-test-#{System.unique_integer([:positive])}")

      source_root = minimal_persona_source_root(:thorn)
      File.mkdir_p!(workspace)

      on_exit(fn ->
        File.rm_rf(workspace)
        File.rm_rf(source_root)
      end)

      assert {:error, "persona sync failed (76): link failed"} =
               Workspace.sync_persona("local", workspace, :thorn,
                 source_root: source_root,
                 exec_fn: fn _sprite, command, _opts ->
                   if String.contains?(command, "ln -s ../.agents/skills"),
                     do: {:error, "link failed", 76},
                     else: {:ok, ""}
                 end
               )
    end
  end

  defp write_workspace_file(workspace, relative_path, contents) do
    path = Path.join(workspace, relative_path)
    File.mkdir_p!(Path.dirname(path))
    File.write!(path, contents)
  end

  defp minimal_persona_source_root(role) do
    source_root =
      Path.join(System.tmp_dir!(), "persona-source-#{System.unique_integer([:positive])}")

    role_name = Atom.to_string(role)

    write_workspace_file(source_root, "shared/CLAUDE.md", "shared claude\n")
    write_workspace_file(source_root, "shared/AGENTS.md", "shared agents\n")
    write_workspace_file(source_root, "#{role_name}/CLAUDE.md", "#{role_name} claude\n")
    write_workspace_file(source_root, "#{role_name}/AGENTS.md", "#{role_name} agents\n")

    source_root
  end

  defp local_exec(_sprite, command, opts) do
    for {source, destination} <- Keyword.get(opts, :files, []) do
      File.mkdir_p!(Path.dirname(destination))
      File.cp!(source, destination)
    end

    case System.cmd("bash", ["-lc", command], stderr_to_stdout: true) do
      {output, 0} -> {:ok, output}
      {output, code} -> {:error, output, code}
    end
  end
end
