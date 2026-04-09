defmodule Conductor.Launcher do
  @moduledoc """
  Dispatch autonomous agent loops to sprites.

  Each sprite gets bootstrapped with spellbook, synced with its
  persona, and launched with a minimal loop prompt. The agent reads its
  AGENTS.md and runs its own work loop — no orchestrator needed.
  """

  require Logger

  alias Conductor.{Bootstrap, Shell, Sprite, Workspace}

  @harness_modules %{
    "claude-code" => Conductor.ClaudeCode
  }
  @default_harness Conductor.Codex

  @doc """
  Launch a single sprite with its autonomous agent loop.

  1. Bootstrap spellbook
  2. Sync persona for the sprite's role
  3. Start detached agent loop
  """
  @spec launch(map(), binary(), keyword()) :: {:ok, binary()} | {:error, term()}
  def launch(sprite_config, repo, opts \\ []) do
    sprite = sprite_config.name
    role = sprite_config.role

    Logger.info("[launcher] launching #{sprite} (#{role})")

    workspace = workspace_module().repo_root(repo)
    persona = workspace_module().persona_for_role(role)

    # Preflight: kill stale processes and pid files from any previous run.
    # Best-effort — don't fail launch if cleanup errors.
    case sprite_module().stop_loop(sprite) do
      :ok ->
        :ok

      {:error, reason} ->
        Logger.debug("[launcher] #{sprite} preflight cleanup: #{inspect(reason)}")
    end

    # Detect auth failures from previous run before re-sync.
    case sprite_module().detect_auth_failure(sprite) do
      {:auth_failure, reason} ->
        Logger.warning("[launcher] #{sprite} auth failure detected: #{reason}")

      :ok ->
        :ok
    end

    with :ok <- maybe_sync_codex_auth(sprite),
         :ok <- bootstrap_module().ensure_spellbook(sprite),
         :ok <- ensure_repo_checkout(sprite_config, repo, workspace),
         :ok <- workspace_module().sync_persona(sprite, workspace, persona) do
      prompt = loop_prompt(sprite_config, repo)

      harness = Map.get(@harness_modules, sprite_config[:harness], @default_harness)
      harness_opts = [reasoning_effort: sprite_config[:reasoning_effort] || "medium"]

      case sprite_module().start_loop(sprite, prompt, repo, [
             {:workspace, workspace},
             {:persona_role, persona},
             {:harness, harness},
             {:harness_opts, harness_opts} | opts
           ]) do
        {:ok, output} ->
          Logger.info("[launcher] #{sprite} loop started")
          {:ok, output}

        {:error, msg, _code} ->
          Logger.warning("[launcher] #{sprite} loop failed to start: #{msg}")
          {:error, msg}
      end
    else
      {:error, reason} ->
        Logger.warning("[launcher] #{sprite} setup failed: #{inspect(reason)}")
        {:error, reason}
    end
  end

  @doc "Build the minimal loop prompt for a sprite role."
  @spec loop_prompt(map(), binary()) :: binary()
  def loop_prompt(sprite_config, repo) do
    role = sprite_config.role
    name = role_display_name(role)

    """
    # #{name} Loop

    Repository: #{repo}

    You are #{name}. Read your AGENTS.md for your full loop definition.
    Execute your loop: observe the repository state, decide what needs doing, do it, repeat.

    Your skills are installed. Use them.
    """
  end

  defp role_display_name(:builder), do: "Weaver"
  defp role_display_name(:fixer), do: "Thorn"
  defp role_display_name(:polisher), do: "Fern"
  defp role_display_name(:triage), do: "Muse"
  defp role_display_name(:responder), do: "Tansy"
  defp role_display_name(role), do: to_string(role) |> String.capitalize()

  defp ensure_repo_checkout(sprite_config, repo, workspace) do
    sprite = sprite_config.name
    git_dir = Path.join(workspace, ".git")

    case sprite_module().exec(sprite, "test -d #{shell_quote(git_dir)}", timeout: 15_000) do
      {:ok, _} ->
        refresh_workspace(sprite, workspace)

      {:error, reason, _code} ->
        Logger.info(
          "[launcher] #{sprite} repo checkout missing at #{workspace}, reprovisioning: #{reason}"
        )

        sprite_module().provision(sprite,
          repo: repo,
          persona: sprite_config.persona,
          harness: sprite_config.harness,
          force: false
        )
    end
  end

  defp refresh_workspace(sprite, workspace) do
    refresh_cmd =
      "cd #{shell_quote(workspace)} && " <>
        "git fetch origin && " <>
        "git checkout -f origin/master && " <>
        "git clean -fd"

    case sprite_module().exec(sprite, refresh_cmd, timeout: 60_000) do
      {:ok, _} ->
        Logger.info("[launcher] #{sprite} workspace refreshed to origin/master")
        :ok

      {:error, msg, _code} ->
        Logger.warning("[launcher] #{sprite} workspace refresh failed: #{msg}")
        {:error, msg}
    end
  end

  defp sprite_module do
    Application.get_env(:conductor, :sprite_module, Sprite)
  end

  defp bootstrap_module do
    Application.get_env(:conductor, :bootstrap_module, Bootstrap)
  end

  defp workspace_module do
    Application.get_env(:conductor, :workspace_module, Workspace)
  end

  defp maybe_sync_codex_auth(sprite) do
    if System.get_env("OPENAI_API_KEY") do
      :ok
    else
      sprite_module().force_sync_codex_auth(sprite)
    end
  end

  defp shell_quote(value), do: Shell.quote_arg(to_string(value))
end
