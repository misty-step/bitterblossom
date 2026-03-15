defmodule Conductor.RunServer do
  @moduledoc """
  Per-run GenServer. Owns one issue from lease to PR opened.

  State machine:

      pending → building → pr_opened (terminal)
                            ├── blocked
                            └── failed

  The builder opens a PR and exits. Governance (CI, reviews, merge)
  is handled by the orchestrator's label-driven merge loop — no sprite needed.
  """

  use GenServer, restart: :temporary
  require Logger

  alias Conductor.{Store, Workspace, Prompt, Config, Retro}

  defp tracker_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)

  @heartbeat_ms 30_000

  defstruct [
    :run_id,
    :repo,
    :issue,
    :worker,
    :branch,
    :existing_branch,
    :worktree_path,
    :artifact_path,
    :pr_number,
    :pr_url,
    :dispatch_task,
    :heartbeat_timer,
    phase: :pending,
    turn_count: 0
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
      existing_branch: Keyword.get(opts, :existing_branch),
      pr_number: Keyword.get(opts, :existing_pr_number),
      pr_url: Keyword.get(opts, :existing_pr_url)
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
            branch = state.existing_branch || "factory/#{state.issue.number}-#{ts}"
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

    prepare_fn =
      if state.existing_branch do
        fn -> Workspace.adopt_branch(state.worker, state.repo, state.run_id, state.branch) end
      else
        fn -> Workspace.prepare(state.worker, state.repo, state.run_id, state.branch) end
      end

    case prepare_fn.() do
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
    worker_mod().exec(state.worker, "rm -f '#{state.artifact_path}'", timeout: 10_000)

    prompt =
      Prompt.build_builder_prompt(
        state.issue,
        state.run_id,
        state.branch,
        state.artifact_path,
        pr_number: state.pr_number,
        repo_context: read_repo_context()
      )

    Store.record_event(state.run_id, "builder_dispatched", %{
      sprite: state.worker,
      turn: state.turn_count + 1
    })

    # Dispatch in a linked task so GenServer stays responsive
    task =
      Task.async(fn ->
        worker_mod().dispatch(state.worker, prompt, state.repo,
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

    case worker_mod().read_artifact(state.worker, state.artifact_path, []) do
      {:ok, artifact} ->
        handle_artifact(artifact, state)

      {:error, reason} ->
        # Builder finished but no artifact — treat as failure
        fail(state, "artifact_missing", "builder completed without artifact: #{reason}")
    end
  end

  # Governance (CI polling, review handling, merge) has been moved to the
  # orchestrator's label-driven merge loop. The RunServer exits at PR open.

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

    # Builder's job is done. PR is open. Governance is label-driven by the orchestrator.
    # Retro runs after merge (orchestrator), not here — avoids double analysis.
    Store.complete_run(state.run_id, "pr_opened", "pr_opened")
    Store.release_lease(state.repo, state.issue.number)
    cleanup_workspace(state)
    {:stop, :normal, %{state | phase: :pr_opened, pr_number: pr_number}}
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
    Retro.analyze(state.run_id)
    {:stop, :normal, %{state | phase: :failed}}
  end

  defp block(state, reason) do
    Logger.warning("[#{state.run_id}] blocked: #{reason}")
    Store.record_event(state.run_id, "run_blocked", %{reason: reason})
    Store.complete_run(state.run_id, "blocked", "blocked")
    Store.release_lease(state.repo, state.issue.number)
    cleanup_workspace(state)

    # Comment on the issue so the operator knows
    tracker_mod().comment(
      state.repo,
      state.issue.number,
      "Bitterblossom blocked `#{state.run_id}`: #{reason}"
    )

    Retro.analyze(state.run_id)
    {:stop, :normal, %{state | phase: :blocked}}
  end

  defp cleanup_workspace(state) do
    if state.worktree_path do
      case worker_mod().cleanup(state.worker, state.repo, state.run_id) do
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

  # Read CLAUDE.md and project.md from the repo root (one level above conductor/).
  # Returns nil if neither file exists. Truncated to ~8 KB to stay within prompt budget.
  defp read_repo_context do
    root = Path.expand("../../..", __DIR__)

    parts =
      ["CLAUDE.md", "project.md"]
      |> Enum.flat_map(fn filename ->
        path = Path.join(root, filename)

        case File.read(path) do
          {:ok, content} -> [String.trim(content)]
          _ -> []
        end
      end)

    case parts do
      [] -> nil
      _ -> parts |> Enum.join("\n\n---\n\n") |> String.slice(0, 8_000)
    end
  end

  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)

  defp log(state, msg) do
    label = state.run_id || "init"
    Logger.info("[#{label}] #{msg}")
    IO.puts("[#{label}] #{msg}")
  end
end
