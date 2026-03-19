defmodule Conductor.PersonaTest do
  use ExUnit.Case, async: false

  alias Conductor.Persona

  setup do
    root = Path.join(System.tmp_dir!(), "persona_test_#{System.unique_integer([:positive])}")
    previous_root = Application.get_env(:conductor, :sprites_root)

    File.rm_rf(root)
    File.mkdir_p!(root)
    Application.put_env(:conductor, :sprites_root, root)

    on_exit(fn ->
      if previous_root do
        Application.put_env(:conductor, :sprites_root, previous_root)
      else
        Application.delete_env(:conductor, :sprites_root)
      end

      File.rm_rf(root)
    end)

    %{root: root, workspace: "/remote/workspace"}
  end

  test "manifest/2 returns an empty manifest for nil role", %{workspace: workspace} do
    assert {:ok, %{uploads: [], directories: []}} = Persona.manifest(workspace, nil)
  end

  test "manifest/2 rejects invalid roles", %{workspace: workspace} do
    assert {:error, :invalid_role} = Persona.manifest(workspace, "thorn!")
    assert {:error, :invalid_role} = Persona.manifest(workspace, 123)
  end

  test "manifest/2 combines shared and role files and filters artifact files", %{
    root: root,
    workspace: workspace
  } do
    create_persona_fixture(root)

    assert {:ok, manifest} = Persona.manifest(workspace, :thorn)

    uploads = Map.new(manifest.uploads)

    assert uploads[Path.join(workspace, "CLAUDE.md")] ==
             "# shared claude\n\n# thorn claude\n"

    assert uploads[Path.join(workspace, "AGENTS.md")] ==
             "# shared agents\n\n# thorn agents\n"

    assert uploads[Path.join([workspace, ".claude", "skills", "gather-pr-context", "SKILL.md"])] ==
             "shared gather"

    assert uploads[Path.join([workspace, ".codex", "skills", "plan-fix", "SKILL.md"])] ==
             "thorn plan"

    assert length(manifest.uploads) == 10
    assert manifest.directories == Enum.sort(manifest.directories)
    assert Path.join([workspace, ".claude", "skills", "diagnose-ci"]) in manifest.directories
    assert Path.join([workspace, ".codex", "skills", "verify-invariants"]) in manifest.directories

    refute Enum.any?(manifest.uploads, fn {dest, _body} ->
             String.contains?(dest, ".DS_Store") or String.ends_with?(dest, "~") or
               String.ends_with?(dest, ".swp") or String.ends_with?(dest, ".tmp")
           end)
  end

  test "manifest/2 fails when shared root instructions are missing", %{
    root: root,
    workspace: workspace
  } do
    create_root_file(root, "thorn", "CLAUDE.md", "# thorn claude")
    create_root_file(root, "shared", "AGENTS.md", "# shared agents")
    create_root_file(root, "thorn", "AGENTS.md", "# thorn agents")

    assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
    assert path == Path.join([root, "sprites", "shared", "CLAUDE.md"])
  end

  test "manifest/2 fails when role root instructions are missing", %{
    root: root,
    workspace: workspace
  } do
    create_root_file(root, "shared", "CLAUDE.md", "# shared claude")
    create_root_file(root, "shared", "AGENTS.md", "# shared agents")
    create_root_file(root, "thorn", "AGENTS.md", "# thorn agents")

    assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
    assert path == Path.join([root, "sprites", "thorn", "CLAUDE.md"])
  end

  test "manifest/2 surfaces unreadable skill files as missing persona files", %{
    root: root,
    workspace: workspace
  } do
    create_persona_fixture(root)

    broken_skill = Path.join([root, "sprites", "thorn", "skills", "diagnose-ci", "SKILL.md"])
    File.rm!(broken_skill)
    assert :ok = File.ln_s("missing-skill.md", broken_skill)

    assert {:error, {:missing_persona_file, path}} = Persona.manifest(workspace, :thorn)
    assert path == broken_skill
  end

  defp create_persona_fixture(root) do
    create_root_file(root, "shared", "CLAUDE.md", "# shared claude")
    create_root_file(root, "shared", "AGENTS.md", "# shared agents")
    create_root_file(root, "thorn", "CLAUDE.md", "# thorn claude")
    create_root_file(root, "thorn", "AGENTS.md", "# thorn agents")

    create_skill_file(root, "shared", "gather-pr-context", "SKILL.md", "shared gather")
    create_skill_file(root, "shared", "verify-invariants", "SKILL.md", "shared verify")
    create_skill_file(root, "thorn", "diagnose-ci", "SKILL.md", "thorn diagnose")
    create_skill_file(root, "thorn", "plan-fix", "SKILL.md", "thorn plan")

    create_skill_file(root, "shared", "gather-pr-context", ".DS_Store", "ignored")
    create_skill_file(root, "shared", "gather-pr-context", "backup~", "ignored")
    create_skill_file(root, "shared", "gather-pr-context", "scratch.swp", "ignored")
    create_skill_file(root, "shared", "gather-pr-context", "scratch.tmp", "ignored")
  end

  defp create_root_file(root, role, name, body) do
    write_file(Path.join([root, "sprites", role, name]), body)
  end

  defp create_skill_file(root, role, skill, name, body) do
    write_file(Path.join([root, "sprites", role, "skills", skill, name]), body)
  end

  defp write_file(path, body) do
    path |> Path.dirname() |> File.mkdir_p!()
    File.write!(path, body)
  end
end
