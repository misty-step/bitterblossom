defmodule Conductor.WorkspaceTest do
  use ExUnit.Case, async: true

  alias Conductor.Workspace

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
  end

  defp write_workspace_file(workspace, relative_path, contents) do
    path = Path.join(workspace, relative_path)
    File.mkdir_p!(Path.dirname(path))
    File.write!(path, contents)
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
