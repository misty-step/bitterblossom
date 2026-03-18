defmodule Conductor.RunServer do
  @moduledoc """
  Per-run GenServer. Owns one issue from lease to PR opened.

  State machine:

      pending → building → pr_opened (terminal)
                            ├── blocked
                            └── failed

  Lease lifecycle: the lease means "this issue is claimed" — it persists from
  dispatch through merge, block, or external resolution. RunServer exits at
  pr_opened but the lease holds. The orchestrator releases the lease at merge
  or via reconciliation. fail/3 and block/2 release immediately (terminal).
  """

  use GenServer, restart: :temporary
  require Logger

  alias Conductor.{Store, Workspace, Prompt, Config, Retro}

  defp tracker_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)
  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  defp workspace_mod, do: Application.get_env(:conductor, :workspace_module, Workspace)

  @heartbeat_ms 30_000

  defstruct [
    :run_id,
    :repo,
    :issue,
    :worker,
    :branch,
    :existing_branch,
    :worktree_path,
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

  def operator_block(pid, reason) do
    GenServer.call(pid, {:operator_block, reason})
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
            state = %{state | run_id: run_id, branch: branch}

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
        fn ->
          workspace_mod().adopt_branch(state.worker, state.repo, state.run_id, state.branch)
        end
      else
        fn -> workspace_mod().prepare(state.worker, state.repo, state.run_id, state.branch) end
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
    log(state, "dispatching Weaver to #{state.worker}")

    prompt =
      Prompt.build_builder_prompt(
        state.issue,
        state.run_id,
        state.branch,
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
        log(state, "Weaver dispatch completed, detecting PR")
        detect_pr(state)

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

  @impl true
  def handle_call({:operator_block, reason}, _from, state) do
    state = cancel_dispatch(state)

    case block(state, reason) do
      {:stop, :normal, new_state} -> {:stop, :normal, :ok, new_state}
    end
  end

  # --- Private ---

  defp detect_pr(state) do
    case code_host_mod().find_open_pr(state.repo, state.issue.number, state.branch) do
      {:ok, %{"headRefName" => head_ref, "number" => _number, "url" => _url} = pr}
      when head_ref == state.branch ->
        handle_pr_ready(pr, state)

      {:ok, %{"headRefName" => head_ref}} when head_ref != state.branch ->
        fail(
          state,
          "pr_branch_mismatch",
          "Weaver opened PR on unexpected branch #{inspect(head_ref)} (expected #{inspect(state.branch)})"
        )

      {:ok, pr} ->
        fail(
          state,
          "pr_detection_failed",
          "Weaver PR lookup returned incomplete data: #{inspect(Map.take(pr, ["number", "url", "headRefName"]))}"
        )

      {:error, :not_found} ->
        case read_workspace_file(state, "BLOCKED.md") do
          {:ok, reason} ->
            block(state, reason)

          {:error, :not_found} ->
            fail(state, "pr_not_found", "Weaver completed without opening a PR")

          {:error, reason} ->
            fail(state, "workspace_read_error", inspect(reason))
        end

      {:error, reason} ->
        fail(state, "pr_detection_failed", inspect(reason))
    end
  end

  defp handle_pr_ready(pr, state) do
    pr_number = pr["number"]
    pr_url = pr["url"]

    Store.complete_run(state.run_id, "pr_opened", "pr_opened")

    Store.update_run(state.run_id, %{
      pr_number: pr_number,
      pr_url: pr_url,
      turn_count: state.turn_count
    })

    Store.record_event(state.run_id, "builder_pr_detected", %{
      pr_number: pr_number,
      pr_url: pr_url
    })

    log(state, "Weaver opened PR ##{pr_number}: #{pr_url}")
    cleanup_workspace(state)
    {:stop, :normal, %{state | phase: :pr_opened, pr_number: pr_number}}
  end

  defp fail(state, event_type, reason) do
    role_log(:error, state, "#{event_type}: #{reason}")
    Store.record_event(state.run_id, event_type, %{reason: reason})
    Store.terminate_run(state.run_id, "failed", "failed", state.repo, state.issue.number)
    cleanup_workspace(state)
    Retro.analyze(state.run_id)
    {:stop, :normal, %{state | phase: :failed}}
  end

  defp block(state, reason) do
    role_log(:warning, state, "blocked: #{reason}")
    Store.record_event(state.run_id, "run_blocked", %{reason: reason})
    Store.terminate_run(state.run_id, "blocked", "blocked", state.repo, state.issue.number)
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

  defp cancel_dispatch(%{dispatch_task: %Task{} = task} = state) do
    cancel_heartbeat(state.heartbeat_timer)
    Task.shutdown(task, :brutal_kill)
    maybe_kill_worker(state)
    %{state | dispatch_task: nil, heartbeat_timer: nil}
  end

  defp cancel_dispatch(state) do
    cancel_heartbeat(state.heartbeat_timer)
    maybe_kill_worker(state)
    %{state | heartbeat_timer: nil}
  end

  defp maybe_kill_worker(state) do
    if function_exported?(worker_mod(), :kill, 1) do
      _ = worker_mod().kill(state.worker)
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

        case read_file(path) do
          {:ok, content} -> [String.trim(content)]
          _ -> []
        end
      end)

    case parts do
      [] -> nil
      _ -> parts |> Enum.join("\n\n---\n\n") |> String.slice(0, 8_000)
    end
  end

  defp read_workspace_file(%{worktree_path: nil}, _filename), do: {:error, :not_found}

  defp read_workspace_file(state, filename) do
    path = Path.join(state.worktree_path, filename)

    case worker_mod().exec(state.worker, "cat '#{path}'", timeout: 30_000) do
      {:ok, content} -> {:ok, String.trim(content)}
      {:error, output, code} -> classify_workspace_read_error(output, code)
    end
  end

  defp classify_workspace_read_error(output, code) do
    normalized_output = String.downcase(to_string(output || ""))

    file_missing =
      String.contains?(normalized_output, "not found") or
        String.contains?(normalized_output, "no such file or directory")

    if code == 1 and file_missing do
      {:error, :not_found}
    else
      {:error, %{output: String.slice(to_string(output || ""), 0, 200), code: code}}
    end
  end

  defp read_file(path) do
    case File.read(path) do
      {:ok, content} -> {:ok, content}
      _ -> {:error, :not_found}
    end
  end

  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)

  defp log(state, msg) do
    formatted = role_log(:info, state, msg)
    IO.puts(formatted)
  end

  defp role_log(level, state, msg) do
    label = state.run_id || "init"
    formatted = "[weaver][#{label}] #{msg}"

    case level do
      :info -> Logger.info(formatted)
      :warning -> Logger.warning(formatted)
      :error -> Logger.error(formatted)
    end

    formatted
  end
end
