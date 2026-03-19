defmodule Conductor.PersonaTest do
  use ExUnit.Case, async: false

  alias Conductor.Persona

  setup do
    root = Path.join(System.tmp_dir!(), "persona_test_#{System.unique_integer([:positive])}")
    workspace = Path.join(root, "workspace")

    File.rm_rf!(root)
    File.mkdir_p!(workspace)

    original_root = Application.get_env(:conductor, :sprites_root)
    Application.put_env(:conductor, :sprites_root, root)

    on_exit(fn ->
      if original_root do
        Application.put_env(:conductor, :sprites_root, original_root)
      else
        Application.delete_env(:conductor, :sprites_root)
      end

      File.rm_rf!(root)
    end)

    %{root: root, workspace: workspace}
  end

  describe "manifest/2" do
    test "returns an empty manifest when no role is provided" do
      assert {:ok, %{uploads: [], directories: []}} = Persona.manifest("/ws", nil)
    end

    test "rejects invalid roles" do
      assert {:error, :invalid_role} = Persona.manifest("/ws", "thorn;rm -rf /")
    end

    test "combines shared and role root files and uploads skills", %{
      root: root,
      workspace: workspace
    } do
      write_persona_tree(root)

      assert {:ok, %{uploads: uploads, directories: directories}} =
               Persona.manifest(workspace, :thorn)

      assert {Path.join(workspace, "CLAUDE.md"), "shared claude\n\n\nthorn claude\n\n"} in uploads
      assert {Path.join(workspace, "AGENTS.md"), "shared agents\n\n\nthorn agents\n\n"} in uploads

      assert {Path.join(workspace, ".claude/skills/gather-pr-context/SKILL.md"), "gather"} in uploads

      assert {Path.join(workspace, ".codex/skills/gather-pr-context/SKILL.md"), "gather"} in uploads

      assert {Path.join(workspace, ".claude/skills/diagnose-ci/SKILL.md"), "diagnose"} in uploads
      assert {Path.join(workspace, ".codex/skills/plan-fix/SKILL.md"), "plan"} in uploads
      refute Enum.any?(uploads, fn {dest, _} -> String.ends_with?(dest, ".tmp") end)

      expected_directories =
        [
          Path.join(workspace, ".claude/skills/diagnose-ci"),
          Path.join(workspace, ".claude/skills/gather-pr-context"),
          Path.join(workspace, ".claude/skills/plan-fix"),
          Path.join(workspace, ".claude/skills/verify-invariants"),
          Path.join(workspace, ".codex/skills/diagnose-ci"),
          Path.join(workspace, ".codex/skills/gather-pr-context"),
          Path.join(workspace, ".codex/skills/plan-fix"),
          Path.join(workspace, ".codex/skills/verify-invariants"),
          workspace
        ]
        |> Enum.sort()

      assert MapSet.new(directories) == MapSet.new(expected_directories)
    end

    test "fails when the shared root file is missing", %{root: root, workspace: workspace} do
      write_persona_tree(root)
      File.rm!(Path.join(root, "sprites/shared/CLAUDE.md"))

      assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
      assert path == Path.join(root, "sprites/shared/CLAUDE.md")
    end

    test "fails when the role root file is missing", %{root: root, workspace: workspace} do
      write_persona_tree(root)
      File.rm!(Path.join(root, "sprites/thorn/AGENTS.md"))

      assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
      assert path == Path.join(root, "sprites/thorn/AGENTS.md")
    end

    test "fails closed when a required Thorn skill is missing", %{
      root: root,
      workspace: workspace
    } do
      write_persona_tree(root)
      File.rm!(Path.join(root, "sprites/thorn/skills/plan-fix/SKILL.md"))

      assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
      assert path == Path.join(root, "sprites/thorn/skills/plan-fix/SKILL.md")
    end

    test "fails when a required skill file is unreadable", %{root: root, workspace: workspace} do
      write_persona_tree(root)
      unreadable = Path.join(root, "sprites/shared/skills/verify-invariants/SKILL.md")
      File.chmod!(unreadable, 0)

      try do
        assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
        assert path == unreadable
      after
        File.chmod!(unreadable, 0o644)
      end
    end
  end

  defp write_persona_tree(root) do
    files = [
      {"sprites/shared/CLAUDE.md", "shared claude\n"},
      {"sprites/shared/AGENTS.md", "shared agents\n"},
      {"sprites/shared/skills/gather-pr-context/SKILL.md", "gather"},
      {"sprites/shared/skills/verify-invariants/SKILL.md", "verify"},
      {"sprites/shared/skills/ignored.tmp", "ignore me"},
      {"sprites/thorn/CLAUDE.md", "thorn claude\n"},
      {"sprites/thorn/AGENTS.md", "thorn agents\n"},
      {"sprites/thorn/skills/diagnose-ci/SKILL.md", "diagnose"},
      {"sprites/thorn/skills/plan-fix/SKILL.md", "plan"}
    ]

    Enum.each(files, fn {path, body} ->
      abs = Path.join(root, path)
      File.mkdir_p!(Path.dirname(abs))
      File.write!(abs, body)
    end)
  end
end
