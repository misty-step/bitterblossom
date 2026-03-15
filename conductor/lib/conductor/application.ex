defmodule Conductor.Application do
  @moduledoc false

  use Application
  require Logger

  @impl true
  def start(_type, _args) do
    children =
      [
        {Phoenix.PubSub, name: Conductor.PubSub},
        Conductor.Store,
        Conductor.Retro,
        {DynamicSupervisor, name: Conductor.RunSupervisor, strategy: :one_for_one},
        Conductor.Orchestrator
      ] ++ dashboard_children()

    Supervisor.start_link(children, strategy: :one_for_one, name: Conductor.Supervisor)
  end

  @doc """
  Boot the full fleet from fleet.toml: reconcile sprites, start orchestrator
  loop, start fixer and polisher. Called by `mix conductor start`.
  """
  @spec boot_fleet(binary()) :: :ok | {:error, term()}
  def boot_fleet(fleet_path) do
    alias Conductor.Fleet.{Loader, Reconciler}

    # 1. Load and validate fleet.toml
    config = Loader.load!(fleet_path)
    sprites = config.sprites
    defaults = config.defaults

    Logger.info("[boot] loaded #{length(sprites)} sprite(s) from #{fleet_path}")

    # 2. Reconcile all sprites (idempotent provisioning)
    {:ok, results} = Reconciler.reconcile_all(sprites)
    healthy_sprites = Enum.filter(results, & &1.healthy) |> Enum.map(& &1.name)

    if healthy_sprites == [] do
      Logger.error("[boot] no healthy sprites after reconciliation — cannot start")
      {:error, :no_healthy_sprites}
    else
      # 3. Start orchestrator polling with builder sprites
      builders =
        Loader.by_role(sprites, :builder)
        |> Enum.filter(&(&1.name in healthy_sprites))
        |> Enum.map(& &1.name)

      repo = defaults.repo

      if builders != [] do
        Conductor.Orchestrator.start_loop(
          repo: repo,
          workers: builders,
          label: defaults.label
        )

        Logger.info("[boot] orchestrator polling with builders: #{Enum.join(builders, ", ")}")
      else
        Logger.warning("[boot] no healthy builders — orchestrator will not poll")
      end

      # 4. Start fixer
      fixers =
        Loader.by_role(sprites, :fixer)
        |> Enum.filter(&(&1.name in healthy_sprites))

      for fixer <- fixers do
        case Supervisor.start_child(Conductor.Supervisor, {
               Conductor.Fixer,
               repo: repo, fixer_sprite: fixer.name
             }) do
          {:ok, _} -> Logger.info("[boot] fixer started: #{fixer.name}")
          {:error, reason} -> Logger.warning("[boot] fixer failed: #{inspect(reason)}")
        end
      end

      # 5. Start polisher
      polishers =
        Loader.by_role(sprites, :polisher)
        |> Enum.filter(&(&1.name in healthy_sprites))

      for polisher <- polishers do
        case Supervisor.start_child(Conductor.Supervisor, {
               Conductor.Polisher,
               repo: repo, polisher_sprite: polisher.name
             }) do
          {:ok, _} -> Logger.info("[boot] polisher started: #{polisher.name}")
          {:error, reason} -> Logger.warning("[boot] polisher failed: #{inspect(reason)}")
        end
      end

      # 6. Store fleet config for runtime queries
      Application.put_env(:conductor, :fleet_config, config)
      Application.put_env(:conductor, :fleet_sprites, sprites)

      Logger.info("[boot] bitterblossom running — #{length(healthy_sprites)} healthy sprites")
      :ok
    end
  end

  # Only start the web endpoint when explicitly enabled (dashboard command sets this).
  defp dashboard_children do
    if Application.get_env(:conductor, :start_dashboard, false) do
      [Conductor.Web.Endpoint]
    else
      []
    end
  end
end
