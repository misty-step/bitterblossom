defmodule Conductor.Workspace do
  @moduledoc """
  Worktree lifecycle on sprites.

  Each run gets an isolated git worktree under the warm mirror.
  Preparation and cleanup are idempotent — stale state is cleaned first.
  """

  alias Conductor.{Config, Sprite}
  @mirror_base "/home/sprite/workspace"
  @safe_input ~r/^[a-zA-Z0-9_\-\.\/]+$/
  @persona_roles ~w(weaver thorn fern)

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

  @spec prepare(binary(), binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, term()}
  def prepare(sprite, repo, run_id, branch, opts \\ []) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id),
         :ok <- validate_input(branch) do
      do_prepare(sprite, repo, run_id, branch, opts)
    end
  end

  defp do_prepare(sprite, repo, run_id, branch, opts) do
    mirror = repo_mirror(repo, opts)
    worktree = worktree_path(repo, run_id, opts)
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)
    managed_prefix = "#{mirror}/.bb/conductor/"

    commands = """
    set -e
    mirror=#{shell_quote(mirror)}
    worktree_dir=#{shell_quote(worktree)}
    branch_name=#{shell_quote(branch)}
    managed_prefix=#{shell_quote(managed_prefix)}
    cd "$mirror"
    export worktree_dir branch_name managed_prefix
    flock .git/bb-worktree.lock bash <<'EOF'
      set -e
      git fetch --all --prune --quiet 2>/dev/null || true
      git worktree prune 2>/dev/null || true
      #{clear_existing_branch_worktree_commands(delete_branch?: true)}
      rm -rf "$worktree_dir" 2>/dev/null || true
      default_branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed "s|refs/remotes/origin/||" || echo master)
      git worktree add -b "$branch_name" "$worktree_dir" origin/$default_branch --quiet
    EOF
    #{install_branch_guard_commands(worktree, branch)}
    echo "$worktree_dir"
    """

    case exec_fn.(sprite, commands, timeout: 120_000) do
      {:ok, output} ->
        {:ok, extract_path(output)}

      {:error, msg, code} ->
        {:error, "workspace preparation failed (#{code}): #{msg}"}
    end
  end

  @doc """
  Rebase an existing PR branch on top of the default branch and force-push.

  Uses a temporary worktree to avoid disturbing the mirror's HEAD.
  Returns `:ok` on success, `{:error, reason}` if rebase or push fails.
  """
  @spec rebase(binary(), binary(), binary()) :: :ok | {:error, term()}
  def rebase(sprite, repo, branch) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(branch) do
      do_rebase(sprite, repo, branch)
    end
  end

  defp do_rebase(sprite, repo, branch) do
    repo_name = repo |> String.split("/") |> List.last()
    mirror = Path.join(@mirror_base, repo_name)
    safe = String.replace(branch, "/", "-")
    tmp = Path.join([mirror, ".bb", "rebase-#{safe}"])

    commands = """
    set -e
    cd #{mirror}
    flock .git/bb-worktree.lock bash -c '
      git fetch origin --quiet
      git worktree prune 2>/dev/null || true
      rm -rf #{tmp} 2>/dev/null || true
      git worktree add #{tmp} #{branch} --quiet
      cd #{tmp}
      default_branch=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed "s|refs/remotes/origin/||" || echo master)
      git rebase origin/$default_branch
      git push --force-with-lease origin #{branch}
      cd #{mirror}
      git worktree remove --force #{tmp} 2>/dev/null || true
      git worktree prune 2>/dev/null || true
    '
    """

    case Sprite.exec(sprite, commands, timeout: 120_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @doc """
  Prepare a worktree by checking out an existing remote branch (no -b flag).
  Used when adopting a PR branch from a prior run instead of building fresh.
  """
  @spec adopt_branch(binary(), binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, term()}
  def adopt_branch(sprite, repo, run_id, branch, opts \\ []) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id),
         :ok <- validate_input(branch) do
      do_adopt_branch(sprite, repo, run_id, branch, opts)
    end
  end

  defp do_adopt_branch(sprite, repo, run_id, branch, opts) do
    mirror = repo_mirror(repo, opts)
    worktree = worktree_path(repo, run_id, opts)
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)
    managed_prefix = "#{mirror}/.bb/conductor/"

    commands = """
    set -e
    mirror=#{shell_quote(mirror)}
    worktree_dir=#{shell_quote(worktree)}
    branch_name=#{shell_quote(branch)}
    managed_prefix=#{shell_quote(managed_prefix)}
    cd "$mirror"
    export worktree_dir branch_name managed_prefix
    flock .git/bb-worktree.lock bash <<'EOF'
      set -e
      git fetch origin --quiet
      git worktree prune 2>/dev/null || true
      #{clear_existing_branch_worktree_commands()}
      rm -rf "$worktree_dir" 2>/dev/null || true
      git worktree add "$worktree_dir" "$branch_name" --quiet
    EOF
    #{install_branch_guard_commands(worktree, branch)}
    echo "$worktree_dir"
    """

    case exec_fn.(sprite, commands, timeout: 120_000) do
      {:ok, output} ->
        {:ok, extract_path(output)}

      {:error, msg, code} ->
        {:error, "branch adoption failed (#{code}): #{msg}"}
    end
  end

  @spec cleanup(binary(), binary(), binary()) :: :ok | {:error, term()}
  def cleanup(sprite, repo, run_id),
    do: cleanup(sprite, repo, run_id, branch: run_id_to_branch(run_id))

  @spec cleanup(binary(), binary(), binary(), keyword()) :: :ok | {:error, term()}
  def cleanup(sprite, repo, run_id, opts) do
    with :ok <- validate_input(repo),
         :ok <- validate_input(run_id),
         :ok <- validate_optional_branch(branch_option(opts)) do
      do_cleanup(sprite, repo, run_id, opts)
    end
  end

  defp do_cleanup(sprite, repo, run_id, opts) do
    mirror = repo_mirror(repo, opts)
    worktree = Keyword.get(opts, :path, worktree_path(repo, run_id, opts))
    branch = branch_option(opts)
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)

    commands = """
    set -e
    mirror=#{shell_quote(mirror)}
    worktree_dir=#{shell_quote(worktree)}
    cd "$mirror"
    export worktree_dir
    #{cleanup_branch_assignment(branch)}
    flock .git/bb-worktree.lock bash <<'EOF'
      set -e
      if git worktree list --porcelain | grep -F "worktree $worktree_dir" >/dev/null 2>&1; then
        git worktree remove --force "$worktree_dir" 2>/dev/null || true
      fi
      if [ -d "$worktree_dir" ]; then
        rm -rf "$worktree_dir" 2>/dev/null || true
      fi
      git worktree prune 2>/dev/null || true
      #{delete_branch_command(branch)}
    EOF
    """

    case exec_fn.(sprite, commands, timeout: 60_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  # run-648-1773580938 → factory/648-1773580938
  defp run_id_to_branch("run-" <> rest), do: "factory/#{rest}"
  defp run_id_to_branch(_), do: nil

  @spec factory_branch?(binary() | nil) :: boolean()
  def factory_branch?(branch) when is_binary(branch) and branch != "" do
    String.starts_with?(branch, "factory/")
  end

  def factory_branch?(_branch), do: false

  defp validate_optional_branch(nil), do: :ok
  defp validate_optional_branch(branch), do: validate_input(branch)

  defp extract_path(output) do
    output
    |> String.split("\n", trim: true)
    |> List.last()
    |> String.trim()
  end

  defp repo_mirror(repo, opts) do
    repo
    |> String.split("/")
    |> List.last()
    |> then(&Path.join(mirror_base(opts), &1))
  end

  defp worktree_path(repo, run_id, opts) do
    Path.join([repo_mirror(repo, opts), ".bb", "conductor", run_id, "builder-worktree"])
  end

  defp mirror_base(opts) do
    Keyword.get(opts, :mirror_base, @mirror_base)
  end

  defp clear_existing_branch_worktree_commands(opts \\ []) do
    delete_branch? = Keyword.get(opts, :delete_branch?, false)

    """
    existing_worktree=$(
      git worktree list --porcelain | while IFS= read -r line; do
        case "$line" in
          worktree\\ *)
            current_worktree=${line#worktree }
            ;;
          branch\\ *)
            current_branch=${line#branch }
            if [ "$current_branch" = "refs/heads/$branch_name" ]; then
              printf '%s\\n' "$current_worktree"
              break
            fi
            ;;
        esac
      done
    )
    if [ -n "$existing_worktree" ] && [ "$existing_worktree" != "$worktree_dir" ]; then
      if [[ "$existing_worktree" == "$managed_prefix"*"/builder-worktree" ]]; then
        git worktree remove --force "$existing_worktree" 2>/dev/null || rm -rf "$existing_worktree" 2>/dev/null || true
        git worktree prune 2>/dev/null || true
      else
        echo "branch $branch_name already uses worktree $existing_worktree" >&2
        exit 17
      fi
    fi
    #{if delete_branch?, do: delete_branch_command(:use_branch_name_var), else: ""}
    """
  end

  defp delete_branch_command(nil), do: ""

  defp delete_branch_command(:use_branch_name_var),
    do: "git branch -D \"$branch_name\" 2>/dev/null || true"

  defp delete_branch_command(branch) do
    "git branch -D #{shell_quote(branch)} 2>/dev/null || true"
  end

  defp branch_option(opts) do
    case Keyword.fetch(opts, :branch) do
      {:ok, branch} -> branch
      :error -> nil
    end
  end

  defp cleanup_branch_assignment(nil), do: ""

  defp cleanup_branch_assignment(branch) do
    """
    branch_name=#{shell_quote(branch)}
    export branch_name
    """
  end

  defp install_branch_guard_commands(worktree, branch) do
    """
    git config extensions.worktreeConfig true
    git -C #{shell_quote(worktree)} config --worktree core.hooksPath .bb-hooks
    hook_dir=#{shell_quote(Path.join(worktree, ".bb-hooks"))}
    hook_path="$hook_dir/pre-push"
    mkdir -p "$hook_dir"
    cat > "$hook_path" <<EOF
    #!/usr/bin/env bash
    set -eu
    expected_branch="#{branch}"
    current_branch=$(git symbolic-ref --quiet --short HEAD 2>/dev/null || true)

    if [ "$current_branch" != "$expected_branch" ]; then
      echo "Bitterblossom branch guard: refusing push from $current_branch; expected $expected_branch" >&2
      exit 1
    fi

    while read -r local_ref local_sha remote_ref remote_sha; do
      if [ -z "${local_ref:-}" ]; then
        continue
      fi

      case "$local_ref" in
        refs/heads/*)
          local_branch=${local_ref#refs/heads/}
          ;;
        *)
          echo "Bitterblossom branch guard: refusing non-branch push $local_ref" >&2
          exit 1
          ;;
      esac

      if [ "$local_branch" != "$expected_branch" ]; then
        echo "Bitterblossom branch guard: refusing push from $local_branch; expected $expected_branch" >&2
        exit 1
      fi

      if [ "$remote_ref" != "refs/heads/$expected_branch" ]; then
        echo "Bitterblossom branch guard: refusing push to $remote_ref; expected refs/heads/$expected_branch" >&2
        exit 1
      fi
    done
    EOF
    chmod +x "$hook_path"
    """
  end

  @spec repo_root(binary()) :: binary()
  def repo_root(repo) do
    case validate_input(repo) do
      :ok ->
        repo_name = repo |> String.split("/") |> List.last()
        Path.join(@mirror_base, repo_name)

      {:error, :invalid_input} ->
        raise ArgumentError, "invalid repo path: #{inspect(repo)}"
    end
  end

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

  @spec persona_launch_dir(binary(), atom() | binary()) :: binary()
  def persona_launch_dir(workspace, role) do
    role_name =
      case normalize_persona_role(role) do
        {:ok, value} -> value
        {:error, :invalid_role} -> raise ArgumentError, "invalid persona role: #{inspect(role)}"
      end

    Path.join([workspace, ".bb", "persona", role_name])
  end

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

  defp shell_quote(value) do
    escaped = value |> to_string() |> String.replace("'", "'\"'\"'")
    "'#{escaped}'"
  end
end
