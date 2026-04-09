defmodule Conductor.Bootstrap do
  @moduledoc """
  Spellbook bootstrap on sprites.

  Syncs or clones the configured spellbook source onto a sprite, then runs
  bootstrap.sh which symlinks skills and agents into the codex/claude harness
  directories. This gives every sprite the full spellbook capability set before
  dispatch without hard-coding a GitHub transport.
  """

  require Logger

  alias Conductor.{Config, Shell, Sprite}

  @spellbook_dir "/home/sprite/spellbook"
  @local_source_excludes MapSet.new([".git", ".pytest_cache", "__pycache__", "node_modules"])

  @doc """
  Ensure the spellbook is present and bootstrapped on a sprite.

  Local directories are uploaded as the source of truth. Git sources are cloned
  or pulled on the sprite.
  """
  @spec ensure_spellbook(binary(), keyword()) :: :ok | {:error, term()}
  def ensure_spellbook(sprite, opts \\ []) do
    source = Keyword.get(opts, :spellbook_source, Config.spellbook_source())
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)

    with :ok <- sync_source(sprite, source, exec_fn),
         :ok <- clean_broken_symlinks(sprite, exec_fn),
         :ok <- run_bootstrap(sprite, exec_fn) do
      Logger.info("[bootstrap] spellbook ready on #{sprite}")
      :ok
    else
      {:error, reason} ->
        Logger.warning("[bootstrap] spellbook failed on #{sprite}: #{reason}")
        {:error, reason}
    end
  end

  defp sync_source(sprite, source, exec_fn) do
    case local_source_dir(source) do
      {:error, :missing_source} ->
        {:error,
         "missing spellbook source: set :spellbook_source to a local path or explicit git URL"}

      {:ok, local_dir} ->
        sync_local_source(sprite, local_dir, exec_fn)

      :remote ->
        clone_or_pull(sprite, normalize_git_source(source), exec_fn)

      {:error, reason} ->
        {:error, reason}
    end
  end

  defp sync_local_source(sprite, local_dir, exec_fn) do
    files = spellbook_uploads(local_dir)

    if files == [] do
      {:error, "spellbook source #{local_dir} has no files to upload"}
    else
      with {:ok, _} <-
             exec_fn.(
               sprite,
               "rm -rf #{shell_quote(@spellbook_dir)} && mkdir -p #{shell_quote(@spellbook_dir)}",
               timeout: 30_000
             ),
           {:ok, _} <- exec_fn.(sprite, "true", files: files, timeout: 120_000) do
        :ok
      else
        {:error, msg, _code} -> {:error, "spellbook sync failed: #{msg}"}
      end
    end
  end

  defp clone_or_pull(sprite, source, exec_fn) do
    cmd = """
    if [ -d #{@spellbook_dir}/.git ]; then
      cd #{@spellbook_dir} && git pull --ff-only --quiet 2>&1
    else
      git clone --depth 1 #{shell_quote(source)} #{@spellbook_dir} 2>&1
    fi
    """

    case exec_fn.(sprite, cmd, timeout: 60_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, "spellbook clone failed: #{msg}"}
    end
  end

  defp clean_broken_symlinks(sprite, exec_fn) do
    cmd = """
    find /home/sprite/.claude/skills /home/sprite/.codex/skills \
      -xtype l -print0 2>/dev/null | xargs -0 -r rm -f 2>/dev/null; true
    """

    case exec_fn.(sprite, cmd, timeout: 15_000) do
      {:ok, _} ->
        :ok

      {:error, msg, _code} ->
        Logger.debug("[bootstrap] broken symlink cleanup failed on #{sprite}: #{msg}")
        :ok
    end
  end

  defp run_bootstrap(sprite, exec_fn) do
    cmd = "cd #{@spellbook_dir} && bash bootstrap.sh 2>&1"

    case exec_fn.(sprite, cmd, timeout: 60_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, "spellbook bootstrap failed: #{msg}"}
    end
  end

  defp local_source_dir(nil), do: {:error, :missing_source}
  defp local_source_dir(""), do: {:error, :missing_source}

  defp local_source_dir(source) when is_binary(source) do
    expanded = Path.expand(source)

    cond do
      File.dir?(expanded) ->
        {:ok, expanded}

      path_like_source?(source) ->
        {:error, "spellbook source missing: #{expanded}"}

      true ->
        :remote
    end
  end

  defp spellbook_uploads(local_dir) do
    local_dir
    |> Path.join("**/*")
    |> Path.wildcard(match_dot: true)
    |> Enum.filter(&File.regular?/1)
    |> Enum.reject(fn source ->
      source
      |> Path.relative_to(local_dir)
      |> excluded_upload?()
    end)
    |> Enum.map(fn source ->
      relative_path = Path.relative_to(source, local_dir)
      destination = Path.join(@spellbook_dir, relative_path)
      {source, destination}
    end)
  end

  defp excluded_upload?(relative_path) do
    relative_path
    |> Path.split()
    |> Enum.any?(fn segment ->
      segment in @local_source_excludes or segment in [".DS_Store", ".env", ".venv"]
    end)
  end

  defp normalize_git_source(source), do: source

  defp path_like_source?(source) do
    cond do
      String.starts_with?(source, ["/", "./", "../", "~/"]) -> true
      String.contains?(source, "://") -> false
      String.starts_with?(source, "git@") -> false
      true -> true
    end
  end

  defp shell_quote(value), do: Shell.quote_arg(to_string(value))
end
