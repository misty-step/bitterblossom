defmodule Conductor.Orchestrator do
  @moduledoc """
  Main polling loop. Symphony-inspired single authority.

  In `run_once` mode: starts one RunServer and waits for it.
  In `loop` mode: polls for eligible issues and starts RunServers up to concurrency limit.
  Reconciles stale runs on every tick.
  """

  use GenServer
  require Logger

  alias Conductor.{Store, GitHub, Config, Issue}

  defstruct [
    :repo,
    :label,
    :workers,
    :trusted_surfaces,
    mode: :idle,
    active_runs: %{},
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

    case GitHub.get_issue(repo, issue_number) do
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

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    {:ok, %__MODULE__{
      repo: Keyword.get(opts, :repo),
      label: Keyword.get(opts, :label, "autopilot"),
      workers: Keyword.get(opts, :workers, []),
      trusted_surfaces: Keyword.get(opts, :trusted_surfaces, [])
    }}
  end

  @impl true
  def handle_call({:start_loop, opts}, _from, state) do
    workers = Keyword.fetch!(opts, :workers)

    if workers == [] do
      {:reply, {:error, :no_workers}, state}
    else
      state = %{state |
        repo: Keyword.fetch!(opts, :repo),
        label: Keyword.get(opts, :label, state.label),
        workers: workers,
        trusted_surfaces: Keyword.get(opts, :trusted_surfaces, state.trusted_surfaces),
        mode: :polling
      }

      schedule_poll(0)
      {:reply, :ok, state}
    end
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

      eligible = GitHub.eligible_issues(state.repo, label: state.label)
      unleased = Enum.reject(eligible, &Store.leased?(state.repo, &1.number))

      unleased
      |> Enum.take(slots)
      |> Enum.reduce(state, fn issue, acc -> start_run(acc, issue) end)
    end
  end

  defp start_run(state, issue) do
    worker = pick_worker(state)

    opts = [
      repo: state.repo,
      issue: issue,
      worker: worker,
      trusted_surfaces: state.trusted_surfaces
    ]

    case DynamicSupervisor.start_child(Conductor.RunSupervisor, {Conductor.RunServer, opts}) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        run_entry = %{pid: pid, ref: ref, issue: issue.number, worker: worker}

        Logger.info("started run for issue ##{issue.number} on #{worker}")

        %{state |
          active_runs: Map.put(state.active_runs, issue.number, run_entry),
          worker_index: state.worker_index + 1
        }

      {:error, reason} ->
        Logger.error("failed to start run for issue ##{issue.number}: #{inspect(reason)}")
        state
    end
  end

  defp pick_worker(%{workers: []}), do: raise("no workers configured")
  defp pick_worker(%{workers: workers, worker_index: idx}) do
    Enum.at(workers, rem(idx, length(workers)))
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

  defp schedule_poll(delay) do
    Process.send_after(self(), :poll, delay)
  end
end
