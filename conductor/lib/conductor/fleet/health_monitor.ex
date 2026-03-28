defmodule Conductor.Fleet.HealthMonitor do
  @moduledoc """
  Periodic fleet health monitor. Probes sprites and tracks recovery/degradation.

  When a sprite recovers, logs the event and re-launches its agent loop.
  When a sprite degrades, logs the event.

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

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec configure(keyword()) :: :ok
  def configure(opts) do
    GenServer.call(__MODULE__, {:configure, opts})
  end

  @spec status() :: map()
  def status do
    GenServer.call(__MODULE__, :status)
  end

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
  def handle_info(:check, %{sprites: []} = state), do: {:noreply, state}

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
    Enum.reduce(state.sprites, state, fn sprite, acc ->
      new_health = probe_sprite(sprite)
      old_health = Map.get(acc.known_health, sprite.name, :unhealthy)

      cond do
        old_health == :unhealthy and new_health == :healthy ->
          Logger.info("[health] #{sprite.name} recovered")

          Store.record_event("fleet", "sprite_recovered", %{
            name: sprite.name,
            role: to_string(sprite.role)
          })

          # Re-launch the agent loop for the recovered sprite
          if sprite_repo(sprite, acc.repo) do
            Task.Supervisor.start_child(Conductor.TaskSupervisor, fn ->
              launcher_mod().launch(sprite, sprite_repo(sprite, acc.repo))
            end)
          end

          put_health(acc, sprite.name, :healthy)

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

  defp put_health(state, name, health) do
    %{state | known_health: Map.put(state.known_health, name, health)}
  end

  defp schedule_check(interval_ms) when is_integer(interval_ms) do
    Process.send_after(self(), :check, interval_ms)
  end

  defp sprite_repo(sprite, fallback_repo), do: Map.get(sprite, :repo, fallback_repo)

  defp launcher_mod do
    Application.get_env(:conductor, :launcher_module, Conductor.Launcher)
  end

  defp reconciler_mod do
    Application.get_env(:conductor, :reconciler_module, Reconciler)
  end
end
