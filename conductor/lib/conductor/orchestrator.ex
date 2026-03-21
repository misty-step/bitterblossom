defmodule Conductor.Orchestrator do
  @moduledoc """
  Issue dispatch authority.

  `run_once/1` — start one RunServer synchronously and wait for it.
  `configure_polling/1` — poll for eligible issues and start RunServers up to concurrency limit.
  Reconciles stale runs and merges lgtm-labeled PRs on every tick.
  """

  use GenServer
  require Logger

  alias Conductor.{Store, Config, Issue, Workspace}

  defmodule RunLauncher do
    @moduledoc false

    @doc false
    def start(opts) do
      DynamicSupervisor.start_child(
        Conductor.RunSupervisor,
        {Conductor.RunServer, opts}
      )
    end
  end

  defstruct [
    :repo,
    :label,
    :worker_order,
    :workers,
    :trusted_surfaces,
    mode: :idle,
    active_runs: %{},
    shape_attempts: %{},
    shape_tasks: %{},
    worker_index: 0
  ]

  # --- Public API ---

  @doc "Start the orchestrator GenServer under the conductor supervision tree."
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
            case safe_shape_issue(repo, issue_number) do
              {:ok, result} when result in [:shaped, :already_shaped] ->
                IO.puts(
                  "issue ##{issue_number} not ready: #{Enum.join(failures, ", ")}; " <>
                    "shaping #{result} and deferring execution until the next fetch"
                )

              {:error, reason} ->
                IO.puts(
                  "issue ##{issue_number} not ready: #{Enum.join(failures, ", ")}; " <>
                    "shaping failed: #{inspect(reason)}"
                )
            end

            {:error, :not_ready}
        end

      {:error, reason} ->
        {:error, reason}
    end
  end

  @doc "Configure the orchestrator and begin polling."
  @spec configure_polling(keyword()) :: :ok | {:error, :no_workers}
  def configure_polling(opts) do
    GenServer.call(__MODULE__, {:configure_polling, opts})
  end

  @doc "Pause dispatch of new runs. Existing work continues."
  @spec pause() :: :ok
  def pause do
    GenServer.call(__MODULE__, :pause)
  end

  @doc "Resume dispatch of new runs."
  @spec resume() :: :ok
  def resume do
    GenServer.call(__MODULE__, :resume)
  end

  @doc "Return fleet worker status in round-robin order."
  @spec fleet_status() :: [map()]
  def fleet_status do
    GenServer.call(__MODULE__, :fleet_status)
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    # Required for terminate/2 to fire during supervisor shutdown.
    # Without this, the VM kills the process without calling terminate,
    # and sprites retain merge-capable credentials.
    Process.flag(:trap_exit, true)

    workers = normalize_workers(Keyword.get(opts, :workers, []))

    {:ok,
     %__MODULE__{
       repo: Keyword.get(opts, :repo),
       label: Keyword.get(opts, :label),
       workers: worker_map(workers),
       worker_order: Enum.map(workers, & &1.name),
       trusted_surfaces: Keyword.get(opts, :trusted_surfaces, [])
     }}
  end

  @impl true
  def handle_call({:configure_polling, opts}, _from, state) do
    workers = normalize_workers(Keyword.fetch!(opts, :workers))
    repo = Keyword.fetch!(opts, :repo)

    if workers == [] do
      {:reply, {:error, :no_workers}, state}
    else
      state = maybe_reset_shape_state(state, repo)
      shape_attempts = if repo == state.repo, do: state.shape_attempts, else: %{}

      state = %{
        state
        | repo: repo,
          label: Keyword.get(opts, :label),
          shape_attempts: shape_attempts,
          workers: worker_map(workers),
          worker_order: Enum.map(workers, & &1.name),
          trusted_surfaces: Keyword.get(opts, :trusted_surfaces, state.trusted_surfaces),
          mode: dispatch_mode()
      }

      maybe_warn_unfiltered_loop(state)
      schedule_poll(0)
      {:reply, :ok, state}
    end
  end

  @impl true
  def handle_call(:pause, _from, state) do
    Store.set_dispatch_paused(true)
    {:reply, :ok, pause_state(state)}
  end

  @impl true
  def handle_call(:resume, _from, state) do
    Store.set_dispatch_paused(false)

    state =
      if state.mode == :idle do
        schedule_poll(0)
        state
      else
        state
      end

    {:reply, :ok, state}
  end

  @impl true
  def handle_call(:fleet_status, _from, state) do
    {:reply, ordered_workers(state), state}
  end

  @impl true
  def terminate(_reason, state) do
    kill_fleet_agents(state)
    :ok
  end

  @impl true
  def handle_info(:poll, %{mode: :idle} = state) do
    {:noreply, state}
  end

  @impl true
  def handle_info(:poll, state) do
    self_update_mod().check_for_updates()
    state = reconcile(state)
    reconcile_held_leases(state)
    merge_labeled_prs(state)
    state = cancel_active_runs(state)

    state =
      if dispatch_paused?() do
        %{state | mode: :paused}
      else
        state
        |> Map.put(:mode, :polling)
        |> maybe_start_runs()
      end

    schedule_poll(Config.poll_seconds() * 1_000)
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, _reason}, state) do
    case Map.pop(state.shape_tasks, ref) do
      {nil, _shape_tasks} ->
        active =
          state.active_runs
          |> Enum.reject(fn {_id, %{ref: r}} -> r == ref end)
          |> Map.new()

        {:noreply, %{state | active_runs: active}}

      {_task_meta, shape_tasks} ->
        {:noreply, %{state | shape_tasks: shape_tasks}}
    end
  end

  @impl true
  def handle_info({ref, result}, state) when is_reference(ref) do
    case Map.fetch(state.shape_tasks, ref) do
      {:ok, task_meta} ->
        Process.demonitor(ref, [:flush])
        state = complete_shape_task(state, ref)
        log_shape_result(task_meta, result)

        if match?({:ok, result} when result in [:shaped, :already_shaped], result) do
          schedule_poll(0)
        end

        {:noreply, state}

      :error ->
        {:noreply, state}
    end
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
      workers: [worker],
      trusted_surfaces: trusted_surfaces
    ]

    case run_launcher().start(opts) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        timeout = (Config.builder_timeout() + Config.ci_timeout() + 10) * 60_000

        receive do
          {:DOWN, ^ref, :process, ^pid, _reason} ->
            case find_latest_run(repo, issue.number) do
              %{"phase" => phase} -> {:ok, String.to_atom(phase)}
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
    max_runs = Config.max_concurrent_runs()
    slots = min(max_runs - map_size(state.active_runs), Config.max_starts_per_tick())

    state.repo
    |> tracker_mod().list_eligible(label: state.label)
    |> Enum.reject(&Store.leased?(state.repo, &1.number))
    |> Enum.reject(&operator_blocked_issue?(state.repo, &1.number))
    |> Enum.reject(&issue_in_cooldown?(state.repo, &1.number))
    |> Enum.reduce({state, max(slots, 0)}, fn issue, {acc, remaining_slots} ->
      {next_state, outcome} = consider_issue(acc, issue, remaining_slots)

      slots_left =
        if outcome == :started, do: max(remaining_slots - 1, 0), else: remaining_slots

      {next_state, slots_left}
    end)
    |> elem(0)
  end

  defp consider_issue(state, issue, remaining_slots) do
    case Issue.ready?(issue) do
      :ok ->
        next_state =
          state
          |> clear_shape_attempt(issue.number)
          |> maybe_start_ready_issue(issue, remaining_slots)

        outcome =
          if map_size(next_state.active_runs) > map_size(state.active_runs),
            do: :started,
            else: :skipped

        {next_state, outcome}

      {:error, failures} ->
        maybe_shape_issue(state, issue, failures)
    end
  end

  defp maybe_shape_issue(state, issue, failures) do
    revision_id = Issue.revision_id(issue)

    cond do
      shape_in_flight?(state, issue.number) ->
        {state, :skipped}

      Map.get(state.shape_attempts, issue.number) == revision_id ->
        Logger.info("issue ##{issue.number} still unready after prior shaping attempt, skipping")
        {state, :skipped}

      true ->
        Logger.info(
          "issue ##{issue.number} not ready (#{Enum.join(failures, ", ")}); shaping asynchronously"
        )

        task =
          Task.Supervisor.async_nolink(task_supervisor(), fn ->
            safe_shape_issue(state.repo, issue.number)
          end)

        next_state =
          state
          |> put_shape_attempt(issue.number, revision_id)
          |> put_shape_task(task, state.repo, issue.number, revision_id, failures)

        {next_state, :shaped}
    end
  end

  defp clear_shape_attempt(state, issue_number) do
    %{state | shape_attempts: Map.delete(state.shape_attempts, issue_number)}
  end

  defp put_shape_attempt(state, issue_number, revision_id) do
    %{state | shape_attempts: Map.put(state.shape_attempts, issue_number, revision_id)}
  end

  defp put_shape_task(state, %Task{} = task, repo, issue_number, revision_id, failures) do
    task_meta = %{
      task: task,
      repo: repo,
      issue_number: issue_number,
      revision_id: revision_id,
      failures: failures
    }

    %{state | shape_tasks: Map.put(state.shape_tasks, task.ref, task_meta)}
  end

  defp complete_shape_task(state, ref) do
    %{state | shape_tasks: Map.delete(state.shape_tasks, ref)}
  end

  defp maybe_reset_shape_state(state, repo) when repo == state.repo, do: state

  defp maybe_reset_shape_state(state, _repo) do
    Enum.each(state.shape_tasks, fn {_ref, %{task: task}} ->
      Task.shutdown(task, :brutal_kill)
    end)

    %{state | shape_attempts: %{}, shape_tasks: %{}}
  end

  defp shape_in_flight?(state, issue_number) do
    Enum.any?(state.shape_tasks, fn {_ref, meta} -> meta.issue_number == issue_number end)
  end

  defp log_shape_result(task_meta, {:ok, result}) when result in [:shaped, :already_shaped] do
    Logger.info("issue ##{task_meta.issue_number} shaped successfully, deferring until next poll")
  end

  defp log_shape_result(task_meta, {:error, reason}) do
    Logger.info(
      "issue ##{task_meta.issue_number} not ready (#{Enum.join(task_meta.failures, ", ")}); shaping failed: #{inspect(reason)}"
    )
  end

  defp log_shape_result(task_meta, other) do
    Logger.info(
      "issue ##{task_meta.issue_number} not ready (#{Enum.join(task_meta.failures, ", ")}); shaping failed: #{inspect(other)}"
    )
  end

  defp task_supervisor do
    Application.get_env(:conductor, :task_supervisor, Conductor.TaskSupervisor)
  end

  defp maybe_start_ready_issue(state, issue, remaining_slots)
       when remaining_slots > 0,
       do: start_run(state, issue)

  defp maybe_start_ready_issue(state, _issue, _remaining_slots), do: state

  defp safe_shape_issue(repo, issue_number) do
    try do
      case shaper_mod().shape(repo, issue_number) do
        {:ok, result} when result in [:shaped, :already_shaped] -> {:ok, result}
        {:error, _reason} = error -> error
        other -> {:error, {:unexpected_shaper_result, other}}
      end
    rescue
      error ->
        {:error, {:raised, error}}
    catch
      kind, reason ->
        {:error, {kind, reason}}
    end
  end

  defp maybe_warn_unfiltered_loop(%{repo: repo, label: label}) when label in [nil, ""] do
    Logger.warning(
      "starting poll loop for #{repo} without a label filter; all open issues are eligible"
    )
  end

  defp maybe_warn_unfiltered_loop(_state), do: :ok

  defp start_run(state, issue) do
    case pick_worker(state) do
      {:ok, worker, state} ->
        dispatch_run(state, issue, worker.name)

      {:error, :no_available_workers, state} ->
        Logger.info("no healthy worker available, deferring issue ##{issue.number}")
        state
    end
  end

  defp dispatch_run(state, issue, worker) do
    existing_pr =
      case code_host_mod().find_open_pr(state.repo, issue.number, nil) do
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
        workers: state.worker_order,
        trusted_surfaces: state.trusted_surfaces
      ] ++ adoption_opts(existing_pr)

    case run_launcher().start(opts) do
      {:ok, pid} ->
        ref = Process.monitor(pid)
        run_entry = %{pid: pid, ref: ref, issue: issue.number, worker: worker}

        Logger.info("started run for issue ##{issue.number} on #{worker}")

        %{
          state
          | active_runs: Map.put(state.active_runs, issue.number, run_entry)
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

  defp pick_worker(%{worker_order: []} = state), do: {:error, :no_available_workers, state}

  defp pick_worker(state) do
    count = length(state.worker_order)

    0..(count - 1)
    |> Enum.reduce_while({:error, :no_available_workers, state}, fn offset,
                                                                    {_status, _reason, acc} ->
      candidate_index = rem(acc.worker_index + offset, count)
      worker_name = Enum.at(acc.worker_order, candidate_index)

      case probe_and_reserve_worker(acc, worker_name, candidate_index) do
        {:ok, worker, next_state} ->
          {:halt, {:ok, worker, next_state}}

        {:error, next_state} ->
          {:cont, {:error, :no_available_workers, next_state}}
      end
    end)
  end

  defp probe_and_reserve_worker(state, worker_name, candidate_index) do
    {worker, state} = probe_worker(state, worker_name)

    cond do
      not worker.healthy ->
        {:error, state}

      worker_busy?(worker.name) ->
        Logger.info("worker #{worker.name} busy, skipping this cycle")
        {:error, state}

      true ->
        {:ok, worker,
         %{state | worker_index: rem(candidate_index + 1, length(state.worker_order))}}
    end
  end

  defp probe_worker(state, worker_name) do
    worker = Map.fetch!(state.workers, worker_name)

    case probe_worker_module(worker_mod(), worker.name, capability_tags: worker.capability_tags) do
      {:ok, _} ->
        updated = %{
          worker
          | healthy: true,
            drained: false,
            consecutive_failures: 0,
            last_error: nil
        }

        {updated, put_worker(state, updated)}

      {:error, reason} ->
        failures = worker.consecutive_failures + 1
        drained = failures >= Config.fleet_probe_failure_threshold()

        updated = %{
          worker
          | healthy: false,
            drained: drained,
            consecutive_failures: failures,
            last_error: to_string(reason)
        }

        if drained do
          Logger.warning("worker #{worker.name} drained after #{failures} failed probes")
        end

        {updated, put_worker(state, updated)}
    end
  end

  defp ordered_workers(state) do
    assignments =
      state.active_runs
      |> Enum.map(fn {_issue_number, run} -> {run.worker, run.issue} end)
      |> Map.new()

    Enum.map(state.worker_order, fn worker_name ->
      worker = Map.fetch!(state.workers, worker_name)
      Map.put(worker, :assignment, assignment_for(worker_name, assignments))
    end)
  end

  defp assignment_for(worker_name, assignments) do
    case Map.get(assignments, worker_name) do
      nil -> nil
      issue_number -> %{issue_number: issue_number}
    end
  end

  defp put_worker(state, worker) do
    %{state | workers: Map.put(state.workers, worker.name, worker)}
  end

  defp normalize_workers(workers) do
    workers
    |> Config.normalize_workers()
    |> Enum.map(fn worker ->
      Map.merge(
        %{
          healthy: true,
          drained: false,
          consecutive_failures: 0,
          last_error: nil
        },
        worker
      )
    end)
  end

  defp worker_map(workers), do: Map.new(workers, fn worker -> {worker.name, worker} end)

  defp reconcile(state) do
    try do
      # 1. Remove in-memory entries for dead processes
      active =
        state.active_runs
        |> Enum.filter(fn {_id, %{pid: pid}} -> Process.alive?(pid) end)
        |> Map.new()

      state = %{state | active_runs: active}

      # 2. Detect and expire stale runs from the Store (covers restarts and orphans)
      expire_stale_runs(state)
    rescue
      exception ->
        Logger.warning("[reconcile] failed to read active runs: #{Exception.message(exception)}")
        state
    catch
      :exit, reason ->
        Logger.warning("[reconcile] failed to read active runs: #{inspect(reason)}")
        state
    end
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

  defp reconcile_held_leases(%{repo: nil}), do: :ok

  defp reconcile_held_leases(%{repo: repo}) do
    repo
    |> Store.list_held_leases()
    |> Enum.each(fn lease ->
      issue_number = lease["issue_number"]
      run_id = lease["run_id"]
      pr_number = lease["pr_number"]

      resolution = resolve_held_lease(repo, pr_number, issue_number)

      case resolution do
        :hold ->
          :ok

        {"external_merge", _reason} = merged_resolution ->
          case close_issue_after_merge(repo, issue_number, pr_number, "external merge") do
            :ok ->
              release_reconciled_lease(repo, run_id, issue_number, pr_number, merged_resolution)

            {:error, reason} ->
              Logger.warning(
                "[reconcile] keeping lease for issue ##{issue_number} after external merge of PR ##{pr_number}; issue closure will retry on the next poll: #{inspect(reason)}"
              )

              :ok
          end

        {event_type, _reason} ->
          release_reconciled_lease(repo, run_id, issue_number, pr_number, {event_type, nil})
      end
    end)
  rescue
    exception ->
      Logger.warning("[reconcile] held lease check failed: #{Exception.message(exception)}")
      :ok
  catch
    :exit, reason ->
      Logger.warning("[reconcile] held lease check failed: #{inspect(reason)}")
      :ok
  end

  defp resolve_held_lease(repo, pr_number, _issue_number) when is_integer(pr_number) do
    case code_host_mod().pr_state(repo, pr_number) do
      {:ok, "MERGED"} -> {"external_merge", "PR merged externally"}
      {:ok, "CLOSED"} -> {"external_close", "PR closed without merge"}
      {:ok, _open} -> :hold
      {:error, _} -> :hold
    end
  end

  defp resolve_held_lease(_repo, _pr_number, _issue_number), do: :hold

  defp find_latest_run(repo, issue_number) do
    Store.list_runs(repo: repo, issue_number: issue_number, limit: 1)
    |> List.first()
  end

  defp release_reconciled_lease(repo, run_id, issue_number, pr_number, {event_type, _reason}) do
    Logger.info("[reconcile] releasing orphan lease for issue ##{issue_number}: #{event_type}")

    Store.record_event(run_id, event_type, %{
      issue_number: issue_number,
      pr_number: pr_number
    })

    Store.release_lease(repo, issue_number)
  end

  # --- Label-Driven Merge ---

  defp merge_labeled_prs(%{repo: nil}), do: :ok

  defp merge_labeled_prs(%{repo: repo} = state) do
    case code_host_mod().labeled_prs(repo, "lgtm") do
      {:ok, prs} ->
        Enum.each(prs, fn pr ->
          pr_number = pr["number"]

          case code_host_mod().ci_status(repo, pr_number) do
            {:ok, %{state: :green}} ->
              cond do
                ci_timeout_run?(repo, pr_number) ->
                  Logger.warning(
                    "[merge] PR ##{pr_number} previously timed out waiting for CI, skipping"
                  )

                not conductor_tracked?(repo, pr_number) ->
                  Logger.debug(
                    "[merge] PR ##{pr_number} has lgtm but is not conductor-tracked, skipping"
                  )

                true ->
                  clear_ci_wait_tracking(repo, pr_number)

                  case operator_merge_decision(repo, pr) do
                    :allow ->
                      Logger.info("[merge] PR ##{pr_number} has lgtm + green CI, merging")

                      case code_host_mod().merge(repo, pr_number, []) do
                        :ok ->
                          Logger.info("[merge] PR ##{pr_number} merged successfully")

                          case record_merge(repo, pr_number) do
                            :ok ->
                              Conductor.SelfUpdate.maybe_reload(repo, pr_number)

                            {:error, reason} ->
                              Logger.warning(
                                "[merge] PR ##{pr_number} merged but post-merge reconciliation is incomplete: #{inspect(reason)}"
                              )
                          end

                        {:error, reason} ->
                          if merge_conflict?(reason) do
                            attempt_rebase_merge(
                              repo,
                              pr_number,
                              pr["headRefName"],
                              state.worker_order
                            )
                          else
                            Logger.warning("[merge] PR ##{pr_number} merge failed: #{reason}")
                          end
                      end

                    {:blocked, reason} ->
                      mark_operator_blocked(repo, pr_number, reason)

                    :skip ->
                      Logger.warning(
                        "[merge] PR ##{pr_number} operator checks unavailable, skipping"
                      )
                  end
              end

            {:ok, %{state: :pending} = ci_status} ->
              maybe_handle_pending_ci(repo, pr_number, ci_status)

            {:ok, %{state: :failed} = ci_status} ->
              clear_ci_wait_tracking(repo, pr_number)

              Logger.debug(
                "[merge] PR ##{pr_number} has lgtm but CI failed: #{ci_status.summary}"
              )

            {:ok, %{state: :unknown} = ci_status} ->
              Logger.debug(
                "[merge] PR ##{pr_number} has lgtm but CI is still unclear: #{ci_status.summary}"
              )

            {:error, reason} ->
              Logger.warning(
                "[merge] failed to inspect CI for PR ##{pr_number}: #{inspect(reason)}"
              )
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
      {:ok, %{"run_id" => run_id, "issue_number" => issue_number}} ->
        Store.record_event(run_id, "merge_conflict_blocked", %{
          pr_number: pr_number,
          reason: "merge_conflict_unresolvable"
        })

        Store.terminate_run(run_id, "blocked", "blocked", repo, issue_number)

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

            case record_merge(repo, pr_number) do
              :ok ->
                Conductor.SelfUpdate.maybe_reload(repo, pr_number)

              {:error, reason} ->
                Logger.warning(
                  "[merge] PR ##{pr_number} merged after rebase but post-merge reconciliation is incomplete: #{inspect(reason)}"
                )
            end

          {:error, retry_reason} ->
            if merge_conflict?(retry_reason) do
              Logger.warning("[merge] PR ##{pr_number} still has conflict after rebase")
              mark_conflict_blocked(repo, pr_number)
            else
              # Transient/policy failure after rebase — don't mark blocked,
              # let normal retry pick it up on the next poll cycle.
              Logger.warning(
                "[merge] PR ##{pr_number} post-rebase merge failed (non-conflict): #{retry_reason}"
              )
            end
        end

      {:error, reason} ->
        Logger.warning("[merge] rebase failed for PR ##{pr_number}: #{reason}")
        mark_conflict_blocked(repo, pr_number)
    end
  end

  defp issue_in_cooldown?(repo, issue_number) do
    {streak, last_failed_at} = Store.issue_failure_streak(repo, issue_number)

    if streak == 0 do
      false
    else
      cooldown_minutes = min(trunc(:math.pow(2, streak)), Config.issue_cooldown_cap_minutes())

      case DateTime.from_iso8601(last_failed_at || "") do
        {:ok, failed_at, _} ->
          cutoff = DateTime.add(DateTime.utc_now(), -cooldown_minutes * 60, :second)

          if DateTime.compare(failed_at, cutoff) == :gt do
            Logger.info(
              "[governor] issue ##{issue_number} in cooldown (streak=#{streak}, #{cooldown_minutes}m)"
            )

            true
          else
            false
          end

        _ ->
          false
      end
    end
  end

  defp operator_blocked_issue?(repo, issue_number) do
    case operator_issue_decision(repo, issue_number) do
      {:blocked, reason} ->
        Logger.info("[dispatch] issue ##{issue_number} skipped: #{reason}")
        true

      :allow ->
        false

      :skip ->
        Logger.warning("[dispatch] issue ##{issue_number} operator checks unavailable, skipping")
        true
    end
  end

  defp cancel_active_runs(%{repo: nil} = state), do: state

  defp cancel_active_runs(state) do
    active_runs =
      Enum.reduce(state.active_runs, state.active_runs, fn {issue_number, run}, acc ->
        if active_run_cancelled?(state.repo, issue_number) do
          case run_control_mod().operator_block(run.pid, "operator_cancel") do
            :ok ->
              Logger.warning("[dispatch] active issue ##{issue_number} blocked: operator_cancel")
              Map.delete(acc, issue_number)

            {:error, reason} ->
              Logger.warning(
                "[dispatch] failed to block active issue ##{issue_number}: #{inspect(reason)}"
              )

              acc
          end
        else
          acc
        end
      end)

    %{state | active_runs: active_runs}
  end

  defp active_run_cancelled?(repo, issue_number) do
    case tracker_mod().issue_comments(repo, issue_number) do
      {:ok, comments} ->
        cancel_comment_present?(comments)

      {:error, reason} ->
        Logger.warning(
          "[dispatch] failed to read comments for active issue ##{issue_number}: #{inspect(reason)}"
        )

        false
    end
  end

  defp operator_merge_decision(repo, pr) do
    case issue_number_for_pr(repo, pr) do
      {:ok, issue_number} -> operator_issue_decision(repo, issue_number)
      :unmapped -> :allow
      :skip -> :skip
    end
  end

  defp operator_issue_decision(repo, issue_number) do
    case tracker_mod().issue_has_label?(repo, issue_number, Config.operator_hold_label()) do
      {:ok, true} ->
        {:blocked, "operator_hold"}

      {:ok, false} ->
        case tracker_mod().issue_comments(repo, issue_number) do
          {:ok, comments} ->
            if cancel_comment_present?(comments) do
              {:blocked, "operator_cancel"}
            else
              :allow
            end

          {:error, reason} ->
            Logger.warning(
              "[operator] failed to read comments for issue ##{issue_number}: #{inspect(reason)}"
            )

            :skip
        end

      {:error, reason} ->
        Logger.warning(
          "[operator] failed to check hold label for issue ##{issue_number}: #{inspect(reason)}"
        )

        :skip
    end
  end

  defp cancel_comment_present?(comments) do
    Enum.any?(comments, fn comment ->
      body = Map.get(comment, "body", "")
      String.trim(body) |> String.downcase() == String.downcase(Config.operator_cancel_command())
    end)
  end

  defp issue_number_for_pr(repo, pr) do
    issue_number_for_pr_lookup(repo, pr["number"], pr["headRefName"] || "")
  end

  @doc false
  def issue_number_for_pr_lookup(
        repo,
        pr_number,
        head_ref_name,
        find_run_by_pr_fn \\ &Store.find_run_by_pr/2
      ) do
    try do
      case find_run_by_pr_fn.(repo, pr_number) do
        {:ok, %{"issue_number" => issue_number}} ->
          {:ok, issue_number}

        {:error, :not_found} ->
          case parse_issue_number_from_branch(head_ref_name) do
            {:ok, issue_number} -> {:ok, issue_number}
            :skip -> :unmapped
          end

        {:error, reason} ->
          Logger.warning("[operator] failed to find run for PR ##{pr_number}: #{inspect(reason)}")

          :skip
      end
    rescue
      exception ->
        Logger.warning(
          "[operator] failed to find run for PR ##{pr_number}: #{Exception.message(exception)}"
        )

        :skip
    catch
      :exit, reason ->
        Logger.warning("[operator] failed to find run for PR ##{pr_number}: #{inspect(reason)}")
        :skip
    end
  end

  # Extract issue number from branch names like "factory/42-ts", "fix/42-desc",
  # "team/fix/42-desc", "42-desc", "fix/42". Uses the final path segment after the last "/".
  @doc false
  def parse_issue_number_from_branch(branch) do
    segment = branch |> String.split("/") |> List.last()

    case String.split(segment, "-", parts: 2) do
      [issue_str | _] ->
        case Integer.parse(issue_str) do
          {value, ""} -> {:ok, value}
          _ -> :skip
        end

      _ ->
        :skip
    end
  end

  defp mark_operator_blocked(repo, pr_number, reason) do
    Logger.warning("[merge] PR ##{pr_number} blocked: #{reason}")

    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"run_id" => run_id, "issue_number" => issue_number}} ->
        Store.record_event(run_id, "operator_blocked", %{
          pr_number: pr_number,
          issue_number: issue_number,
          reason: reason
        })

        Store.terminate_run(run_id, "blocked", "blocked", repo, issue_number)

      _ ->
        Logger.debug("[merge] no run found for PR ##{pr_number}, cannot mark operator block")
    end
  end

  defp maybe_handle_pending_ci(repo, pr_number, ci_status) do
    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"phase" => "ci_timeout"}} ->
        Logger.debug("[merge] PR ##{pr_number} already marked ci_timeout, skipping")

      {:ok, %{"phase" => phase}} when phase not in ["pr_opened", nil] ->
        Logger.debug("[merge] PR ##{pr_number} is in phase #{phase}, skipping CI wait tracking")

      {:ok, run} ->
        track_pending_ci(repo, run, ci_status)

      {:error, :not_found} ->
        Logger.debug("[merge] no run found for PR ##{pr_number}, cannot track CI wait")

      {:error, reason} ->
        Logger.warning("[merge] failed to look up run for PR ##{pr_number}: #{inspect(reason)}")
    end
  end

  defp track_pending_ci(repo, run, ci_status) do
    now = DateTime.utc_now()
    run_id = run["run_id"]
    issue_number = run["issue_number"]
    pr_number = run["pr_number"]

    wait_started_at =
      case parse_datetime(run["ci_wait_started_at"]) do
        {:ok, dt} ->
          dt

        :error ->
          Store.update_run(run_id, %{ci_wait_started_at: DateTime.to_iso8601(now)})

          Store.record_event(run_id, "ci_wait_started", %{
            pr_number: pr_number,
            summary: ci_status.summary,
            pending_checks: ci_status.pending
          })

          now
      end

    elapsed_minutes = elapsed_minutes(wait_started_at, now)

    if ci_wait_log_due?(run["ci_last_reported_at"], now) do
      Logger.info("[merge] PR ##{pr_number} #{ci_status.summary}")

      Store.record_event(run_id, "ci_wait_status", %{
        pr_number: pr_number,
        elapsed_minutes: elapsed_minutes,
        summary: ci_status.summary,
        pending_checks: ci_status.pending
      })

      Store.update_run(run_id, %{ci_last_reported_at: DateTime.to_iso8601(now)})
    end

    if ci_wait_timed_out?(wait_started_at, now) do
      reason = ci_timeout_reason(ci_status, elapsed_minutes)

      Logger.warning("[merge] PR ##{pr_number} #{reason}")

      Store.record_event(run_id, "ci_timeout", %{
        pr_number: pr_number,
        elapsed_minutes: elapsed_minutes,
        reason: reason,
        pending_checks: ci_status.pending
      })

      Store.update_run(run_id, %{
        blocked_reason: reason,
        ci_wait_started_at: DateTime.to_iso8601(wait_started_at),
        ci_last_reported_at: DateTime.to_iso8601(now)
      })

      Store.terminate_run(run_id, "ci_timeout", "ci_timeout", repo, issue_number)
      maybe_comment_ci_timeout(repo, issue_number, run_id, reason)
      Conductor.Retro.analyze(run_id)
    end
  end

  defp ci_wait_log_due?(nil, _now), do: true

  defp ci_wait_log_due?(last_reported_at, now) do
    case parse_datetime(last_reported_at) do
      {:ok, last_reported} ->
        DateTime.diff(now, last_reported, :second) >= Config.ci_status_log_interval() * 60

      :error ->
        true
    end
  end

  defp ci_wait_timed_out?(wait_started_at, now) do
    DateTime.diff(now, wait_started_at, :second) >= Config.ci_timeout() * 60
  end

  defp elapsed_minutes(wait_started_at, now) do
    DateTime.diff(now, wait_started_at, :second)
    |> Kernel./(60)
    |> trunc()
    |> max(0)
  end

  defp ci_timeout_reason(ci_status, elapsed_minutes) do
    "timed out after #{elapsed_minutes} minutes waiting for CI: #{ci_status.summary}. " <>
      "Next actions: inspect the job URL(s), rerun or fix the external CI job, then trigger a new run for this issue once checks are green."
  end

  defp maybe_comment_ci_timeout(repo, issue_number, run_id, reason) do
    case tracker_mod().comment(
           repo,
           issue_number,
           "Bitterblossom timed out `#{run_id}` waiting for CI: #{reason}"
         ) do
      :ok ->
        :ok

      {:error, comment_reason} ->
        Logger.warning(
          "[merge] failed to comment on issue ##{issue_number} after CI timeout: #{inspect(comment_reason)}"
        )
    end
  end

  defp ci_timeout_run?(repo, pr_number) do
    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"phase" => "ci_timeout"}} -> true
      _ -> false
    end
  end

  defp clear_ci_wait_tracking(repo, pr_number) do
    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"run_id" => run_id, "phase" => "pr_opened"}} ->
        Store.update_run(run_id, %{
          ci_wait_started_at: nil,
          ci_last_reported_at: nil
        })

      _ ->
        :ok
    end
  end

  defp parse_datetime(nil), do: :error

  defp parse_datetime(value) do
    case DateTime.from_iso8601(value) do
      {:ok, dt, _offset} -> {:ok, dt}
      _ -> :error
    end
  end

  defp dispatch_paused? do
    try do
      Store.dispatch_paused?()
    rescue
      exception ->
        Logger.warning(
          "[dispatch] failed to read pause state: #{Exception.message(exception)}; defaulting to paused"
        )

        true
    catch
      :exit, reason ->
        Logger.warning(
          "[dispatch] failed to read pause state: #{inspect(reason)}; defaulting to paused"
        )

        true
    end
  end

  defp dispatch_mode do
    if dispatch_paused?(), do: :paused, else: :polling
  end

  defp pause_state(%{mode: :idle} = state), do: state
  defp pause_state(state), do: %{state | mode: :paused}

  # Update the Store when the orchestrator merges a PR (not the RunServer).
  defp record_merge(repo, pr_number) do
    case Store.find_run_by_pr(repo, pr_number) do
      {:ok, %{"run_id" => run_id, "issue_number" => issue_number}} ->
        case close_issue_after_merge(repo, issue_number, pr_number, "merge") do
          :ok ->
            Store.update_run(run_id, %{
              ci_wait_started_at: nil,
              ci_last_reported_at: nil,
              blocked_reason: nil
            })

            Store.record_event(run_id, "merged", %{
              pr_number: pr_number,
              merged_by: "orchestrator"
            })

            Store.terminate_run(run_id, "merged", "merged", repo, issue_number)
            Conductor.Retro.analyze(run_id)

            :ok

          {:error, reason} ->
            Logger.warning(
              "[merge] leaving run #{run_id} and lease for issue ##{issue_number} unchanged until issue closure succeeds"
            )

            {:error, reason}
        end

      _ ->
        Logger.debug("[merge] no run found for PR ##{pr_number}, skipping store update")
        :ok
    end
  end

  defp conductor_tracked?(repo, pr_number) do
    try do
      match?({:ok, _}, Store.find_run_by_pr(repo, pr_number))
    rescue
      exception ->
        Logger.warning(
          "[merge] failed to check if PR ##{pr_number} is tracked: #{Exception.message(exception)}"
        )

        false
    catch
      :exit, reason ->
        Logger.warning(
          "[merge] failed to check if PR ##{pr_number} is tracked: #{inspect(reason)}"
        )

        false
    end
  end

  defp close_issue_after_merge(repo, issue_number, pr_number, context) do
    case code_host_mod().close_issue(repo, issue_number) do
      :ok ->
        :ok

      {:error, reason} ->
        Logger.warning(
          "[merge] failed to close issue ##{issue_number} after #{context} for PR ##{pr_number}: #{inspect(reason)}"
        )

        {:error, reason}
    end
  end

  defp tracker_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  defp shaper_mod, do: Application.get_env(:conductor, :shaper_module, Conductor.Shaper)

  defp run_launcher,
    do: Application.get_env(:conductor, :run_launcher_module, __MODULE__.RunLauncher)

  defp run_control_mod,
    do: Application.get_env(:conductor, :run_control_module, Conductor.RunServer)

  defp self_update_mod,
    do: Application.get_env(:conductor, :self_update_module, Conductor.SelfUpdate)

  @doc false
  def probe_worker_module(worker_module, worker, opts \\ []) do
    cond do
      function_exported?(worker_module, :probe, 2) ->
        worker_module.probe(worker, opts)

      function_exported?(worker_module, :status, 1) ->
        worker_module.status(worker)

      true ->
        if worker_module.reachable?(worker),
          do: {:ok, %{sprite: worker, reachable: true}},
          else: {:error, :unreachable}
    end
  end

  defp worker_busy?(worker) do
    if function_exported?(worker_mod(), :busy?, 2) do
      worker_mod().busy?(worker, [])
    else
      worker_mod().busy?(worker)
    end
  end

  defp schedule_poll(delay) do
    Process.send_after(self(), :poll, delay)
  end

  # On shutdown, kill all agent processes and revoke gh auth on every fleet sprite.
  # Ensures sprites that outlive the conductor cannot exercise merge authority.
  # Best-effort: failures are logged but don't block shutdown of remaining sprites.
  # Uses state.worker_order (not Application env) so this works on all startup paths.
  defp kill_fleet_agents(%{worker_order: workers}) when is_list(workers) do
    Enum.each(workers, fn name ->
      try do
        worker_mod().kill_and_revoke(name)
      rescue
        e ->
          Logger.warning(
            "[shutdown] failed to kill/revoke sprite #{name}: #{Exception.message(e)}"
          )
      catch
        kind, reason ->
          Logger.warning(
            "[shutdown] failed to kill/revoke sprite #{name}: #{kind} #{inspect(reason)}"
          )
      end
    end)
  end

  defp kill_fleet_agents(_state), do: :ok
end
