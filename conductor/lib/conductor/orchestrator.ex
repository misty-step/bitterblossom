defmodule Conductor.Orchestrator do
  @moduledoc """
  Main polling loop. Symphony-inspired single authority.

  In `run_once` mode: starts one RunServer and waits for it.
  In `loop` mode: polls for eligible issues and starts RunServers up to concurrency limit.
  Reconciles stale runs on every tick.

  ## Fleet and health tracking

  Workers are probed before each dispatch. A worker that fails N consecutive probes
  (see `Config.max_probe_failures/0`, default 3) is marked drained and skipped.
  A drained worker is automatically recovered on the next successful probe — the
  conductor retries drained workers on every tick so they rejoin the pool once the
  sprite is reachable again.
  """

  use GenServer
  require Logger

  alias Conductor.{Store, Config, Issue}

  @health_default %{consecutive_failures: 0, drained: false}

  defstruct [
    :repo,
    :label,
    :workers,
    :trusted_surfaces,
    # Injected in tests to probe workers without real sprite calls.
    :probe_fn,
    # Injected in tests to avoid starting real RunServers.
    :dispatch_fn,
    mode: :idle,
    active_runs: %{},
    worker_index: 0,
    worker_health: %{}
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

    case tracker_mod().get_issue(repo, issue_number) do
      {:ok, issue} ->
        case Issue.ready?(issue) do
          :ok ->
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

  @doc """
  Returns health and assignment status for each declared worker.
  Only meaningful after `start_loop/1` has been called.
  """
  @spec fleet_status() :: [map()]
  def fleet_status do
    GenServer.call(__MODULE__, :fleet_status)
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    {:ok,
     %__MODULE__{
       repo: Keyword.get(opts, :repo),
       label: Keyword.get(opts, :label, "autopilot"),
       workers: Keyword.get(opts, :workers, []),
       trusted_surfaces: Keyword.get(opts, :trusted_surfaces, []),
       probe_fn: Keyword.get(opts, :probe_fn),
       dispatch_fn: Keyword.get(opts, :dispatch_fn)
     }}
  end

  @impl true
  def handle_call({:start_loop, opts}, _from, state) do
    workers = Keyword.fetch!(opts, :workers)

    if workers == [] do
      {:reply, {:error, :no_workers}, state}
    else
      state = %{
        state
        | repo: Keyword.fetch!(opts, :repo),
          label: Keyword.get(opts, :label, state.label),
          workers: workers,
          trusted_surfaces: Keyword.get(opts, :trusted_surfaces, state.trusted_surfaces),
          probe_fn: Keyword.get(opts, :probe_fn, state.probe_fn),
          dispatch_fn: Keyword.get(opts, :dispatch_fn, state.dispatch_fn),
          mode: :polling
      }

      schedule_poll(0)
      {:reply, :ok, state}
    end
  end

  @impl true
  def handle_call(:fleet_status, _from, state) do
    statuses =
      Enum.map(state.workers, fn worker ->
        health = Map.get(state.worker_health, worker, @health_default)

        active =
          Enum.count(state.active_runs, fn {_, entry} -> entry.worker == worker end)

        %{
          worker: worker,
          drained: health.drained,
          consecutive_failures: health.consecutive_failures,
          active_runs: active
        }
      end)

    {:reply, statuses, state}
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
    case pick_healthy_worker(state) do
      {nil, state} ->
        Logger.warning("no healthy workers for issue ##{issue.number}, skipping until next poll")
        state

      {worker, state} ->
        dispatch_run(state, issue, worker)
    end
  end

  defp dispatch_run(state, issue, worker) do
    run_opts = [
      repo: state.repo,
      issue: issue,
      worker: worker,
      trusted_surfaces: state.trusted_surfaces
    ]

    start_child =
      state.dispatch_fn ||
        fn opts ->
          DynamicSupervisor.start_child(Conductor.RunSupervisor, {Conductor.RunServer, opts})
        end

    case start_child.(run_opts) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        run_entry = %{pid: pid, ref: ref, issue: issue.number, worker: worker}
        Logger.info("started run for issue ##{issue.number} on #{worker}")
        %{state | active_runs: Map.put(state.active_runs, issue.number, run_entry)}

      {:error, reason} ->
        Logger.error("failed to start run for issue ##{issue.number}: #{inspect(reason)}")
        state
    end
  end

  @doc false
  # Pick the next healthy worker via round-robin, probing each candidate.
  # Returns `{worker, updated_state}` or `{nil, updated_state}` when all are drained.
  defp pick_healthy_worker(%{workers: []} = state) do
    {nil, state}
  end

  defp pick_healthy_worker(%{workers: workers, worker_index: idx} = state) do
    n = length(workers)

    result =
      Enum.reduce_while(0..(n - 1), state, fn offset, acc ->
        candidate = Enum.at(workers, rem(idx + offset, n))
        health = Map.get(acc.worker_health, candidate, @health_default)

        case do_probe(acc, candidate) do
          :ok ->
            # Probe succeeded — clear failures (auto-recovery if was drained)
            if health.drained do
              Logger.info("worker #{candidate} auto-recovered after successful probe")
            end

            new_acc = clear_health(acc, candidate)
            {:halt, {:found, candidate, new_acc}}

          :error ->
            if health.drained do
              # Still drained, keep going
              {:cont, acc}
            else
              # Record failure; drain if threshold hit
              {:cont, record_failure(acc, candidate)}
            end
        end
      end)

    case result do
      {:found, worker, new_state} ->
        {worker, %{new_state | worker_index: idx + 1}}

      state ->
        {nil, state}
    end
  end

  defp do_probe(state, worker) do
    probe = state.probe_fn || (&Conductor.Sprite.wake/1)

    case probe.(worker) do
      :ok -> :ok
      _ -> :error
    end
  end

  defp clear_health(state, worker) do
    %{state | worker_health: Map.put(state.worker_health, worker, @health_default)}
  end

  defp record_failure(state, worker) do
    health = Map.get(state.worker_health, worker, @health_default)
    failures = health.consecutive_failures + 1
    max_fails = Config.max_probe_failures()
    drained = failures >= max_fails

    if drained do
      Logger.warning("worker #{worker} drained after #{failures} consecutive probe failures")
    end

    new_health = %{consecutive_failures: failures, drained: drained}
    %{state | worker_health: Map.put(state.worker_health, worker, new_health)}
  end

  defp reconcile(state) do
    # 1. Remove in-memory entries for dead processes
    active =
      state.active_runs
      |> Enum.filter(fn {_id, %{pid: pid}} -> Process.alive?(pid) end)
      |> Map.new()

    state = %{state | active_runs: active}

    # 2. Detect and expire stale runs from the Store (covers restarts and orphans)
    expire_stale_runs(state)
  end

  defp expire_stale_runs(%{repo: nil} = state), do: state

  defp expire_stale_runs(state) do
    threshold = Config.stale_run_threshold_minutes()
    cutoff = DateTime.add(DateTime.utc_now(), -threshold * 60, :second)

    state.repo
    |> Store.list_active_runs()
    |> Enum.reject(fn run -> Map.has_key?(state.active_runs, run["issue_number"]) end)
    |> Enum.filter(fn run -> stale_heartbeat?(run["heartbeat_at"], cutoff) end)
    |> Enum.each(fn run ->
      run_id = run["run_id"]
      issue_number = run["issue_number"]

      Logger.warning("[reconcile] stale run #{run_id} (issue ##{issue_number}), expiring lease")
      Store.expire_stale_run(state.repo, run_id, issue_number, run["heartbeat_at"])
    end)

    state
  end

  defp stale_heartbeat?(nil, _cutoff), do: true

  defp stale_heartbeat?(heartbeat_str, cutoff) do
    case DateTime.from_iso8601(heartbeat_str) do
      {:ok, dt, _} -> DateTime.compare(dt, cutoff) == :lt
      _ -> true
    end
  end

  defp find_latest_run(repo, issue_number) do
    Store.list_runs(limit: 50)
    |> Enum.find(fn r ->
      r["repo"] == repo and r["issue_number"] == issue_number
    end)
  end

  defp tracker_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)

  defp schedule_poll(delay) do
    Process.send_after(self(), :poll, delay)
  end
end
