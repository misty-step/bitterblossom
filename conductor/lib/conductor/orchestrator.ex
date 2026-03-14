defmodule Conductor.Orchestrator do
  @moduledoc """
  Main polling loop. Symphony-inspired single authority.

  In `run_once` mode: starts one RunServer and waits for it.
  In `loop` mode: polls for eligible issues and starts RunServers up to concurrency limit.
  Reconciles stale runs on every tick.

  ## Fleet Management

  The orchestrator maintains a fleet of declared worker sprites. Before each
  dispatch, it probes the chosen sprite with `wake/1` to auto-wake sleeping
  machines (Fly.io suspends idle VMs). Consecutive probe failures drain a
  worker from the round-robin pool; a successful probe auto-recovers it.

  Fleet state per worker:
    - `name`     — sprite name
    - `tags`     — capability tags (reserved for future routing, not used yet)
    - `health`   — `:healthy | :drained`
    - `failures` — consecutive probe failure count
  """

  use GenServer
  require Logger

  alias Conductor.{Store, Config, Issue}

  defstruct [
    :repo,
    :label,
    :trusted_surfaces,
    :wake_fn,
    mode: :idle,
    active_runs: %{},
    fleet: [],
    worker_index: 0
  ]

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @doc "Run a single issue synchronously. Returns the terminal phase."
  @spec run_once(keyword()) :: {:ok, atom()} | {:error, term()}
  def run_once(opts) do
    repo = Keyword.fetch!(opts, :repo)
    issue_number = Keyword.fetch!(opts, :issue)
    worker = Keyword.fetch!(opts, :worker)
    trusted_surfaces = Keyword.get(opts, :trusted_surfaces, [])
    wake_fn = Keyword.get(opts, :wake_fn, &Conductor.Sprite.wake/1)

    case tracker_mod().get_issue(repo, issue_number) do
      {:ok, issue} ->
        case Issue.ready?(issue) do
          :ok ->
            # Auto-wake the sprite before dispatching
            case wake_fn.(worker) do
              :ok ->
                :ok

              {:error, msg} ->
                Logger.warning("wake probe failed for #{worker}: #{msg}, proceeding anyway")
            end

            run_issue(repo, issue, worker, trusted_surfaces)

          {:error, failures} ->
            IO.puts("issue ##{issue_number} not ready: #{Enum.join(failures, ", ")}")
            {:error, :not_ready}
        end

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc "Start the continuous polling loop."
  @spec start_loop(keyword()) :: :ok | {:error, :no_workers}
  def start_loop(opts) do
    GenServer.call(__MODULE__, {:start_loop, opts})
  end

  @doc "Return current fleet state for all declared workers."
  @spec fleet_status() :: {:ok, [map()]} | {:error, :not_running}
  def fleet_status do
    case GenServer.whereis(__MODULE__) do
      nil -> {:error, :not_running}
      _pid -> {:ok, GenServer.call(__MODULE__, :fleet_status)}
    end
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    {:ok,
     %__MODULE__{
       repo: Keyword.get(opts, :repo),
       label: Keyword.get(opts, :label, "autopilot"),
       fleet: init_fleet(Keyword.get(opts, :workers, [])),
       trusted_surfaces: Keyword.get(opts, :trusted_surfaces, []),
       wake_fn: Keyword.get(opts, :wake_fn, &Conductor.Sprite.wake/1)
     }}
  end

  @impl true
  def handle_call({:start_loop, opts}, _from, state) do
    workers = Keyword.fetch!(opts, :workers)

    if workers == [] do
      {:reply, {:error, :no_workers}, state}
    else
      wake_fn = Keyword.get(opts, :wake_fn, state.wake_fn || (&Conductor.Sprite.wake/1))

      state = %{
        state
        | repo: Keyword.fetch!(opts, :repo),
          label: Keyword.get(opts, :label, state.label),
          fleet: init_fleet(workers),
          trusted_surfaces: Keyword.get(opts, :trusted_surfaces, state.trusted_surfaces),
          wake_fn: wake_fn,
          mode: :polling
      }

      schedule_poll(0)
      {:reply, :ok, state}
    end
  end

  @impl true
  def handle_call(:fleet_status, _from, state) do
    {:reply, state.fleet, state}
  end

  @impl true
  def handle_info(:poll, %{mode: :polling} = state) do
    state = reconcile(state)
    state = maybe_start_runs(state)
    schedule_poll(Config.poll_seconds() * 1_000)
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, _reason}, state) do
    # A RunServer died. Remove it from active runs.
    active =
      state.active_runs
      |> Enum.reject(fn {_id, %{ref: r}} -> r == ref end)
      |> Map.new()

    {:noreply, %{state | active_runs: active}}
  end

  @impl true
  def handle_info(_msg, state) do
    {:noreply, state}
  end

  # --- Private ---

  defp run_issue(repo, issue, worker, trusted_surfaces) do
    opts = [
      repo: repo,
      issue: issue,
      worker: worker,
      trusted_surfaces: trusted_surfaces
    ]

    case DynamicSupervisor.start_child(
           Conductor.RunSupervisor,
           {Conductor.RunServer, opts}
         ) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        timeout = (Config.builder_timeout() + Config.ci_timeout() + 10) * 60_000

        receive do
          {:DOWN, ^ref, :process, ^pid, _reason} ->
            case find_latest_run(repo, issue.number) do
              %{"phase" => phase} -> {:ok, String.to_existing_atom(phase)}
              nil -> {:error, :run_not_found}
            end
        after
          timeout ->
            Process.exit(pid, :kill)
            {:error, :timeout}
        end

      {:error, reason} ->
        {:error, {:start_failed, reason}}
    end
  end

  defp maybe_start_runs(state) do
    max = Config.max_concurrent_runs()
    active_count = map_size(state.active_runs)

    if active_count >= max do
      state
    else
      slots = max - active_count

      eligible = tracker_mod().list_eligible(state.repo, label: state.label)
      unleased = Enum.reject(eligible, &Store.leased?(state.repo, &1.number))

      unleased
      |> Enum.take(slots)
      |> Enum.reduce(state, fn issue, acc -> start_run(acc, issue) end)
    end
  end

  defp start_run(state, issue) do
    case pick_fleet_worker(state) do
      nil ->
        Logger.warning("no healthy workers available for issue ##{issue.number}")
        state

      fleet_worker ->
        # Always advance the index so the next pick tries a different worker
        state = %{state | worker_index: state.worker_index + 1}

        # Auto-wake probe: confirms sprite is responsive, wakes sleeping VMs
        wake_result = state.wake_fn.(fleet_worker.name)
        {state, worker_ok} = update_fleet_health(state, fleet_worker.name, wake_result)

        if worker_ok do
          do_start_run(state, issue, fleet_worker.name)
        else
          state
        end
    end
  end

  defp do_start_run(state, issue, worker_name) do
    opts = [
      repo: state.repo,
      issue: issue,
      worker: worker_name,
      trusted_surfaces: state.trusted_surfaces
    ]

    case DynamicSupervisor.start_child(Conductor.RunSupervisor, {Conductor.RunServer, opts}) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        run_entry = %{pid: pid, ref: ref, issue: issue.number, worker: worker_name}
        Logger.info("started run for issue ##{issue.number} on #{worker_name}")

        %{state | active_runs: Map.put(state.active_runs, issue.number, run_entry)}

      {:error, reason} ->
        Logger.error("failed to start run for issue ##{issue.number}: #{inspect(reason)}")
        state
    end
  end

  # Return the next healthy worker by round-robin, or nil if none are healthy.
  defp pick_fleet_worker(%{fleet: fleet, worker_index: idx}) do
    healthy = Enum.filter(fleet, &(&1.health == :healthy))

    case healthy do
      [] -> nil
      workers -> Enum.at(workers, rem(idx, length(workers)))
    end
  end

  # Update fleet health based on wake probe result.
  # Returns `{new_state, worker_ok?}` where `worker_ok?` is true if dispatch should proceed.
  defp update_fleet_health(state, name, :ok) do
    fleet =
      Enum.map(state.fleet, fn
        %{name: ^name} = w -> %{w | failures: 0, health: :healthy}
        w -> w
      end)

    {%{state | fleet: fleet}, true}
  end

  defp update_fleet_health(state, name, {:error, msg}) do
    threshold = Config.probe_failure_threshold()

    fleet =
      Enum.map(state.fleet, fn
        %{name: ^name} = w ->
          failures = w.failures + 1
          health = if failures >= threshold, do: :drained, else: :healthy

          if health == :drained do
            Logger.warning(
              "worker #{name} drained after #{failures} consecutive probe failure(s): #{msg}"
            )
          else
            Logger.warning("worker #{name} probe failed (#{failures}/#{threshold}): #{msg}")
          end

          %{w | failures: failures, health: health}

        w ->
          w
      end)

    {%{state | fleet: fleet}, false}
  end

  defp reconcile(state) do
    # Remove active runs whose processes have died
    active =
      state.active_runs
      |> Enum.filter(fn {_id, %{pid: pid}} -> Process.alive?(pid) end)
      |> Map.new()

    %{state | active_runs: active}
  end

  defp find_latest_run(repo, issue_number) do
    Store.list_runs(limit: 50)
    |> Enum.find(fn r ->
      r["repo"] == repo and r["issue_number"] == issue_number
    end)
  end

  # Normalize raw worker declarations into fleet worker maps.
  defp init_fleet(workers) do
    Enum.map(workers, fn
      name when is_binary(name) ->
        %{name: name, tags: [], health: :healthy, failures: 0}

      %{name: name} = w ->
        %{name: name, tags: Map.get(w, :tags, []), health: :healthy, failures: 0}

      %{"name" => name} = w ->
        %{name: name, tags: Map.get(w, "tags", []), health: :healthy, failures: 0}
    end)
  end

  defp tracker_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)

  defp schedule_poll(delay) do
    Process.send_after(self(), :poll, delay)
  end
end
