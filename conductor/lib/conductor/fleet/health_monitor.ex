defmodule Conductor.Fleet.HealthMonitor do
  @moduledoc """
  Periodic fleet health re-check. Detects sprite recovery and auto-starts
  phase workers (Fixer, Polisher) that were skipped at boot due to unhealthy sprites.

  Deep module: hides all sprite lifecycle recovery behind a simple status/0 interface.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Store, Time}
  alias Conductor.Fleet.Reconciler

  defstruct [
    :repo,
    :interval_ms,
    :timer_ref,
    :last_check_at,
    sprites: [],
    sprite_statuses: %{}
  ]

  @role_to_module %{
    fixer: {Conductor.Fixer, :fixer_sprite},
    polisher: {Conductor.Polisher, :polisher_sprite}
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

    sprite_statuses =
      Map.new(sprites, fn s ->
        {s.name,
         %{
           name: s.name,
           role: s.role,
           status: if(MapSet.member?(initial_healthy, s.name), do: :healthy, else: :degraded),
           last_probe_at: nil,
           consecutive_failures: 0
         }}
      end)

    if state.timer_ref, do: Process.cancel_timer(state.timer_ref)
    ref = schedule_check(state.interval_ms)

    state = %{
      state
      | sprites: sprites,
        repo: repo,
        sprite_statuses: sprite_statuses,
        timer_ref: ref,
        last_check_at: nil
    }

    {:reply, :ok, state}
  end

  @impl true
  def handle_call(:status, _from, state) do
    {:reply,
     %{
       sprites: state.sprite_statuses,
       repo: state.repo,
       interval_ms: state.interval_ms,
       last_check_at: state.last_check_at
     }, state}
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
    checked_at = Time.now_utc()

    state =
      Enum.reduce(state.sprites, state, fn sprite, acc ->
        previous = Map.get(acc.sprite_statuses, sprite.name, default_sprite_status(sprite))
        next_failure_count = previous.consecutive_failures + 1
        new_status = probe_sprite(sprite, next_failure_count)

        updated =
          %{
            previous
            | role: sprite.role,
              status: new_status,
              last_probe_at: checked_at,
              consecutive_failures: if(new_status == :healthy, do: 0, else: next_failure_count)
          }

        cond do
          recovered?(previous.status, new_status) ->
            Logger.info(
              "[health] #{sprite.name} recovered#{maybe_worker_start_note(sprite.role)}"
            )

            acc = put_sprite_status(acc, sprite.name, updated)

            case maybe_start_phase_worker(sprite, acc.repo) do
              :ok ->
                Store.record_event("fleet", "sprite_recovered", %{
                  name: sprite.name,
                  role: to_string(sprite.role)
                })

                acc

              :error ->
                acc
            end

          healthy?(previous.status) and not healthy?(new_status) ->
            Logger.warning("[health] #{sprite.name} degraded")

            Store.record_event("fleet", "sprite_degraded", %{
              name: sprite.name,
              role: to_string(sprite.role)
            })

            put_sprite_status(acc, sprite.name, updated)

          true ->
            put_sprite_status(acc, sprite.name, updated)
        end
      end)

    %{state | last_check_at: checked_at}
  end

  defp probe_sprite(%{role: :builder} = sprite, failure_count) do
    harness = Map.get(sprite, :harness) || Map.get(sprite, "harness")

    result =
      cond do
        function_exported?(sprite_mod(), :status, 2) ->
          sprite_mod().status(sprite.name, harness: harness)

        function_exported?(sprite_mod(), :status, 1) ->
          sprite_mod().status(sprite.name)

        true ->
          {:error, :unsupported_status_probe}
      end

    case result do
      {:ok, %{healthy: true}} -> :healthy
      _ -> failure_status(failure_count)
    end
  end

  defp probe_sprite(sprite, failure_count) do
    case reconciler_mod().reconcile_sprite(sprite) do
      %{healthy: true} ->
        :healthy

      _ ->
        failure_status(failure_count)
    end
  end

  @spec maybe_start_phase_worker(map(), binary()) :: :ok | :error
  defp maybe_start_phase_worker(%{role: role}, _repo) when role not in [:fixer, :polisher],
    do: :ok

  defp maybe_start_phase_worker(sprite, repo) do
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

  defp put_sprite_status(state, name, sprite_status) do
    %{state | sprite_statuses: Map.put(state.sprite_statuses, name, sprite_status)}
  end

  defp schedule_check(interval_ms) when is_integer(interval_ms) do
    Process.send_after(self(), :check, interval_ms)
  end

  defp reconciler_mod do
    Application.get_env(:conductor, :reconciler_module, Reconciler)
  end

  defp sprite_mod do
    Application.get_env(:conductor, :sprite_module, Conductor.Sprite)
  end

  defp default_sprite_status(sprite) do
    %{
      name: sprite.name,
      role: sprite.role,
      status: :unknown,
      last_probe_at: nil,
      consecutive_failures: 0
    }
  end

  defp healthy?(:healthy), do: true
  defp healthy?(_), do: false

  defp recovered?(old_status, :healthy), do: old_status in [:degraded, :unavailable]
  defp recovered?(_old_status, _new_status), do: false

  defp maybe_worker_start_note(role) when role in [:fixer, :polisher],
    do: ", starting phase worker"

  defp maybe_worker_start_note(_role), do: ""

  defp failure_status(failure_count) do
    if failure_count >= Config.fleet_probe_failure_threshold(),
      do: :unavailable,
      else: :degraded
  end
end
