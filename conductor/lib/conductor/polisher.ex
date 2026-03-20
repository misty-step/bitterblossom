defmodule Conductor.Polisher do
  @moduledoc """
  Polls for open PRs with green CI (no `lgtm`) and dispatches the polisher sprite.

  The polisher sprite does judgment work (address feedback, simplify code, run tests)
  and labels `lgtm` when the PR is genuinely merge-ready. The conductor then verifies
  the label + CI green and merges.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Store}

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
    polisher_sprite = Keyword.fetch!(opts, :polisher_sprite)
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
  def handle_info({ref, result}, state) when is_reference(ref) do
    Process.demonitor(ref, [:flush])
    state = complete_task(state, ref, result)
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, reason}, state) do
    if reason not in [:normal, :shutdown] do
      Logger.warning("[fern] dispatch task exited: #{inspect(reason)}")
    end

    state = complete_task(state, ref, {:error, reason})
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

  defp needs_polish?(pr, state) do
    labels = pr["labels"] || []
    label_names = Enum.map(labels, &String.downcase(&1["name"] || ""))
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)

    "lgtm" not in label_names and
      Conductor.GitHub.evaluate_checks(checks) and
      polish_eligible?(state.repo, pr["number"])
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
    prompt = Prompt.build_polisher_prompt(pr, comments, issue_body, may_label: conductor_managed)
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
            workspace: workspace_for_branch(state.repo, branch),
            harness_opts: [reasoning_effort: "high"]
          )
        rescue
          e -> {:error, "polisher dispatch crashed: #{Exception.message(e)}", 1}
        end
      end)

    %{state | in_flight: Map.put(state.in_flight, pr_number, task.ref)}
  end

  defp complete_task(state, ref, result) do
    {pr_number, in_flight} =
      Enum.reduce(state.in_flight, {nil, %{}}, fn {pr, r}, {found, acc} ->
        if r == ref, do: {pr, acc}, else: {found, Map.put(acc, pr, r)}
      end)

    if pr_number do
      maybe_mark_polished(state.repo, pr_number, result)
      Logger.info("[fern] completed work on PR ##{pr_number}")
      Store.record_event("polisher", "polisher_complete", %{pr_number: pr_number})
    end

    %{state | in_flight: in_flight}
  end

  defp polish_eligible?(repo, pr_number) when is_integer(pr_number) do
    with {:ok, substantive_change_at} <- code_host_mod().pr_substantive_change_at(repo, pr_number),
         :ok <-
           Store.upsert_pr_state(repo, pr_number, %{
             last_substantive_change_at: substantive_change_at
           }),
         {:ok, pr_state} <- Store.get_pr_state(repo, pr_number) do
      needs_polish_after_change?(pr_state)
    else
      {:error, reason} ->
        Logger.warning(
          "[fern] failed to evaluate polish state for PR ##{pr_number}: #{inspect(reason)}"
        )

        true
    end
  end

  defp polish_eligible?(_repo, _pr_number), do: true

  defp needs_polish_after_change?(%{"polished_at" => nil}), do: true
  defp needs_polish_after_change?(%{"last_substantive_change_at" => nil}), do: true

  defp needs_polish_after_change?(%{
         "polished_at" => polished_at,
         "last_substantive_change_at" => last_substantive_change_at
       }) do
    case compare_timestamps(polished_at, last_substantive_change_at) do
      :lt -> true
      :eq -> false
      :gt -> false
      :error -> true
    end
  end

  defp maybe_mark_polished(repo, pr_number, {:ok, _output}) do
    Store.mark_pr_polished(repo, pr_number)
  end

  defp maybe_mark_polished(_repo, _pr_number, _result), do: :ok

  defp compare_timestamps(left, right) when is_binary(left) and is_binary(right) do
    with {:ok, left_dt, _} <- DateTime.from_iso8601(left),
         {:ok, right_dt, _} <- DateTime.from_iso8601(right) do
      DateTime.compare(left_dt, right_dt)
    else
      _ -> :error
    end
  end

  defp compare_timestamps(_left, _right), do: :error

  defp workspace_for_branch(repo, _branch) do
    repo_name = repo |> String.split("/") |> List.last()
    "/home/sprite/workspace/#{repo_name}"
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
end
