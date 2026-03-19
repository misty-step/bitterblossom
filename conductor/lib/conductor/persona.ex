defmodule Conductor.Persona do
  @moduledoc """
  Role-specific agent context staged into a dispatch workspace.

  Shared instructions live under `sprites/shared/`. Role overlays live under
  `sprites/<role>/`. At dispatch time the conductor combines the shared and
  role-specific root instructions into workspace-local `CLAUDE.md` and
  `AGENTS.md`, then mirrors skills into both `.claude/skills/` and
  `.codex/skills/` so either harness can discover the same guidance surface.
  """

  @valid_role ~r/^[a-z0-9_-]+$/

  @type upload :: {binary(), binary()}
  @type manifest :: %{uploads: [upload()], directories: [binary()]}

  @spec manifest(binary(), atom() | binary() | nil) :: {:ok, manifest()} | {:error, term()}
  def manifest(_workspace, nil), do: {:ok, %{uploads: [], directories: []}}

  def manifest(workspace, role) do
    with {:ok, role} <- normalize_role(role),
         {:ok, uploads} <- uploads(workspace, role) do
      {:ok,
       %{
         uploads: uploads,
         directories:
           uploads
           |> Enum.map(&Path.dirname(elem(&1, 0)))
           |> Enum.uniq()
           |> Enum.sort()
       }}
    end
  end

  defp normalize_role(role) when is_atom(role), do: normalize_role(Atom.to_string(role))

  defp normalize_role(role) when is_binary(role) do
    if Regex.match?(@valid_role, role), do: {:ok, role}, else: {:error, :invalid_role}
  end

  defp normalize_role(_), do: {:error, :invalid_role}

  defp uploads(workspace, role) do
    root = Conductor.Config.sprites_root()
    shared_root = Path.join(root, "sprites/shared")
    role_root = Path.join(root, "sprites/#{role}")

    with :ok <- ensure_required_skills(shared_root, role_root, role),
         {:ok, root_uploads} <- combined_root_uploads(workspace, shared_root, role_root),
         {:ok, shared_skill_uploads} <- skill_uploads(workspace, shared_root),
         {:ok, role_skill_uploads} <- skill_uploads(workspace, role_root) do
      {:ok, root_uploads ++ shared_skill_uploads ++ role_skill_uploads}
    end
  end

  defp combined_root_uploads(workspace, shared_root, role_root) do
    with {:ok, claude} <- combined_root_file(shared_root, role_root, "CLAUDE.md"),
         {:ok, agents} <- combined_root_file(shared_root, role_root, "AGENTS.md") do
      {:ok,
       [
         {Path.join(workspace, "CLAUDE.md"), claude},
         {Path.join(workspace, "AGENTS.md"), agents}
       ]}
    end
  end

  defp combined_root_file(shared_root, role_root, name) do
    [Path.join(shared_root, name), Path.join(role_root, name)]
    |> read_files()
    |> case do
      {:ok, contents} -> {:ok, contents |> Enum.join("\n\n") |> Kernel.<>("\n")}
      {:error, _} = error -> error
    end
  end

  defp ensure_required_skills(shared_root, role_root, "thorn") do
    [
      Path.join(shared_root, "skills/gather-pr-context/SKILL.md"),
      Path.join(shared_root, "skills/verify-invariants/SKILL.md"),
      Path.join(role_root, "skills/diagnose-ci/SKILL.md"),
      Path.join(role_root, "skills/plan-fix/SKILL.md")
    ]
    |> Enum.reduce_while(:ok, fn path, :ok ->
      case File.read(path) do
        {:ok, _body} -> {:cont, :ok}
        {:error, _reason} -> {:halt, {:error, {:missing_persona_file, path}}}
      end
    end)
  end

  defp ensure_required_skills(_shared_root, _role_root, _role), do: :ok

  defp skill_uploads(workspace, root) do
    root
    |> Path.join("skills")
    |> Path.join("**/*")
    |> Path.wildcard(match_dot: false)
    |> Enum.reject(&File.dir?/1)
    |> Enum.reject(&artifact_file?/1)
    |> Enum.reduce_while({:ok, []}, fn path, {:ok, acc} ->
      rel = Path.relative_to(path, Path.join(root, "skills"))

      case File.read(path) do
        {:ok, body} ->
          uploads = [
            {Path.join([workspace, ".claude", "skills", rel]), body},
            {Path.join([workspace, ".codex", "skills", rel]), body}
          ]

          {:cont, {:ok, Enum.reverse(uploads, acc)}}

        {:error, _reason} ->
          {:halt, {:error, {:missing_persona_file, path}}}
      end
    end)
    |> case do
      {:ok, uploads} -> {:ok, Enum.reverse(uploads)}
      {:error, _} = error -> error
    end
  end

  defp read_files(paths) do
    Enum.reduce_while(paths, {:ok, []}, fn path, {:ok, acc} ->
      case File.read(path) do
        {:ok, body} -> {:cont, {:ok, [body | acc]}}
        {:error, _reason} -> {:halt, {:error, {:missing_persona_file, path}}}
      end
    end)
    |> case do
      {:ok, contents} -> {:ok, Enum.reverse(contents)}
      {:error, _} = error -> error
    end
  end

  defp artifact_file?(path) do
    name = Path.basename(path)

    String.starts_with?(name, ".") or String.ends_with?(name, "~") or
      String.ends_with?(name, ".swp") or String.ends_with?(name, ".tmp")
  end
end
