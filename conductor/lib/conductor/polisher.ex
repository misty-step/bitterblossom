defmodule Conductor.Polisher do
  @moduledoc """
  Polls for open PRs with green CI (no `lgtm`) and dispatches the polisher sprite.

  The polisher sprite does judgment work (address feedback, simplify code, run tests)
  and labels `lgtm` when the PR is genuinely merge-ready. The conductor then verifies
  the label + CI green and merges.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Store, Time, Workspace}

  defstruct [
    :repo,
    :polisher_sprite,
    :poll_ms,
    :base_poll_ms,
    :last_dispatch_at,
    :last_completion_at,
    in_flight: %{},
    failure_count: 0,
    health: :healthy
  ]

  # --- Public API ---

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec status() :: map()
  def status do
    GenServer.call(__MODULE__, :status)
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    repo = Keyword.fetch!(opts, :repo)
    polisher_sprite = Keyword.fetch!(opts, :polisher_sprite)
    poll_ms = Keyword.get(opts, :poll_ms, Config.poll_seconds() * 1_000)

    state = %__MODULE__{
      repo: repo,
      polisher_sprite: polisher_sprite,
      poll_ms: poll_ms,
      base_poll_ms: poll_ms
    }

    schedule_poll(state, 0)
    {:ok, state}
  end

  @impl true
  def handle_info(:poll, state) do
    state = poll_and_dispatch(state)
    schedule_poll(state, state.poll_ms)
    {:noreply, state}
  end

  @impl true
  def handle_info({ref, result}, state) when is_reference(ref) do
    Process.demonitor(ref, [:flush])
    state = complete_task(state, ref, result)
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, _ref, :process, _pid, reason}, state)
      when reason in [:normal, :shutdown] do
    # Normal exit — result already handled by {ref, result} handler
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, reason}, state) do
    Logger.warning("[fern] dispatch task crashed: #{inspect(reason)}")
    state = complete_task(state, ref, {:error, "task_crashed: #{inspect(reason)}", 1})
    {:noreply, state}
  end

  @impl true
  def handle_info(_msg, state), do: {:noreply, state}

  @impl true
  def handle_call(:status, _from, state) do
    {:reply,
     %{
       repo: state.repo,
       polisher_sprite: state.polisher_sprite,
       in_flight: Map.keys(state.in_flight) |> Map.new(&{&1, :working}),
       health: state.health,
       failure_count: state.failure_count,
       poll_ms: state.poll_ms,
       last_dispatch_at: state.last_dispatch_at,
       last_completion_at: state.last_completion_at
     }, state}
  end

  # --- Private ---

  defp poll_and_dispatch(state) do
    # Skip if sprite is already working on a PR
    if map_size(state.in_flight) > 0 do
      state
    else
      case code_host_mod().open_prs(state.repo) do
        {:ok, prs} ->
          # Dispatch at most one PR per poll (single sprite, single workspace)
          case Enum.find(prs, &needs_polish?(&1, state)) do
            nil -> state
            pr -> dispatch_polisher(state, pr)
          end

        {:error, reason} ->
          Logger.warning("[fern] failed to list open PRs: #{reason}")
          state
      end
    end
  end

  defp needs_polish?(pr, _state) do
    labels = pr["labels"] || []
    label_names = Enum.map(labels, &String.downcase(&1["name"] || ""))
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)

    "lgtm" not in label_names and Conductor.GitHub.evaluate_checks(checks)
  end

  defp dispatch_polisher(state, pr) do
    pr_number = pr["number"]
    Logger.info("[fern] PR ##{pr_number} is green, dispatching Fern")

    comments =
      case code_host_mod().pr_review_comments(state.repo, pr_number) do
        {:ok, comments} ->
          comments

        {:error, reason} ->
          Logger.warning("[fern] failed to fetch reviews for PR ##{pr_number}: #{reason}")
          []
      end

    issue_body = pr["body"] || ""
    conductor_managed = conductor_managed?(state.repo, pr_number)
    branch = pr["headRefName"]
    workspace = workspace_for_branch(state.repo, branch)

    prompt =
      Prompt.build_polisher_prompt(pr, comments, issue_body,
        may_label: conductor_managed,
        workspace_root: workspace
      )

    Store.record_event("polisher", "polisher_dispatched", %{
      pr_number: pr_number,
      sprite: state.polisher_sprite
    })

    task =
      Task.Supervisor.async_nolink(Conductor.TaskSupervisor, fn ->
        try do
          with :ok <- workspace_mod().sync_persona(state.polisher_sprite, workspace, :fern),
               {:ok, output} <-
                 worker_mod().dispatch(
                   state.polisher_sprite,
                   prompt,
                   state.repo,
                   workspace: workspace,
                   persona_role: :fern,
                   timeout: Config.polisher_timeout(),
                   harness_opts: [reasoning_effort: "high"]
                 ) do
            {:ok, output}
          else
            {:error, msg, code} -> {:error, msg, code}
            {:error, reason} -> {:error, to_string(reason), 1}
          end
        rescue
          e -> {:error, "polisher dispatch crashed: #{Exception.message(e)}", 1}
        end
      end)

    %{
      state
      | in_flight: Map.put(state.in_flight, pr_number, task.ref),
        last_dispatch_at: Time.now_utc()
    }
  end

  defp complete_task(state, ref, result) do
    {pr_number, in_flight} =
      Enum.reduce(state.in_flight, {nil, %{}}, fn {pr, r}, {found, acc} ->
        if r == ref, do: {pr, acc}, else: {found, Map.put(acc, pr, r)}
      end)

    state = %{state | in_flight: in_flight, last_completion_at: Time.now_utc()}

    if pr_number do
      case result do
        {:ok, _} ->
          Logger.info("[fern] completed work on PR ##{pr_number}")
          Store.record_event("polisher", "polisher_complete", %{pr_number: pr_number})
          reset_health(state)

        {:error, msg, _code} ->
          Logger.warning("[fern] dispatch failed for PR ##{pr_number}: #{msg}")
          Store.record_event("polisher", "polisher_failed", %{pr_number: pr_number, error: msg})
          apply_backoff(state)

        other ->
          Logger.warning("[fern] unexpected result for PR ##{pr_number}: #{inspect(other)}")
          apply_backoff(state)
      end
    else
      state
    end
  end

  defp apply_backoff(state) do
    count = state.failure_count + 1
    backoff_ms = min(trunc(state.base_poll_ms * :math.pow(2, count)), 600_000)
    health = if count >= 3, do: :unavailable, else: :degraded

    Logger.info("[fern] backoff: failures=#{count}, next_poll=#{backoff_ms}ms, health=#{health}")
    %{state | failure_count: count, poll_ms: backoff_ms, health: health}
  end

  defp reset_health(state) do
    if state.failure_count > 0 do
      Logger.info("[fern] recovered, resetting to healthy")
    end

    %{state | failure_count: 0, poll_ms: state.base_poll_ms, health: :healthy}
  end

  defp workspace_for_branch(repo, _branch) do
    Workspace.repo_root(repo)
  end

  defp schedule_poll(_, delay) do
    Process.send_after(self(), :poll, delay)
  end

  # Only conductor-tracked PRs may receive the automated `lgtm` label.
  # Non-conductor PRs get polisher review but require human merge approval.
  defp conductor_managed?(repo, pr_number) do
    try do
      match?({:ok, _}, Store.find_run_by_pr(repo, pr_number))
    rescue
      exception ->
        Logger.warning(
          "[fern] failed to find run for PR ##{pr_number}: #{Exception.message(exception)}"
        )

        false
    catch
      :exit, reason ->
        Logger.warning("[fern] failed to find run for PR ##{pr_number}: #{inspect(reason)}")
        false
    end
  end

  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
  defp workspace_mod, do: Application.get_env(:conductor, :workspace_module, Workspace)
end
