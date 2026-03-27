defmodule Conductor.Application do
  @moduledoc """
  OTP supervision tree for Bitterblossom infrastructure.

  No judgment lives here — just the plumbing that gives agents a healthy
  environment: sprite provisioning, health monitoring, event logging.
  """

  use Application
  require Logger

  @impl true
  def start(_type, _args) do
    children =
      [
        {Phoenix.PubSub, name: Conductor.PubSub},
        Conductor.Store,
        {Task.Supervisor, name: Conductor.TaskSupervisor},
        Conductor.Fleet.HealthMonitor
      ]

    Supervisor.start_link(children, strategy: :one_for_one, name: Conductor.Supervisor)
  end

  @doc """
  Launch the fleet: reconcile sprites, bootstrap spellbook, dispatch agent loops.
  """
  @spec launch_fleet(binary()) :: :ok | {:error, term()}
  def launch_fleet(fleet_path) do
    alias Conductor.Fleet.Loader

    case Loader.load(fleet_path) do
      {:error, reason} ->
        Logger.error("[boot] fleet.toml: #{reason}")
        {:error, reason}

      {:ok, config} ->
        launch_with_config(config, fleet_path)
    end
  end

  defp launch_with_config(config, fleet_path) do
    sprites = config.sprites
    defaults = config.defaults
    repo = defaults.repo

    Logger.info("[boot] loaded #{length(sprites)} sprite(s) from #{fleet_path}")

    # 1. Reconcile all sprites (idempotent provisioning + health check)
    {:ok, results} = fleet_reconciler().reconcile_all(sprites)
    healthy = MapSet.new(for r <- results, r.healthy, do: r.name)

    if MapSet.size(healthy) == 0 do
      Logger.warning("[boot] no healthy sprites — HealthMonitor will recover")
    end

    # 2. Configure health monitor
    Conductor.Fleet.HealthMonitor.configure(
      sprites: sprites,
      repo: repo,
      healthy: healthy
    )

    # 3. Dispatch agent loops for all healthy sprites
    #    Unhealthy sprites will be re-launched by HealthMonitor when they recover.
    healthy_sprites = Enum.filter(sprites, &MapSet.member?(healthy, &1.name))

    Logger.info(
      "[boot] launching #{length(healthy_sprites)} agent loop(s), " <>
        "#{length(sprites) - length(healthy_sprites)} deferred to HealthMonitor"
    )

    for sprite <- healthy_sprites do
      launch_with_restart(sprite, repo)
    end

    # 4. Store fleet config for runtime queries
    Application.put_env(:conductor, :fleet_config, config)
    Application.put_env(:conductor, :fleet_sprites, sprites)

    Logger.info("[boot] bitterblossom running")
    :ok
  end

  @doc "Attach the optional canary client when environment variables are present."
  def attach_canary do
    with endpoint when is_binary(endpoint) <- System.get_env("CANARY_ENDPOINT"),
         api_key when is_binary(api_key) <- System.get_env("CANARY_API_KEY") do
      try do
        CanarySdk.attach(endpoint: endpoint, api_key: api_key, service: "bitterblossom")
      rescue
        e ->
          require Logger
          Logger.warning("[canary] attach failed: #{Exception.message(e)}")
          :ok
      end
    else
      _ -> :ok
    end
  end

  @doc "Start the dashboard endpoint under the main supervisor."
  @spec start_dashboard() :: :ok | {:error, term()}
  def start_dashboard do
    if Application.get_env(:conductor, :start_dashboard, false) do
      case Supervisor.start_child(Conductor.Supervisor, Conductor.Web.Endpoint) do
        {:ok, _pid} -> :ok
        {:error, {:already_started, _pid}} -> :ok
        other -> other
      end
    else
      :ok
    end
  end

  @restart_backoff_ms 30_000

  @doc false
  def launch_with_restart(sprite, repo) do
    Task.Supervisor.start_child(Conductor.TaskSupervisor, fn ->
      case Conductor.Launcher.launch(sprite, repo) do
        {:ok, _} ->
          Logger.info(
            "[launcher] #{sprite.name} completed, restarting in #{div(@restart_backoff_ms, 1000)}s"
          )

        {:error, reason} ->
          Logger.warning(
            "[launcher] #{sprite.name} failed: #{inspect(reason)}, restarting in #{div(@restart_backoff_ms, 1000)}s"
          )
      end

      Process.sleep(@restart_backoff_ms)
      launch_with_restart(sprite, repo)
    end)
  end

  defp fleet_reconciler do
    Application.get_env(:conductor, :fleet_reconciler, Conductor.Fleet.Reconciler)
  end
end
