defmodule Conductor.PhaseWorker do
  @moduledoc """
  Shared GenServer for phase roles like Thorn and Fern.

  One worker process owns one role and a pool of sprites for that role.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Store, Workspace}
  alias Conductor.PhaseWorker.Roles

  defstruct [
    :repo,
    :role_module,
    :poll_ms,
    :base_poll_ms,
    :timer_ref,
    :sprite_generation,
    sprites: [],
    in_flight: %{},
    ref_to_sprite: %{},
    failure_count: 0,
    health: :healthy
  ]

  @type task_info :: %{ref: reference(), run_id: binary() | nil, work_ref: pos_integer()}

  def start_link(opts) do
    role_module = Keyword.fetch!(opts, :role_module)
    GenServer.start_link(__MODULE__, opts, name: via_name(role_module))
  end

  def child_spec(opts) do
    role_module = Keyword.fetch!(opts, :role_module)

    %{
      id: {__MODULE__, role_module},
      start: {__MODULE__, :start_link, [opts]}
    }
  end

  @spec status(atom() | module()) :: map()
  def status(role_module) do
    GenServer.call(via_name(role_module), :status)
  end

  @spec statuses() :: [map()]
  def statuses do
    Roles.all()
    |> Enum.filter(&whereis/1)
    |> Enum.map(&status/1)
  end

  @spec update_sprites(atom() | module(), [binary()], integer() | nil) :: :ok
  def update_sprites(role_module, sprites, generation \\ nil) do
    normalized = Enum.uniq(sprites) |> Enum.sort()
    GenServer.call(via_name(role_module), {:update_sprites, normalized, generation})
  end

  @spec whereis(atom() | module()) :: pid() | nil
  def whereis(role_module) do
    role_key = role_key(role_module)

    case Registry.lookup(registry_name(), role_key) do
      [{pid, _value}] when is_pid(pid) ->
        if Process.alive?(pid), do: pid, else: nil

      [] ->
        nil
    end
  end

  @impl true
  def init(opts) do
    repo = Keyword.fetch!(opts, :repo)
    role_module = Roles.fetch!(Keyword.fetch!(opts, :role_module))
    poll_ms = Keyword.get(opts, :poll_ms, Config.poll_seconds() * 1_000)
    provided_sprites = Keyword.get(opts, :sprites, []) |> Enum.uniq() |> Enum.sort()
    provided_generation = Keyword.get(opts, :sprite_generation, 0)
    stored_generation = stored_sprite_generation(provided_generation, role_module)

    sprites =
      if stored_generation > provided_generation do
        stored_sprites(provided_sprites, role_module)
      else
        provided_sprites
      end

    state = %__MODULE__{
      repo: repo,
      role_module: role_module,
      poll_ms: poll_ms,
      base_poll_ms: poll_ms,
      timer_ref: nil,
      sprite_generation: max(provided_generation, stored_generation),
      sprites: sprites
    }

    {:ok, reschedule_poll(state, 0)}
  end

  @impl true
  def handle_call(:status, _from, state) do
    {:reply, status_map(state), state}
  end

  @impl true
  def handle_call({:update_sprites, _sprites, generation}, _from, state)
      when is_integer(generation) and generation < state.sprite_generation do
    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:update_sprites, sprites, generation}, _from, state) do
    state =
      if is_integer(generation) do
        %{state | sprites: sprites, sprite_generation: generation}
      else
        %{state | sprites: sprites}
      end

    {:reply, :ok, state}
  end

  @impl true
  def handle_info(:poll, state) do
    state =
      state
      |> Map.put(:timer_ref, nil)
      |> poll_and_dispatch()
      |> reschedule_poll()

    {:noreply, state}
  end

  @impl true
  def handle_info({ref, result}, state) when is_reference(ref) do
    Process.demonitor(ref, [:flush])
    {:noreply, complete_task(state, ref, result)}
  end

  # A dispatch task that exits with :shutdown before replying still owns a sprite
  # slot; let it fall through the crash path so in-flight capacity is released.
  @impl true
  def handle_info({:DOWN, _ref, :process, _pid, :normal}, state) do
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, _reason}, state) do
    Logger.warning("[#{log_prefix(state)}] dispatch task crashed")
    {:noreply, complete_task(state, ref, {:error, "task_crashed", 1})}
  end

  @impl true
  def handle_info(_msg, state), do: {:noreply, state}

  defp poll_and_dispatch(%{sprites: []} = state), do: state

  defp poll_and_dispatch(state) do
    idle_sprites = idle_sprites(state)

    if idle_sprites == [] do
      state
    else
      case state.role_module.find_work(state.repo, code_host_mod()) do
        {:ok, work_items} ->
          dispatch_work_items(state, work_items, idle_sprites)

        {:error, reason} ->
          Logger.warning("[#{log_prefix(state)}] failed to list open PRs: #{reason}")
          state
      end
    end
  end

  defp dispatch_work_items(state, work_items, idle_sprites) do
    reserved_refs =
      state.in_flight
      |> Map.values()
      |> Enum.reduce(MapSet.new(), fn %{work_ref: work_ref}, acc -> MapSet.put(acc, work_ref) end)

    {state, _idle_sprites, _reserved_refs} =
      Enum.reduce_while(work_items, {state, idle_sprites, reserved_refs}, fn work_item,
                                                                             {acc_state, acc_idle,
                                                                              acc_refs} ->
        case acc_idle do
          [] ->
            {:halt, {acc_state, [], acc_refs}}

          [sprite | rest] ->
            work_ref = acc_state.role_module.work_ref(work_item)

            cond do
              MapSet.member?(acc_refs, work_ref) ->
                {:cont, {acc_state, acc_idle, acc_refs}}

              not acc_state.role_module.eligible?(work_item, status_map(acc_state)) ->
                {:cont, {acc_state, acc_idle, acc_refs}}

              true ->
                updated_state = dispatch_work(acc_state, sprite, work_item)
                {:cont, {updated_state, rest, MapSet.put(acc_refs, work_ref)}}
            end
        end
      end)

    state
  end

  defp dispatch_work(state, sprite, work_item) do
    role_module = state.role_module
    work_ref = role_module.work_ref(work_item)
    run_id = run_id_for_work(state.repo, work_ref)
    workspace = workspace_for_branch(state.repo, work_item["headRefName"])
    context = role_module.enrich_context(work_item, state.repo, code_host_mod())
    prompt = role_module.build_prompt(work_item, context, workspace_root: workspace)

    Logger.info("[#{log_prefix(state)}] #{role_module.dispatch_log_message(work_item)}")

    record_task_event(run_id, "#{role_module.event_prefix()}_dispatched", %{
      pr_number: work_ref,
      sprite: sprite
    })

    task =
      Task.Supervisor.async_nolink(task_supervisor_name(), fn ->
        try do
          dispatch_opts =
            [
              workspace: workspace,
              persona_role: role_module.persona_role()
            ] ++ role_module.dispatch_opts(work_item)

          with :ok <- workspace_mod().sync_persona(sprite, workspace, role_module.persona_role()),
               {:ok, output} <- worker_mod().dispatch(sprite, prompt, state.repo, dispatch_opts) do
            {:ok, output}
          else
            {:error, msg, code} -> {:error, msg, code}
            {:error, reason} -> {:error, to_string(reason), 1}
          end
        rescue
          _exception ->
            Logger.warning("[#{log_prefix(state)}] dispatch handler raised an exception")

            {:error, "dispatch_crashed", 1}
        end
      end)

    task_info = %{ref: task.ref, run_id: run_id, work_ref: work_ref}

    %{
      state
      | in_flight: Map.put(state.in_flight, sprite, task_info),
        ref_to_sprite: Map.put(state.ref_to_sprite, task.ref, sprite)
    }
  end

  defp complete_task(state, ref, result) do
    {sprite, ref_to_sprite} = Map.pop(state.ref_to_sprite, ref)
    {task_info, in_flight} = pop_in_flight(state.in_flight, sprite)
    state = %{state | in_flight: in_flight, ref_to_sprite: ref_to_sprite}

    if task_info do
      work_ref = task_info.work_ref
      run_id = task_info.run_id
      event_prefix = state.role_module.event_prefix()

      case result do
        {:ok, _output} ->
          Logger.info("[#{log_prefix(state)}] completed work on PR ##{work_ref}")
          record_task_event(run_id, "#{event_prefix}_complete", %{pr_number: work_ref})
          reset_health(state)

        {:error, msg, _code} ->
          Logger.warning("[#{log_prefix(state)}] dispatch failed for PR ##{work_ref}: #{msg}")

          record_task_event(run_id, "#{event_prefix}_failed", %{
            pr_number: work_ref,
            error: msg,
            sprite: sprite
          })

          apply_backoff(state)

        other ->
          Logger.warning(
            "[#{log_prefix(state)}] unexpected result for PR ##{work_ref}: #{inspect(other)}"
          )

          apply_backoff(state)
      end
    else
      state
    end
  end

  defp idle_sprites(state) do
    busy = Map.keys(state.in_flight) |> MapSet.new()
    Enum.reject(state.sprites, &MapSet.member?(busy, &1))
  end

  defp pop_in_flight(in_flight, nil), do: {nil, in_flight}

  defp pop_in_flight(in_flight, sprite) do
    Map.pop(in_flight, sprite)
  end

  defp status_map(state) do
    %{
      repo: state.repo,
      role: state.role_module.role(),
      sprites: state.sprites,
      in_flight:
        Map.new(state.in_flight, fn {sprite, task_info} -> {sprite, task_info.work_ref} end),
      health: state.health,
      failure_count: state.failure_count
    }
  end

  defp apply_backoff(state) do
    count = state.failure_count + 1
    backoff_ms = min(trunc(state.base_poll_ms * :math.pow(2, count)), 600_000)
    health = if count >= 3, do: :unavailable, else: :degraded

    Logger.info(
      "[#{log_prefix(state)}] backoff: failures=#{count}, next_poll=#{backoff_ms}ms, health=#{health}"
    )

    state
    |> Map.merge(%{failure_count: count, poll_ms: backoff_ms, health: health})
    |> reschedule_poll()
  end

  defp reset_health(state) do
    if state.failure_count > 0 do
      Logger.info("[#{log_prefix(state)}] recovered, resetting to healthy")
    end

    state
    |> Map.merge(%{failure_count: 0, poll_ms: state.base_poll_ms, health: :healthy})
    |> reschedule_poll()
  end

  defp workspace_for_branch(repo, _branch) do
    Workspace.repo_root(repo)
  end

  defp schedule_poll(delay) do
    Process.send_after(self(), :poll, delay)
  end

  defp reschedule_poll(state, delay \\ nil) do
    cancel_poll(state.timer_ref)
    %{state | timer_ref: schedule_poll(delay || state.poll_ms)}
  end

  defp cancel_poll(nil), do: :ok

  defp cancel_poll(ref) do
    Process.cancel_timer(ref)

    receive do
      :poll -> :ok
    after
      0 -> :ok
    end
  end

  defp log_prefix(state) do
    state.role_module.persona_role()
  end

  defp role_key(role_module) do
    Roles.fetch!(role_module).role()
  end

  defp via_name(role_module) do
    {:via, Registry, {registry_name(), role_key(role_module)}}
  end

  defp registry_name do
    Application.get_env(:conductor, :phase_worker_registry, Conductor.PhaseWorkerRegistry)
  end

  defp task_supervisor_name do
    Application.get_env(:conductor, :task_supervisor_name, Conductor.TaskSupervisor)
  end

  defp code_host_mod do
    Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  end

  defp worker_mod do
    Application.get_env(:conductor, :worker_module, Conductor.Sprite)
  end

  defp workspace_mod do
    Application.get_env(:conductor, :workspace_module, Workspace)
  end

  defp store_mod do
    Application.get_env(:conductor, :store_module, Store)
  end

  defp record_task_event(nil, _event_type, _payload), do: :ok

  defp record_task_event(run_id, event_type, payload) do
    store_mod().record_event(run_id, event_type, payload)
  end

  defp run_id_for_work(repo, work_ref) do
    case store_mod().find_run_by_pr(repo, work_ref) do
      {:ok, %{"run_id" => run_id}} when is_binary(run_id) ->
        run_id

      {:ok, %{run_id: run_id}} when is_binary(run_id) ->
        run_id

      {:ok, _run} ->
        Logger.warning("[phase-worker] tracked PR ##{work_ref} is missing run_id")
        nil

      {:error, :not_found} ->
        nil

      {:error, reason} ->
        Logger.warning(
          "[phase-worker] failed to look up run for PR ##{work_ref}: #{inspect(reason)}"
        )

        nil
    end
  rescue
    exception ->
      Logger.warning(
        "[phase-worker] failed to look up run for PR ##{work_ref}: #{Exception.message(exception)}"
      )

      nil
  catch
    :exit, reason ->
      Logger.warning(
        "[phase-worker] failed to look up run for PR ##{work_ref}: #{inspect(reason)}"
      )

      nil
  end

  defp stored_sprites(default, role_module) do
    supervisor =
      Application.get_env(
        :conductor,
        :phase_worker_supervisor,
        Conductor.PhaseWorker.Supervisor
      )

    if function_exported?(supervisor, :stored_sprites, 2) do
      supervisor.stored_sprites(role_module, default)
    else
      default
    end
  end

  defp stored_sprite_generation(default, role_module) do
    supervisor =
      Application.get_env(
        :conductor,
        :phase_worker_supervisor,
        Conductor.PhaseWorker.Supervisor
      )

    if function_exported?(supervisor, :stored_sprite_generation, 2) do
      supervisor.stored_sprite_generation(role_module, default)
    else
      default
    end
  end
end
