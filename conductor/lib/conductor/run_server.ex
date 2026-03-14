defmodule Conductor.RunServer do
  @moduledoc """
  Per-run GenServer. Owns one issue's lifecycle from lease to terminal state.

  State machine (Symphony-inspired, 4 phases):

      pending → building → governing → terminal
                                         ├── merged
                                         ├── blocked
                                         └── failed

  The builder agent owns the full implementation + revision cycle.
  The conductor only handles authority decisions: lease, merge, block.
  """

  use GenServer, restart: :temporary
  require Logger

  alias Conductor.{Store, GitHub, Sprite, Workspace, Prompt, Config}

  @heartbeat_ms 30_000
  @ci_poll_ms 30_000

  defstruct [
    :run_id,
    :repo,
    :issue,
    :worker,
    :branch,
    :worktree_path,
    :artifact_path,
    :pr_number,
    :pr_url,
    :dispatch_task,
    :heartbeat_timer,
    :ci_deadline,
    phase: :pending,
    turn_count: 0,
    trusted_surfaces: []
  ]

  # --- Public API ---

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts)
  end

  def status(pid) do
    GenServer.call(pid, :status)
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    state = %__MODULE__{
      repo: Keyword.fetch!(opts, :repo),
      issue: Keyword.fetch!(opts, :issue),
      worker: Keyword.fetch!(opts, :worker),
      trusted_surfaces: Keyword.get(opts, :trusted_surfaces, [])
    }

    {:ok, state, {:continue, :acquire_lease}}
  end

  @impl true
  def handle_continue(:acquire_lease, state) do
    # Generate run_id first so the lease is immediately valid
    ts = System.system_time(:second)
    run_id = "run-#{state.issue.number}-#{ts}"

    case Store.acquire_lease(state.repo, state.issue.number, run_id) do
      {:error, :already_leased} ->
        Logger.warning("issue ##{state.issue.number} already leased, skipping")
        {:stop, :normal, state}

      :ok ->
        case Store.create_run(%{
               run_id: run_id,
               repo: state.repo,
               issue_number: state.issue.number,
               issue_title: state.issue.title,
               builder_sprite: state.worker
             }) do
          {:ok, ^run_id} ->
            branch = "factory/#{state.issue.number}-#{ts}"
            artifact = Workspace.artifact_path(state.repo, run_id)

            state = %{state | run_id: run_id, branch: branch, artifact_path: artifact}

            Store.record_event(run_id, "lease_acquired", %{issue: state.issue.number})
            log(state, "lease acquired for issue ##{state.issue.number}")

            {:noreply, state, {:continue, :prepare_workspace}}

          _ ->
            Store.release_lease(state.repo, state.issue.number)

            Logger.error(
              "create_run failed after lease acquired for issue ##{state.issue.number}"
            )

            {:stop, :normal, state}
        end
    end
  end

  @impl true
  def handle_continue(:prepare_workspace, state) do
    log(state, "preparing workspace on #{state.worker}")

    case Workspace.prepare(state.worker, state.repo, state.run_id, state.branch) do
      {:ok, path} ->
        Store.record_event(state.run_id, "builder_workspace_prepared", %{workspace: path})
        Store.update_run(state.run_id, %{phase: "building", branch: state.branch})

        state = %{state | worktree_path: path, phase: :building}
        log(state, "workspace ready: #{path}")

        {:noreply, state, {:continue, :dispatch_builder}}

      {:error, reason} ->
        fail(state, "workspace_preparation_failed", reason)
    end
  end

  @impl true
  def handle_continue(:dispatch_builder, state) do
    log(state, "dispatching builder to #{state.worker}")

    # Delete stale artifact before dispatch (prevent false completion)
    Sprite.exec(state.worker, "rm -f #{state.artifact_path}", timeout: 10_000)

    prompt =
      Prompt.build_builder_prompt(
        state.issue,
        state.run_id,
        state.branch,
        state.artifact_path,
        pr_number: state.pr_number
      )

    Store.record_event(state.run_id, "builder_dispatched", %{
      sprite: state.worker,
      turn: state.turn_count + 1
    })

    # Dispatch in a linked task so GenServer stays responsive
    task =
      Task.async(fn ->
        Sprite.dispatch(state.worker, prompt, state.repo,
          timeout: Config.builder_timeout(),
          workspace: state.worktree_path,
          template: Config.prompt_template()
        )
      end)

    timer = start_heartbeat()

    {:noreply,
     %{state | dispatch_task: task, heartbeat_timer: timer, turn_count: state.turn_count + 1}}
  end

  @impl true
  def handle_continue(:read_artifact, state) do
    log(state, "reading builder artifact")

    case Sprite.read_artifact(state.worker, state.artifact_path) do
      {:ok, artifact} ->
        handle_artifact(artifact, state)

      {:error, reason} ->
        # Builder finished but no artifact — treat as failure
        fail(state, "artifact_missing", "builder completed without artifact: #{reason}")
    end
  end

  @impl true
  def handle_continue(:govern, state) do
    log(state, "entering governance for PR ##{state.pr_number}")
    Store.update_run(state.run_id, %{phase: "governing"})
    ci_deadline = System.monotonic_time(:millisecond) + Config.ci_timeout() * 60_000

    {:noreply, %{state | phase: :governing, ci_deadline: ci_deadline}, {:continue, :wait_pr_age}}
  end

  @impl true
  def handle_continue(:wait_pr_age, state) do
    min_age = Config.pr_minimum_age()

    if min_age > 0 do
      log(state, "waiting #{min_age}s for PR to age")
      Process.send_after(self(), :check_pr_age, min_age * 1_000)
      {:noreply, state}
    else
      {:noreply, state, {:continue, :check_ci}}
    end
  end

  @impl true
  def handle_continue(:check_ci, state) do
    log(state, "checking CI status")

    if GitHub.checks_green?(state.repo, state.pr_number) do
      Store.record_event(state.run_id, "ci_passed", %{pr_number: state.pr_number})
      {:noreply, state, {:continue, :attempt_merge}}
    else
      log(state, "CI not green yet, polling in #{@ci_poll_ms}ms")
      Process.send_after(self(), :poll_ci, @ci_poll_ms)
      {:noreply, state}
    end
  end

  @impl true
  def handle_continue(:attempt_merge, state) do
    log(state, "attempting merge of PR ##{state.pr_number}")

    case GitHub.merge_pr(state.repo, state.pr_number) do
      :ok ->
        Store.record_event(state.run_id, "merged", %{pr_number: state.pr_number})
        Store.complete_run(state.run_id, "merged", "merged")
        Store.release_lease(state.repo, state.issue.number)
        cleanup_workspace(state)
        log(state, "PR ##{state.pr_number} merged successfully")
        {:stop, :normal, %{state | phase: :merged}}

      {:error, reason} ->
        fail(state, "merge_failed", reason)
    end
  end

  # --- Handle dispatch task completion ---

  @impl true
  def handle_info({ref, result}, %{dispatch_task: %Task{ref: ref}} = state) do
    Process.demonitor(ref, [:flush])
    cancel_heartbeat(state.heartbeat_timer)

    state = %{state | dispatch_task: nil, heartbeat_timer: nil}

    case result do
      {:ok, _output} ->
        Store.record_event(state.run_id, "builder_complete", %{turn: state.turn_count})
        log(state, "builder dispatch completed, reading artifact")
        {:noreply, state, {:continue, :read_artifact}}

      {:error, output, code} ->
        fail(state, "builder_dispatch_failed", "exit #{code}: #{String.slice(output, 0, 500)}")
    end
  end

  # Handle task DOWN (crash)
  @impl true
  def handle_info({:DOWN, ref, :process, _pid, reason}, %{dispatch_task: %Task{ref: ref}} = state) do
    cancel_heartbeat(state.heartbeat_timer)
    fail(state, "builder_dispatch_crashed", inspect(reason))
  end

  @impl true
  def handle_info(:heartbeat, state) do
    Store.heartbeat_run(state.run_id)
    timer = start_heartbeat()
    {:noreply, %{state | heartbeat_timer: timer}}
  end

  @impl true
  def handle_info(:check_pr_age, state) do
    {:noreply, state, {:continue, :check_ci}}
  end

  @impl true
  def handle_info(:poll_ci, state) do
    Store.heartbeat_run(state.run_id)

    if System.monotonic_time(:millisecond) > state.ci_deadline do
      fail(state, "ci_timeout", "CI did not pass within #{Config.ci_timeout()} minutes")
    else
      {:noreply, state, {:continue, :check_ci}}
    end
  end

  @impl true
  def handle_call(:status, _from, state) do
    {:reply,
     %{
       run_id: state.run_id,
       phase: state.phase,
       issue: state.issue.number,
       worker: state.worker,
       pr_number: state.pr_number,
       turn_count: state.turn_count
     }, state}
  end

  # --- Private ---

  defp handle_artifact(%{"status" => "ready"} = artifact, state) do
    pr_number = artifact["pr_number"]
    pr_url = artifact["pr_url"]
    summary = artifact["summary"]

    Store.update_run(state.run_id, %{
      pr_number: pr_number,
      pr_url: pr_url,
      turn_count: state.turn_count
    })

    Store.record_event(state.run_id, "builder_artifact_ready", %{
      pr_number: pr_number,
      pr_url: pr_url,
      summary: summary
    })

    log(state, "builder reports ready — PR ##{pr_number}: #{pr_url}")

    state = %{state | pr_number: pr_number, pr_url: pr_url}
    {:noreply, state, {:continue, :govern}}
  end

  defp handle_artifact(%{"status" => "blocked"} = artifact, state) do
    reason = artifact["blocking_reason"] || "builder reported blocked"
    pr_number = artifact["pr_number"]

    if pr_number do
      Store.update_run(state.run_id, %{pr_number: pr_number, pr_url: artifact["pr_url"]})
    end

    block(state, reason)
  end

  defp handle_artifact(artifact, state) do
    fail(state, "invalid_artifact", "unexpected artifact: #{inspect(artifact)}")
  end

  defp fail(state, event_type, reason) do
    Logger.error("[#{state.run_id}] #{event_type}: #{reason}")
    Store.record_event(state.run_id, event_type, %{reason: reason})
    Store.complete_run(state.run_id, "failed", "failed")
    Store.release_lease(state.repo, state.issue.number)
    cleanup_workspace(state)
    {:stop, :normal, %{state | phase: :failed}}
  end

  defp block(state, reason) do
    Logger.warning("[#{state.run_id}] blocked: #{reason}")
    Store.record_event(state.run_id, "run_blocked", %{reason: reason})
    Store.complete_run(state.run_id, "blocked", "blocked")
    Store.release_lease(state.repo, state.issue.number)
    cleanup_workspace(state)

    # Comment on the issue so the operator knows
    GitHub.create_issue_comment(
      state.repo,
      state.issue.number,
      "Bitterblossom blocked `#{state.run_id}`: #{reason}"
    )

    {:stop, :normal, %{state | phase: :blocked}}
  end

  defp cleanup_workspace(state) do
    if state.worktree_path do
      case Workspace.cleanup(state.worker, state.repo, state.run_id) do
        :ok ->
          Store.record_event(state.run_id, "workspace_cleaned", %{})

        {:error, reason} ->
          Store.record_event(state.run_id, "workspace_cleanup_failed", %{reason: reason})
      end
    end
  end

  defp start_heartbeat do
    Process.send_after(self(), :heartbeat, @heartbeat_ms)
  end

  defp cancel_heartbeat(nil), do: :ok
  defp cancel_heartbeat(ref), do: Process.cancel_timer(ref)

  defp log(state, msg) do
    label = state.run_id || "init"
    Logger.info("[#{label}] #{msg}")
    IO.puts("[#{label}] #{msg}")
  end
end
