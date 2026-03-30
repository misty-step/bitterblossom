defmodule Conductor.Launcher do
  @moduledoc """
  Dispatch autonomous agent loops to sprites.

  Each sprite gets bootstrapped with spellbook, synced with its
  persona, and launched with a minimal loop prompt. The agent reads its
  AGENTS.md and runs its own work loop — no orchestrator needed.
  """

  require Logger

  alias Conductor.{Bootstrap, Sprite, Workspace}

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

    workspace = Workspace.repo_root(repo)
    persona = Workspace.persona_for_role(role)

    with :ok <- Sprite.force_sync_codex_auth(sprite),
         :ok <- Bootstrap.ensure_spellbook(sprite),
         :ok <- Workspace.sync_persona(sprite, workspace, persona) do
      prompt = loop_prompt(sprite_config, repo)

      case Sprite.start_loop(sprite, prompt, repo, [
             {:workspace, workspace},
             {:persona_role, persona} | opts
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
  defp role_display_name(role), do: to_string(role) |> String.capitalize()
end
