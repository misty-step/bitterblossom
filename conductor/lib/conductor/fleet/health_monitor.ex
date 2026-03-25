defmodule Conductor.Fleet.HealthMonitor do
  @moduledoc """
  Periodic fleet health re-check. Detects sprite recovery and auto-starts
  phase workers (Fixer, Polisher) that were skipped at boot due to unhealthy sprites.

  Health recovery and phase-worker lifecycle are intentionally coupled so the
  role worker pool tracks the current healthy sprite set.

  Deep module: hides all sprite lifecycle recovery behind a simple status/0 interface.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Store}
  alias Conductor.Fleet.Reconciler
  alias Conductor.PhaseWorker.Roles

  defstruct [
    :repo,
    :label,
    :interval_ms,
    :timer_ref,
    sprites: [],
    known_health: %{}
  ]

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @doc "Configure the monitor with fleet sprites and repo. Called from boot_fleet/1."
  @spec configure(keyword()) :: :ok
  def configure(opts) do
    GenServer.call(__MODULE__, {:configure, opts})
  end

  @doc "Current fleet health state."
  @spec status() :: map()
  def status do
    GenServer.call(__MODULE__, :status)
  end

  @doc "Force immediate health re-check."
  @spec check_now() :: :ok
  def check_now do
    send(__MODULE__, :check)
    :ok
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    interval_ms = Keyword.get(opts, :interval_ms, Config.fleet_health_check_interval_ms())
    {:ok, %__MODULE__{interval_ms: interval_ms}}
  end

  @impl true
  def handle_call({:configure, opts}, _from, state) do
    sprites = Keyword.get(opts, :sprites, [])
    repo = Keyword.fetch!(opts, :repo)
    label = Keyword.get(opts, :label)
    initial_healthy = Keyword.get(opts, :healthy, MapSet.new())

    known_health =
      Map.new(sprites, fn s ->
        {s.name, if(MapSet.member?(initial_healthy, s.name), do: :healthy, else: :unhealthy)}
      end)

    if state.timer_ref, do: Process.cancel_timer(state.timer_ref)
    ref = schedule_check(state.interval_ms)

    state = %{
      state
      | sprites: sprites,
        repo: repo,
        label: label,
        known_health: known_health,
        timer_ref: ref
    }

    {:reply, :ok, state}
  end

  @impl true
  def handle_call(:status, _from, state) do
    {:reply, %{sprites: state.known_health, repo: state.repo}, state}
  end

  @impl true
  def handle_info(:check, %{sprites: []} = state) do
    {:noreply, state}
  end

  @impl true
  def handle_info(:check, state) do
    state = check_and_recover(state)
    ref = schedule_check(state.interval_ms)
    {:noreply, %{state | timer_ref: ref}}
  end

  @impl true
  def handle_info(_msg, state), do: {:noreply, state}

  # --- Private ---

  defp check_and_recover(state) do
    # Probe ALL sprites — builders need recovery too when cold at boot
    phase_sprites = state.sprites

    Enum.reduce(phase_sprites, state, fn sprite, acc ->
      new_health = probe_sprite(sprite)
      old_health = Map.get(acc.known_health, sprite.name, :unhealthy)

      cond do
        old_health == :unhealthy and new_health == :healthy ->
          Logger.info("[health] #{sprite.name} recovered")

          updated = put_health(acc, sprite.name, :healthy)

          case sync_phase_worker(updated, sprite.role) do
            :ok ->
              Store.record_event("fleet", "sprite_recovered", %{
                name: sprite.name,
                role: to_string(sprite.role)
              })

              updated

            :error ->
              # Keep the sprite unhealthy so the next successful probe retries recovery sync.
              acc
          end

        old_health == :healthy and new_health == :unhealthy ->
          Logger.warning("[health] #{sprite.name} degraded")

          Store.record_event("fleet", "sprite_degraded", %{
            name: sprite.name,
            role: to_string(sprite.role)
          })

          updated = put_health(acc, sprite.name, :unhealthy)

          sync_phase_worker(updated, sprite.role)
          updated

        true ->
          acc
      end
    end)
  end

  defp probe_sprite(sprite) do
    case reconciler_mod().reconcile_sprite(sprite) do
      %{healthy: true} -> :healthy
      _ -> :unhealthy
    end
  end

  defp sync_phase_worker(state, :builder) do
    healthy_builders =
      state.sprites
      |> Enum.filter(fn s ->
        s.role == :builder and Map.get(state.known_health, s.name) == :healthy
      end)

    if healthy_builders == [] do
      :ok
    else
      try do
        orchestrator_mod().configure_polling(
          repo: state.repo,
          workers: healthy_builders,
          label: state.label
        )

        Logger.info(
          "[health] orchestrator configured with #{length(healthy_builders)} builder(s)"
        )

        :ok
      catch
        :exit, _ ->
          Logger.warning("[health] orchestrator unavailable, cannot configure builders")
          :error
      end
    end
  end

  defp sync_phase_worker(state, role) do
    case Roles.by_role(role) do
      nil ->
        :ok

      role_module ->
        sprites = healthy_sprites_for_role(state, role)

        case phase_worker_supervisor().ensure_worker(role_module, state.repo, sprites) do
          :ok ->
            :ok

          {:error, reason} ->
            Logger.warning("[health] failed to sync #{inspect(role_module)}: #{inspect(reason)}")
            :error
        end
    end
  end

  defp healthy_sprites_for_role(state, role) do
    state.sprites
    |> Enum.filter(fn sprite ->
      sprite.role == role and Map.get(state.known_health, sprite.name) == :healthy
    end)
    |> Enum.map(& &1.name)
    |> Enum.sort()
  end

  defp put_health(state, name, health) do
    %{state | known_health: Map.put(state.known_health, name, health)}
  end

  defp schedule_check(interval_ms) when is_integer(interval_ms) do
    Process.send_after(self(), :check, interval_ms)
  end

  defp reconciler_mod do
    Application.get_env(:conductor, :reconciler_module, Reconciler)
  end

  defp phase_worker_supervisor do
    Application.get_env(
      :conductor,
      :phase_worker_supervisor,
      Conductor.PhaseWorker.Supervisor
    )
  end

  defp orchestrator_mod do
    Application.get_env(:conductor, :orchestrator_module, Conductor.Orchestrator)
  end
end
