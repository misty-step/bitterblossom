defmodule Conductor.Launcher do
  @moduledoc """
  Dispatch autonomous agent loops to sprites.

  Each sprite gets provisioned, bootstrapped with spellbook, synced with its
  persona, and dispatched with a minimal loop prompt. The agent reads its
  AGENTS.md and runs its own work loop — no orchestrator needed.
  """

  require Logger

  alias Conductor.{Bootstrap, Config, Sprite, Workspace}

  @doc """
  Launch a single sprite with its autonomous agent loop.

  1. Provision (codex, auth, repo)
  2. Bootstrap spellbook
  3. Sync persona for the sprite's role
  4. Dispatch with loop prompt
  """
  @spec launch(map(), binary(), keyword()) :: {:ok, binary()} | {:error, term()}
  def launch(sprite_config, repo, _opts \\ []) do
    sprite = sprite_config.name
    role = sprite_config.role

    Logger.info("[launcher] launching #{sprite} (#{role})")

    workspace = Workspace.repo_root(repo)

    # Reconciliation already provisioned the sprite. We just need spellbook + persona + dispatch.
    persona = persona_for_role(role)

    with :ok <- Sprite.force_sync_codex_auth(sprite),
         :ok <- Bootstrap.ensure_spellbook(sprite),
         :ok <- Workspace.sync_persona(sprite, workspace, persona) do
      prompt = loop_prompt(sprite_config, repo)

      case Sprite.dispatch(sprite, prompt, repo,
             workspace: workspace,
             persona_role: persona,
             timeout: Config.session_timeout_minutes()
           ) do
        {:ok, output} ->
          Logger.info("[launcher] #{sprite} loop completed")
          {:ok, output}

        {:error, msg, _code} ->
          Logger.warning("[launcher] #{sprite} loop failed: #{msg}")
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

  defp persona_for_role(:builder), do: :weaver
  defp persona_for_role(:fixer), do: :thorn
  defp persona_for_role(:polisher), do: :fern
  defp persona_for_role(role), do: role

  defp role_display_name(:builder), do: "Weaver"
  defp role_display_name(:fixer), do: "Thorn"
  defp role_display_name(:polisher), do: "Fern"
  defp role_display_name(role), do: to_string(role) |> String.capitalize()

end
