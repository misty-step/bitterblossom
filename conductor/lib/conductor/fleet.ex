defmodule Conductor.Fleet do
  @moduledoc """
  Fleet provisioning — infrastructure as code for sprites.

  Reads fleet declaration from config, ensures each sprite exists
  and has the correct settings. Called on conductor startup and
  available as `mix conductor provision`.

  Never create or configure sprites manually. This module is the
  source of truth.
  """

  require Logger
  alias Conductor.{Sprite, Shell, Config}

  @settings_path "/home/sprite/.claude/settings.json"

  @doc "Provision all sprites declared in fleet config."
  @spec provision!() :: :ok
  def provision! do
    fleet = Application.get_env(:conductor, :fleet, [])

    if fleet == [] do
      Logger.warning("[fleet] no sprites declared in config")
      :ok
    else
      Enum.each(fleet, &provision_sprite/1)
      Logger.info("[fleet] provisioned #{length(fleet)} sprite(s)")
      :ok
    end
  end

  @doc "List fleet status — name, role, reachable, configured."
  @spec status() :: [map()]
  def status do
    fleet = Application.get_env(:conductor, :fleet, [])

    Enum.map(fleet, fn sprite_config ->
      name = sprite_config.name
      reachable = Sprite.reachable?(name)

      %{
        name: name,
        role: sprite_config.role,
        org: sprite_config[:org] || Config.sprites_org!(),
        reachable: reachable
      }
    end)
  end

  @doc "Get sprite names by role."
  @spec by_role(atom()) :: [binary()]
  def by_role(role) do
    Application.get_env(:conductor, :fleet, [])
    |> Enum.filter(&(&1.role == role))
    |> Enum.map(& &1.name)
  end

  # --- Private ---

  defp provision_sprite(sprite_config) do
    name = sprite_config.name
    org = sprite_config[:org] || Config.sprites_org!()

    # 1. Ensure sprite exists (create if not)
    ensure_exists(name, org)

    # 2. Push settings.json with API key
    push_settings(name)

    Logger.info("[fleet] provisioned #{name} (role=#{sprite_config.role})")
  end

  defp ensure_exists(name, org) do
    case Shell.cmd("sprite", ["-o", org, "-s", name, "exec", "echo", "ok"], timeout: 15_000) do
      {:ok, _} ->
        :ok

      {:error, _, _} ->
        Logger.info("[fleet] creating sprite #{name}")
        Shell.cmd("sprite", ["-o", org, "create", name], timeout: 30_000)
    end
  end

  defp push_settings(name) do
    api_key = System.get_env("ANTHROPIC_API_KEY") || ""

    if api_key == "" do
      Logger.warning(
        "[fleet] ANTHROPIC_API_KEY not set — #{name} will not be able to run Claude Code"
      )
    end

    settings =
      Jason.encode!(%{
        model: "sonnet",
        skipDangerousModePermissionPrompt: true,
        permissions: %{defaultMode: "bypassPermissions"},
        env: %{
          ANTHROPIC_API_KEY: api_key,
          CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
          API_TIMEOUT_MS: "600000"
        }
      })

    encoded = Base.encode64(settings)

    Sprite.exec(
      name,
      "mkdir -p $(dirname #{@settings_path}) && echo #{encoded} | base64 -d > #{@settings_path}",
      timeout: 15_000
    )
  end
end
