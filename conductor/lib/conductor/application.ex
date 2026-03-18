defmodule Conductor.Application do
  @moduledoc false

  use Application
  require Logger

  @doc false
  @impl true
  def start(_type, _args) do
    children =
      [
        {Phoenix.PubSub, name: Conductor.PubSub},
        Conductor.Store,
        Conductor.Retro,
        {Task.Supervisor, name: Conductor.TaskSupervisor},
        {DynamicSupervisor, name: Conductor.RunSupervisor, strategy: :one_for_one},
        Conductor.Orchestrator
      ] ++ dashboard_children()

    result = Supervisor.start_link(children, strategy: :one_for_one, name: Conductor.Supervisor)

    case result do
      {:ok, _pid} -> attach_canary()
      _ -> :ok
    end

    result
  end

  @doc false
  def attach_canary do
    with endpoint when is_binary(endpoint) <- System.get_env("CANARY_ENDPOINT"),
         api_key when is_binary(api_key) <- System.get_env("CANARY_API_KEY") do
      try do
        CanarySdk.attach(
          endpoint: endpoint,
          api_key: api_key,
          service: "bitterblossom"
        )
      rescue
        e ->
          Logger.warning("[canary] attach failed: #{Exception.message(e)}")
          :ok
      end
    else
      _ -> :ok
    end
  end

  @doc """
  Boot the full fleet from fleet.toml: reconcile sprites, start the Weaver lane,
  then start Thorn and Fern. Called by `mix conductor start`.
  """
  @spec boot_fleet(binary()) :: :ok | {:error, term()}
  def boot_fleet(fleet_path) do
    alias Conductor.Fleet.{Loader, Reconciler}

    # 1. Load and validate fleet.toml
    case Loader.load(fleet_path) do
      {:error, reason} ->
        Logger.error("[boot] fleet.toml: #{reason}")
        {:error, reason}

      {:ok, config} ->
        boot_with_config(config, fleet_path, Reconciler)
    end
  end

  defp boot_with_config(config, fleet_path, reconciler_mod) do
    alias Conductor.Fleet.Loader

    sprites = config.sprites
    defaults = config.defaults
    repo = defaults.repo

    Logger.info("[boot] loaded #{length(sprites)} sprite(s) from #{fleet_path}")

    # 2. Reconcile all sprites (idempotent provisioning)
    {:ok, results} = reconciler_mod.reconcile_all(sprites)
    healthy = MapSet.new(for r <- results, r.healthy, do: r.name)

    if MapSet.size(healthy) == 0 do
      Logger.error("[boot] no healthy sprites after reconciliation — cannot start")
      {:error, :no_healthy_sprites}
    else
      # 3. Start orchestrator
      builders =
        Loader.by_role(sprites, :builder)
        |> Enum.filter(&MapSet.member?(healthy, &1.name))

      if builders != [] do
        maybe_warn_unfiltered_scope(repo, defaults.label)

        Conductor.Orchestrator.configure_polling(
          repo: repo,
          workers: builders,
          label: defaults.label
        )

        Logger.info(
          "[boot] orchestrator polling with weavers: #{Enum.map_join(builders, ", ", & &1.name)}"
        )
      else
        Logger.warning("[boot] no healthy weavers — orchestrator will not poll")
      end

      # 4. Start phase workers (fixer + polisher)
      start_phase_workers(sprites, healthy, repo)

      # 5. Store fleet config for runtime queries
      Application.put_env(:conductor, :fleet_config, config)
      Application.put_env(:conductor, :fleet_sprites, sprites)
      Application.put_env(:conductor, :fleet_workers, builders)

      Logger.info("[boot] bitterblossom running — #{MapSet.size(healthy)} healthy sprites")
      :ok
    end
  end

  defp maybe_warn_unfiltered_scope(repo, label) when label in [nil, ""] do
    Logger.warning("[boot] no default label configured for #{repo}; all open issues are eligible")
  end

  defp maybe_warn_unfiltered_scope(_repo, _label), do: :ok

  defp start_phase_workers(sprites, healthy, repo) do
    alias Conductor.Fleet.Loader

    role_to_module = %{
      fixer: {Conductor.Fixer, :fixer_sprite},
      polisher: {Conductor.Polisher, :polisher_sprite}
    }

    for {role, {module, sprite_key}} <- role_to_module do
      Loader.by_role(sprites, role)
      |> Enum.filter(&MapSet.member?(healthy, &1.name))
      |> Enum.each(fn sprite ->
        case Supervisor.start_child(Conductor.Supervisor, {
               module,
               [{:repo, repo}, {sprite_key, sprite.name}]
             }) do
          {:ok, _} ->
            Logger.info("[boot] #{role_display_name(role)} started: #{sprite.name}")

          {:error, reason} ->
            Logger.warning("[boot] #{role_display_name(role)} failed: #{inspect(reason)}")
        end
      end)
    end
  end

  defp role_display_name(:fixer), do: "thorn"
  defp role_display_name(:polisher), do: "fern"
  defp role_display_name(role), do: to_string(role)

  defp dashboard_children do
    if Application.get_env(:conductor, :start_dashboard, false) do
      [Conductor.Web.Endpoint]
    else
      []
    end
  end
end
