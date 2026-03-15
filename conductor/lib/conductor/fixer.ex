defmodule Conductor.Fixer do
  @moduledoc """
  Polls for factory PRs with red CI and dispatches the fixer sprite.

  Single-responsibility: detect CI failures on open factory PRs,
  build a fixer prompt with failure context, and dispatch the fixer
  sprite to push a fix. Tracks in-flight dispatches to avoid
  double-fixing the same PR.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Store, Fleet}

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
    fixer_sprite = Keyword.get(opts, :fixer_sprite) || resolve_fixer_sprite()
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
  def handle_info({:DOWN, ref, :process, _pid, _reason}, state) do
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
    case code_host_mod().factory_prs(state.repo) do
      {:ok, prs} ->
        prs
        |> Enum.filter(&needs_fix?(&1, state))
        |> Enum.reduce(state, &dispatch_fixer(&2, &1))

      {:error, reason} ->
        Logger.warning("[fixer] failed to list factory PRs: #{reason}")
        state
    end
  end

  defp needs_fix?(pr, state) do
    pr_number = pr["number"]
    branch = pr["headRefName"] || ""

    String.starts_with?(branch, "factory/") and
      not Map.has_key?(state.in_flight, pr_number) and
      not code_host_mod().checks_green?(state.repo, pr_number)
  end

  defp dispatch_fixer(state, pr) do
    pr_number = pr["number"]
    Logger.info("[fixer] PR ##{pr_number} has red CI, dispatching fixer")

    # Fetch failure context
    {:ok, ci_logs} = code_host_mod().pr_ci_failure_logs(state.repo, pr_number)

    # Extract issue number from branch (factory/<issue_number>-<ts>)
    issue_body = extract_issue_body(pr)

    prompt = Prompt.build_fixer_prompt(pr, ci_logs, issue_body)
    branch = pr["headRefName"]

    Store.record_event("fixer", "fixer_dispatched", %{
      pr_number: pr_number,
      sprite: state.fixer_sprite
    })

    task =
      Task.async(fn ->
        worker_mod().dispatch(state.fixer_sprite, prompt, state.repo,
          timeout: Config.fixer_timeout(),
          workspace: workspace_for_branch(state.repo, branch)
        )
      end)

    %{state | in_flight: Map.put(state.in_flight, pr_number, task.ref)}
  end

  defp complete_task(state, ref) do
    {pr_number, in_flight} =
      Enum.reduce(state.in_flight, {nil, %{}}, fn {pr, r}, {found, acc} ->
        if r == ref, do: {pr, acc}, else: {found, Map.put(acc, pr, r)}
      end)

    if pr_number do
      Logger.info("[fixer] completed work on PR ##{pr_number}")

      Store.record_event("fixer", "fixer_complete", %{pr_number: pr_number})
    end

    %{state | in_flight: in_flight}
  end

  defp extract_issue_body(pr) do
    # PR body typically contains the issue spec; use it as context
    pr["body"] || ""
  end

  defp workspace_for_branch(repo, _branch) do
    repo_name = repo |> String.split("/") |> List.last()
    "/home/sprite/workspace/#{repo_name}"
  end

  defp resolve_fixer_sprite do
    case Fleet.by_role(:fixer) do
      [sprite | _] -> sprite
      [] -> "bb-fixer"
    end
  end

  defp schedule_poll(_, delay) do
    Process.send_after(self(), :poll, delay)
  end

  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
end
