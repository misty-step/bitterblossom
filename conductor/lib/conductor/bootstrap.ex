defmodule Conductor.Bootstrap do
  @moduledoc """
  Spellbook bootstrap on sprites.

  Clones or updates the spellbook repo on a sprite, then runs bootstrap.sh
  which symlinks skills and agents into the codex/claude harness directories.
  This gives every sprite the full spellbook capability set before dispatch.
  """

  require Logger

  alias Conductor.{Config, Sprite}

  @spellbook_dir "/home/sprite/spellbook"

  @doc """
  Ensure the spellbook is cloned and bootstrapped on a sprite.

  Idempotent: pulls if already cloned, clones if not.
  """
  @spec ensure_spellbook(binary(), keyword()) :: :ok | {:error, term()}
  def ensure_spellbook(sprite, opts \\ []) do
    repo = Keyword.get(opts, :spellbook_repo, Config.spellbook_repo())
    exec_fn = Keyword.get(opts, :exec_fn, &Sprite.exec/3)

    with :ok <- clone_or_pull(sprite, repo, exec_fn),
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

  defp clone_or_pull(sprite, repo, exec_fn) do
    cmd = """
    if [ -d #{@spellbook_dir}/.git ]; then
      cd #{@spellbook_dir} && git pull --ff-only --quiet 2>&1 || true
    else
      git clone --depth 1 https://github.com/#{repo}.git #{@spellbook_dir} 2>&1
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
      -type l ! -e 2>/dev/null | xargs rm -f 2>/dev/null; true
    """

    case exec_fn.(sprite, cmd, timeout: 15_000) do
      {:ok, _} -> :ok
      {:error, _, _} -> :ok
    end
  end

  defp run_bootstrap(sprite, exec_fn) do
    cmd = "cd #{@spellbook_dir} && bash bootstrap.sh 2>&1"

    case exec_fn.(sprite, cmd, timeout: 60_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, "spellbook bootstrap failed: #{msg}"}
    end
  end
end
