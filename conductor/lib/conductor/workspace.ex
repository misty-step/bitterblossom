defmodule Conductor.Workspace do
  @moduledoc """
  Persona management and workspace utilities for sprites.

  Handles syncing persona files (CLAUDE.md, AGENTS.md, skills) from local
  sprite definitions onto remote sprites, and provides workspace path resolution.
  """

  alias Conductor.{Config, Shell, Sprite}
  @mirror_base "/home/sprite/workspace"
  @safe_input ~r/^[a-zA-Z0-9_\-\.\/]+$/
  @repo_segment ~r/^[A-Za-z0-9_.-]+$/
  @persona_roles ~w(weaver thorn fern muse tansy)

  @doc "Validate that a string is safe for shell interpolation. Rejects metacharacters, path traversal, absolute paths, and leading dashes."
  @spec validate_input(binary()) :: :ok | {:error, :invalid_input}
  def validate_input(input) do
    cond do
      not Regex.match?(@safe_input, input) -> {:error, :invalid_input}
      String.contains?(input, "..") -> {:error, :invalid_input}
      String.starts_with?(input, "/") -> {:error, :invalid_input}
      String.starts_with?(input, "-") -> {:error, :invalid_input}
      true -> :ok
    end
  end

  @doc "Validate an `owner/repo` identifier for workspace and clone operations."
  @spec validate_repo(binary()) :: :ok | {:error, :invalid_repo}
  def validate_repo(repo) when is_binary(repo) do
    with :ok <- validate_input(repo),
         [owner, name] <- String.split(repo, "/", parts: 2),
         true <- valid_repo_segment?(owner),
         true <- valid_repo_segment?(name) do
      :ok
    else
      _ -> {:error, :invalid_repo}
    end
  end

  def validate_repo(_repo), do: {:error, :invalid_repo}

  @doc "Return the warm mirror root for a validated `owner/repo` identifier."
  @spec repo_root(binary()) :: binary()
  def repo_root(repo) do
    case validate_repo(repo) do
      :ok ->
        Path.join(@mirror_base, repo)

      {:error, :invalid_repo} ->
        raise ArgumentError, "invalid repo path: #{inspect(repo)}"
    end
  end

  @doc """
  Materialize the merged persona files and linked skills for the workspace role.
  """
  @spec sync_persona(binary(), binary(), atom() | binary(), keyword()) :: :ok | {:error, term()}
  def sync_persona(sprite, workspace, role, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)

    with {:ok, role_name} <- normalize_persona_role(role),
         source_root <- Keyword.get_lazy(opts, :source_root, &Config.persona_source_root!/0),
         {:ok, local_persona_dir} <- build_persona_tree(source_root, role_name) do
      try do
        with {:ok, _output} <-
               exec_fn.(sprite, prepare_persona_command(workspace, role_name), timeout: 30_000),
             {:ok, _output} <-
               exec_fn.(sprite, "true",
                 files:
                   persona_uploads(local_persona_dir, persona_launch_dir(workspace, role_name)),
                 timeout: 30_000
               ),
             {:ok, _output} <-
               exec_fn.(sprite, link_persona_skills_command(workspace, role_name),
                 timeout: 30_000
               ) do
          :ok
        else
          {:error, msg, code} -> {:error, "persona sync failed (#{code}): #{msg}"}
          {:error, reason} -> {:error, reason}
        end
      after
        File.rm_rf(local_persona_dir)
      end
    else
      {:error, reason} -> {:error, reason}
    end
  end

  @doc "Normalize a supported persona role to its string form."
  @spec normalize_persona_role(atom() | binary()) :: {:ok, binary()} | {:error, :invalid_role}
  def normalize_persona_role(role) when is_atom(role),
    do: normalize_persona_role(Atom.to_string(role))

  def normalize_persona_role(role) when is_binary(role) do
    if role in @persona_roles do
      {:ok, role}
    else
      {:error, :invalid_role}
    end
  end

  @doc "Return the workspace-local launch directory for a supported persona role."
  @spec persona_launch_dir(binary(), atom() | binary()) :: binary()
  def persona_launch_dir(workspace, role) do
    role_name =
      case normalize_persona_role(role) do
        {:ok, value} -> value
        {:error, :invalid_role} -> raise ArgumentError, "invalid persona role: #{inspect(role)}"
      end

    Path.join([workspace, ".bb", "persona", role_name])
  end

  @doc "Map a fleet role atom to its persona name."
  @spec persona_for_role(atom()) :: atom()
  def persona_for_role(:builder), do: :weaver
  def persona_for_role(:fixer), do: :thorn
  def persona_for_role(:polisher), do: :fern
  def persona_for_role(:triage), do: :muse
  def persona_for_role(:responder), do: :tansy
  def persona_for_role(role), do: role

  # --- Private ---

  defp prepare_persona_command(workspace, role_name) do
    launch_dir = persona_launch_dir(workspace, role_name)

    """
    set -e
    rm -rf #{shell_quote(launch_dir)}
    mkdir -p #{shell_quote(Path.join(launch_dir, ".agents/skills"))}
    mkdir -p #{shell_quote(Path.join(launch_dir, ".claude"))}
    mkdir -p #{shell_quote(Path.join(workspace, ".claude/skills"))}
    mkdir -p #{shell_quote(Path.join(workspace, ".agents/skills"))}
    """
  end

  defp link_persona_skills_command(workspace, role_name) do
    launch_dir = persona_launch_dir(workspace, role_name)
    agents_skills_dir = Path.join(workspace, ".agents/skills")
    claude_skills_dir = Path.join(workspace, ".claude/skills")

    """
    set -e
    rm -rf #{shell_quote(Path.join(launch_dir, ".claude/skills"))}
    ln -s ../.agents/skills #{shell_quote(Path.join(launch_dir, ".claude/skills"))}
    rm -f #{shell_quote(Path.join(workspace, ".claude/CLAUDE.md"))}
    ln -s #{shell_quote(Path.join(launch_dir, "CLAUDE.md"))} #{shell_quote(Path.join(workspace, ".claude/CLAUDE.md"))}
    agents_skills_dir=#{shell_quote(agents_skills_dir)}
    claude_skills_dir=#{shell_quote(claude_skills_dir)}
    rm -rf "$claude_skills_dir"/bb-persona-#{role_name}-*
    rm -rf "$agents_skills_dir"/bb-persona-#{role_name}-*
    for source in #{shell_quote(Path.join(launch_dir, ".agents/skills"))}/*; do
      [ -e "$source" ] || continue
      name=$(basename "$source")
      agents_target="$agents_skills_dir/bb-persona-#{role_name}-$name"
      claude_target="$claude_skills_dir/bb-persona-#{role_name}-$name"
      rm -rf "$agents_target" "$claude_target"
      ln -s "$source" "$agents_target"
      ln -s "$source" "$claude_target"
    done
    """
  end

  defp build_persona_tree(source_root, role_name) do
    with :ok <- validate_persona_sources(source_root, role_name) do
      local_persona_dir =
        Path.join(
          System.tmp_dir!(),
          "bb-persona-#{role_name}-#{System.unique_integer([:positive])}"
        )

      try do
        File.rm_rf!(local_persona_dir)
        File.mkdir_p!(Path.join(local_persona_dir, ".agents/skills"))
        File.mkdir_p!(Path.join(local_persona_dir, ".claude"))

        write_merged_persona_file(source_root, role_name, "CLAUDE.md", local_persona_dir)
        write_merged_persona_file(source_root, role_name, "AGENTS.md", local_persona_dir)
        copy_skill_tree(source_root, role_name, Path.join(local_persona_dir, ".agents/skills"))

        {:ok, local_persona_dir}
      rescue
        error ->
          File.rm_rf(local_persona_dir)
          {:error, Exception.message(error)}
      end
    end
  end

  defp validate_persona_sources(source_root, role_name) do
    required_paths = [
      Path.join([source_root, "shared", "CLAUDE.md"]),
      Path.join([source_root, "shared", "AGENTS.md"]),
      Path.join([source_root, role_name, "CLAUDE.md"]),
      Path.join([source_root, role_name, "AGENTS.md"])
    ]

    case Enum.find(required_paths, &(not File.exists?(&1))) do
      nil -> :ok
      path -> {:error, "missing persona source #{path}"}
    end
  end

  defp write_merged_persona_file(source_root, role_name, filename, local_persona_dir) do
    contents =
      [
        File.read!(Path.join([source_root, "shared", filename])),
        File.read!(Path.join([source_root, role_name, filename]))
      ]
      |> Enum.join()

    File.write!(Path.join(local_persona_dir, filename), contents)
  end

  defp copy_skill_tree(source_root, role_name, destination) do
    for skill_root <- [
          Path.join([source_root, "shared", "skills"]),
          Path.join([source_root, role_name, "skills"])
        ],
        File.dir?(skill_root) do
      for entry <- File.ls!(skill_root) do
        File.cp_r!(Path.join(skill_root, entry), Path.join(destination, entry))
      end
    end
  end

  defp persona_uploads(local_persona_dir, remote_persona_dir) do
    local_persona_dir
    |> Path.join("**/*")
    |> Path.wildcard(match_dot: true)
    |> Enum.filter(&File.regular?/1)
    |> Enum.map(fn source ->
      relative_path = Path.relative_to(source, local_persona_dir)
      destination = Path.join(remote_persona_dir, relative_path)
      {source, destination}
    end)
  end

  defp shell_quote(value), do: Shell.quote_arg(to_string(value))

  defp valid_repo_segment?(segment), do: segment != "." and Regex.match?(@repo_segment, segment)
end
