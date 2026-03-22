defmodule Conductor.Fleet.HealthMonitor do
  @moduledoc """
  Periodic fleet health re-check. Detects sprite recovery and auto-starts
  phase workers (Fixer, Polisher) that were skipped at boot due to unhealthy sprites.

  Deep module: hides all sprite lifecycle recovery behind a simple status/0 interface.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Store}
  alias Conductor.Fleet.Reconciler

  defstruct [
    :repo,
    :interval_ms,
    :timer_ref,
    sprites: [],
    known_health: %{}
  ]

  @role_to_module %{
    fixer: {Conductor.Fixer, :fixer_sprite},
    polisher: {Conductor.Polisher, :polisher_sprite},
    triage: {Conductor.Muse, :muse_sprite}
  }

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

  @doc "Role-to-module mapping for phase workers."
  @spec role_to_module() :: map()
  def role_to_module, do: @role_to_module

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
    initial_healthy = Keyword.get(opts, :healthy, MapSet.new())

    known_health =
      Map.new(sprites, fn s ->
        {s.name, if(MapSet.member?(initial_healthy, s.name), do: :healthy, else: :unhealthy)}
      end)

    if state.timer_ref, do: Process.cancel_timer(state.timer_ref)
    ref = schedule_check(state.interval_ms)
    state = %{state | sprites: sprites, repo: repo, known_health: known_health, timer_ref: ref}
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
    # Only re-probe non-builder sprites (builders are probed by the Orchestrator)
    phase_sprites = Enum.filter(state.sprites, &(&1.role in [:fixer, :polisher, :triage]))

    Enum.reduce(phase_sprites, state, fn sprite, acc ->
      new_health = probe_sprite(sprite)
      old_health = Map.get(acc.known_health, sprite.name, :unhealthy)

      cond do
        old_health == :unhealthy and new_health == :healthy ->
          Logger.info("[health] #{sprite.name} recovered, starting phase worker")

          case ensure_phase_worker(sprite, acc.repo) do
            :ok ->
              Store.record_event("fleet", "sprite_recovered", %{
                name: sprite.name,
                role: to_string(sprite.role)
              })

              put_health(acc, sprite.name, :healthy)

            :error ->
              acc
          end

        old_health == :healthy and new_health == :unhealthy ->
          Logger.warning("[health] #{sprite.name} degraded")

          Store.record_event("fleet", "sprite_degraded", %{
            name: sprite.name,
            role: to_string(sprite.role)
          })

          put_health(acc, sprite.name, :unhealthy)

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

  @spec ensure_phase_worker(map(), binary()) :: :ok | :error
  defp ensure_phase_worker(sprite, repo) do
    case Map.get(@role_to_module, sprite.role) do
      nil ->
        :ok

      {module, sprite_key} ->
        if Process.whereis(module) do
          :ok
        else
          try do
            case Supervisor.start_child(
                   supervisor_name(),
                   {module, [{:repo, repo}, {sprite_key, sprite.name}]}
                 ) do
              {:ok, _} ->
                Logger.info("[health] started #{inspect(module)} for #{sprite.name}")
                :ok

              {:error, {:already_started, _}} ->
                :ok

              {:error, reason} ->
                Logger.warning("[health] failed to start #{inspect(module)}: #{inspect(reason)}")
                :error
            end
          catch
            :exit, _ ->
              Logger.warning("[health] supervisor unavailable, cannot start #{inspect(module)}")
              :error
          end
        end
    end
  end

  defp supervisor_name do
    Application.get_env(:conductor, :supervisor_name, Conductor.Supervisor)
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
end
