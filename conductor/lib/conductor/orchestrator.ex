defmodule Conductor.Orchestrator do
  @moduledoc """
  Main polling loop. Symphony-inspired single authority.

  In `run_once` mode: starts one RunServer and waits for it.
  In `loop` mode: polls for eligible issues and starts RunServers up to concurrency limit.
  Reconciles stale runs on every tick.
  """

  use GenServer
  require Logger

  alias Conductor.{Store, Config, Issue, Workspace}

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

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    {:ok,
     %__MODULE__{
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
      state = %{
        state
        | repo: Keyword.fetch!(opts, :repo),
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
    merge_labeled_prs(state)
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
    worker = pick_worker(state)

    if worker_mod().busy?(worker) do
      Logger.info("worker #{worker} busy, deferring issue ##{issue.number}")
      state
    else
      dispatch_run(state, issue, worker)
    end
  end

  defp dispatch_run(state, issue, worker) do
    existing_pr =
      case code_host_mod().find_open_pr(state.repo, issue.number) do
        {:ok, pr} ->
          Logger.info(
            "found existing PR ##{pr["number"]} for issue ##{issue.number}, adopting branch #{pr["headRefName"]}"
          )

          pr

        {:error, _} ->
          nil
      end

    opts =
      [
        repo: state.repo,
        issue: issue,
        worker: worker,
        trusted_surfaces: state.trusted_surfaces
      ] ++ adoption_opts(existing_pr)

    case DynamicSupervisor.start_child(Conductor.RunSupervisor, {Conductor.RunServer, opts}) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        run_entry = %{pid: pid, ref: ref, issue: issue.number, worker: worker}

        Logger.info("started run for issue ##{issue.number} on #{worker}")

        %{
          state
          | active_runs: Map.put(state.active_runs, issue.number, run_entry),
            worker_index: state.worker_index + 1
        }

      {:error, reason} ->
        Logger.error("failed to start run for issue ##{issue.number}: #{inspect(reason)}")
        state
    end
  end

  defp adoption_opts(nil), do: []

  defp adoption_opts(pr) do
    [
      existing_branch: pr["headRefName"],
      existing_pr_number: pr["number"],
      existing_pr_url: pr["url"]
    ]
  end

  defp pick_worker(%{workers: []}), do: raise("no workers configured")

  defp pick_worker(%{workers: workers, worker_index: idx}) do
    Enum.at(workers, rem(idx, length(workers)))
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

  # --- Label-Driven Merge ---

  defp merge_labeled_prs(%{repo: nil}), do: :ok

  defp merge_labeled_prs(%{repo: repo, workers: workers}) do
    case code_host_mod().labeled_prs(repo, "lgtm") do
      {:ok, prs} ->
        prs
        |> Enum.filter(fn pr ->
          # Only merge PRs from conductor branches (factory/*)
          branch = pr["headRefName"] || ""
          String.starts_with?(branch, "factory/")
        end)
        |> Enum.each(fn pr ->
          pr_number = pr["number"]

          if code_host_mod().checks_green?(repo, pr_number) do
            Logger.info("[merge] PR ##{pr_number} has lgtm + green CI, merging")

            case code_host_mod().merge(repo, pr_number, []) do
              :ok ->
                Logger.info("[merge] PR ##{pr_number} merged successfully")
                record_merge(repo, pr_number)
                Conductor.SelfUpdate.maybe_reload(repo, pr_number)

              {:error, reason} ->
                if merge_conflict?(reason) do
                  attempt_rebase_merge(repo, pr_number, pr["headRefName"], workers)
                else
                  Logger.warning("[merge] PR ##{pr_number} merge failed: #{reason}")
                end
            end
          else
            Logger.debug("[merge] PR ##{pr_number} has lgtm but CI not green, skipping")
          end
        end)

      {:error, reason} ->
        Logger.warning("[merge] failed to check labeled PRs: #{reason}")
    end
  end

  # Returns true when a merge error message indicates a git conflict rather than
  # a policy or infrastructure failure.
  @doc false
  def merge_conflict?(reason) do
    msg = to_string(reason)
    String.contains?(msg, "not mergeable") or String.contains?(msg, "cannot be cleanly created")
  end

  @doc false
  def mark_conflict_blocked(repo, pr_number) do
    Logger.warning("[merge] PR ##{pr_number} blocked: merge_conflict_unresolvable")

    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"run_id" => run_id}} ->
        Store.record_event(run_id, "merge_conflict_blocked", %{
          pr_number: pr_number,
          reason: "merge_conflict_unresolvable"
        })

        Store.complete_run(run_id, "blocked", "blocked")

      _ ->
        Logger.debug("[merge] no run found for PR ##{pr_number}, cannot mark blocked")
    end
  end

  defp attempt_rebase_merge(_repo, pr_number, _branch, []) do
    Logger.warning("[merge] no workers available to rebase PR ##{pr_number}")
  end

  defp attempt_rebase_merge(repo, pr_number, branch, [worker | _]) do
    Logger.info(
      "[merge] PR ##{pr_number} has merge conflict, rebasing branch #{branch} on #{worker}"
    )

    case Workspace.rebase(worker, repo, branch) do
      :ok ->
        Logger.info("[merge] rebase succeeded, retrying merge for PR ##{pr_number}")

        case code_host_mod().merge(repo, pr_number, []) do
          :ok ->
            Logger.info("[merge] PR ##{pr_number} merged after rebase")
            record_merge(repo, pr_number)
            Conductor.SelfUpdate.maybe_reload(repo, pr_number)

          {:error, retry_reason} ->
            if merge_conflict?(retry_reason) do
              Logger.warning("[merge] PR ##{pr_number} still has conflict after rebase")
              mark_conflict_blocked(repo, pr_number)
            else
              # Transient/policy failure after rebase — don't mark blocked,
              # let normal retry pick it up on the next poll cycle.
              Logger.warning("[merge] PR ##{pr_number} post-rebase merge failed (non-conflict): #{retry_reason}")
            end
        end

      {:error, reason} ->
        Logger.warning("[merge] rebase failed for PR ##{pr_number}: #{reason}")
        mark_conflict_blocked(repo, pr_number)
    end
  end

  # Update the Store when the orchestrator merges a PR (not the RunServer).
  defp record_merge(repo, pr_number) do
    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"run_id" => run_id}} ->
        Store.record_event(run_id, "merged", %{pr_number: pr_number, merged_by: "orchestrator"})
        Store.complete_run(run_id, "merged", "merged")
        Conductor.Retro.analyze(run_id)

      _ ->
        Logger.debug("[merge] no run found for PR ##{pr_number}, skipping store update")
    end
  end

  defp tracker_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)

  defp schedule_poll(delay) do
    Process.send_after(self(), :poll, delay)
  end
end
