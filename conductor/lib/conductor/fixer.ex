defmodule Conductor.Fixer do
  @moduledoc """
  Polls for open PRs with red CI and dispatches the fixer sprite.

  Single-responsibility: detect CI failures on open PRs,
  build a fixer prompt with failure context, and dispatch the fixer
  sprite to push a fix. Tracks in-flight dispatches to avoid
  double-fixing the same PR.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Store, Workspace}

  defstruct [
    :repo,
    :fixer_sprite,
    :poll_ms,
    in_flight: %{}
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
    fixer_sprite = Keyword.fetch!(opts, :fixer_sprite)
    poll_ms = Keyword.get(opts, :poll_ms, Config.poll_seconds() * 1_000)

    state = %__MODULE__{
      repo: repo,
      fixer_sprite: fixer_sprite,
      poll_ms: poll_ms
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
  def handle_info({ref, _result}, state) when is_reference(ref) do
    # Task completed — remove from in_flight
    Process.demonitor(ref, [:flush])
    state = complete_task(state, ref)
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, reason}, state) do
    if reason not in [:normal, :shutdown] do
      Logger.warning("[thorn] dispatch task exited: #{inspect(reason)}")
    end

    state = complete_task(state, ref)
    {:noreply, state}
  end

  @impl true
  def handle_info(_msg, state), do: {:noreply, state}

  @impl true
  def handle_call(:status, _from, state) do
    {:reply,
     %{
       repo: state.repo,
       fixer_sprite: state.fixer_sprite,
       in_flight: Map.keys(state.in_flight) |> Map.new(&{&1, :working})
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
          case Enum.find(prs, &needs_fix?(&1, state)) do
            nil -> state
            pr -> dispatch_fixer(state, pr)
          end

        {:error, reason} ->
          Logger.warning("[thorn] failed to list open PRs: #{reason}")
          state
      end
    end
  end

  defp needs_fix?(pr, _state) do
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)
    Conductor.GitHub.evaluate_checks_failed(checks)
  end

  defp dispatch_fixer(state, pr) do
    pr_number = pr["number"]
    Logger.info("[thorn] PR ##{pr_number} has red CI, dispatching Thorn")

    ci_logs =
      case code_host_mod().pr_ci_failure_logs(state.repo, pr_number) do
        {:ok, logs} ->
          logs

        {:error, reason} ->
          Logger.warning("[thorn] failed to fetch CI logs for PR ##{pr_number}: #{reason}")
          "(CI logs unavailable)"
      end

    branch = pr["headRefName"]
    workspace = workspace_for_branch(state.repo, branch)
    issue_body = extract_issue_body(pr)
    prompt = Prompt.build_fixer_prompt(pr, ci_logs, issue_body, workspace_root: workspace)

    Store.record_event("fixer", "fixer_dispatched", %{
      pr_number: pr_number,
      sprite: state.fixer_sprite
    })

    task =
      Task.async(fn ->
        try do
          with :ok <- workspace_mod().sync_persona(state.fixer_sprite, workspace, :thorn),
               {:ok, output} <-
                 worker_mod().dispatch(
                   state.fixer_sprite,
                   prompt,
                   state.repo,
                   workspace: workspace,
                   persona_role: :thorn,
                   timeout: Config.fixer_timeout()
                 ) do
            {:ok, output}
          else
            {:error, msg, code} -> {:error, msg, code}
            {:error, reason} -> {:error, to_string(reason), 1}
          end
        rescue
          e -> {:error, "fixer dispatch crashed: #{Exception.message(e)}", 1}
        end
      end)

    %{state | in_flight: Map.put(state.in_flight, pr_number, task.ref)}
  end

  defp complete_task(state, ref) do
    {pr_number, in_flight} =
      Enum.reduce(state.in_flight, {nil, %{}}, fn {pr, r}, {found, acc} ->
        if r == ref, do: {pr, acc}, else: {found, Map.put(acc, pr, r)}
      end)

    if pr_number do
      Logger.info("[thorn] completed work on PR ##{pr_number}")

      Store.record_event("fixer", "fixer_complete", %{pr_number: pr_number})
    end

    %{state | in_flight: in_flight}
  end

  defp extract_issue_body(pr) do
    # PR body typically contains the issue spec; use it as context
    pr["body"] || ""
  end

  defp workspace_for_branch(repo, _branch) do
    Workspace.repo_root(repo)
  end

  defp schedule_poll(_, delay) do
    Process.send_after(self(), :poll, delay)
  end

  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
  defp workspace_mod, do: Application.get_env(:conductor, :workspace_module, Workspace)
end
