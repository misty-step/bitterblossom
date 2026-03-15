defmodule Conductor.Polisher do
  @moduledoc """
  Polls for factory PRs with green CI (no `lgtm`) and dispatches the polisher sprite.

  Single-responsibility: detect review-ready factory PRs, build a polisher
  prompt with review context, and dispatch the polisher sprite to address
  feedback and apply `lgtm` when clean.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Store, Fleet}

  defstruct [
    :repo,
    :polisher_sprite,
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
    polisher_sprite = Keyword.get(opts, :polisher_sprite) || resolve_polisher_sprite()
    poll_ms = Keyword.get(opts, :poll_ms, Config.poll_seconds() * 1_000)

    state = %__MODULE__{
      repo: repo,
      polisher_sprite: polisher_sprite,
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
    Process.demonitor(ref, [:flush])
    state = complete_task(state, ref)
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, reason}, state) do
    if reason not in [:normal, :shutdown] do
      Logger.warning("[polisher] dispatch task exited: #{inspect(reason)}")
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
       polisher_sprite: state.polisher_sprite,
       in_flight: Map.keys(state.in_flight) |> Map.new(&{&1, :working})
     }, state}
  end

  # --- Private ---

  defp poll_and_dispatch(state) do
    # Skip if sprite is already working on a PR
    if map_size(state.in_flight) > 0 do
      state
    else
      case code_host_mod().factory_prs(state.repo) do
        {:ok, prs} ->
          # Dispatch at most one PR per poll (single sprite, single workspace)
          case Enum.find(prs, &needs_polish?(&1, state)) do
            nil -> state
            pr -> dispatch_polisher(state, pr)
          end

        {:error, reason} ->
          Logger.warning("[polisher] failed to list factory PRs: #{reason}")
          state
      end
    end
  end

  defp needs_polish?(pr, _state) do
    branch = pr["headRefName"] || ""
    labels = pr["labels"] || []
    label_names = Enum.map(labels, & &1["name"])
    checks = pr["statusCheckRollup"] || []

    String.starts_with?(branch, "factory/") and
      "lgtm" not in label_names and
      Conductor.GitHub.evaluate_checks(checks)
  end

  defp dispatch_polisher(state, pr) do
    pr_number = pr["number"]
    Logger.info("[polisher] PR ##{pr_number} is green, dispatching polisher")

    comments =
      case code_host_mod().pr_review_comments(state.repo, pr_number) do
        {:ok, comments} ->
          comments

        {:error, reason} ->
          Logger.warning("[polisher] failed to fetch reviews for PR ##{pr_number}: #{reason}")
          []
      end

    issue_body = pr["body"] || ""
    prompt = Prompt.build_polisher_prompt(pr, comments, issue_body)
    branch = pr["headRefName"]

    Store.record_event("polisher", "polisher_dispatched", %{
      pr_number: pr_number,
      sprite: state.polisher_sprite
    })

    task =
      Task.async(fn ->
        try do
          worker_mod().dispatch(state.polisher_sprite, prompt, state.repo,
            timeout: Config.polisher_timeout(),
            workspace: workspace_for_branch(state.repo, branch)
          )
        rescue
          e -> {:error, "polisher dispatch crashed: #{Exception.message(e)}", 1}
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
      Logger.info("[polisher] completed work on PR ##{pr_number}")

      Store.record_event("polisher", "polisher_complete", %{pr_number: pr_number})
    end

    %{state | in_flight: in_flight}
  end

  defp workspace_for_branch(repo, _branch) do
    repo_name = repo |> String.split("/") |> List.last()
    "/home/sprite/workspace/#{repo_name}"
  end

  defp resolve_polisher_sprite do
    case Fleet.by_role(:polisher) do
      [sprite | _] -> sprite
      [] -> "bb-polisher"
    end
  end

  defp schedule_poll(_, delay) do
    Process.send_after(self(), :poll, delay)
  end

  defp code_host_mod, do: Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
end
