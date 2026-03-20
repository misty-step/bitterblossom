defmodule Conductor.Polisher do
  @moduledoc """
  Polls for open PRs with green CI (no `lgtm`) and dispatches the polisher sprite.

  The polisher sprite does judgment work (address feedback, simplify code, run tests)
  and labels `lgtm` when the PR is genuinely merge-ready. The conductor then verifies
  the label + CI green and merges.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Store, Workspace}

  defstruct [
    :repo,
    :polisher_sprite,
    :poll_ms,
    :trusted_review_authors,
    in_flight: %{},
    lgtm_pending: MapSet.new()
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

    trusted_review_authors =
      Keyword.get(opts, :trusted_review_authors, Config.trusted_review_authors())

    state = %__MODULE__{
      repo: repo,
      polisher_sprite: polisher_sprite,
      poll_ms: poll_ms,
      trusted_review_authors: trusted_review_authors
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
      Logger.warning("[fern] dispatch task exited: #{inspect(reason)}")
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
       trusted_review_authors: state.trusted_review_authors,
       lgtm_pending: MapSet.to_list(state.lgtm_pending),
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
          state = reconcile_lgtm_pending(state, prs)

          case Enum.find_value(prs, &next_action(&1, state)) do
            nil -> state
            {:auto_label, pr, review_state} -> auto_label_ready_pr(state, pr, review_state)
            {:dispatch, pr, review_state} -> dispatch_polisher(state, pr, review_state)
          end

        {:error, reason} ->
          Logger.warning("[fern] failed to list open PRs: #{reason}")
          state
      end
    end
  end

  defp next_action(pr, state) do
    labels = pr["labels"] || []
    label_names = Enum.map(labels, &String.downcase(&1["name"] || ""))
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)
    pr_number = pr["number"]

    if "lgtm" in label_names or MapSet.member?(state.lgtm_pending, pr_number) or
         not Conductor.GitHub.evaluate_checks(checks) do
      nil
    else
      conductor_managed = conductor_managed?(state.repo, pr_number)

      case review_state(state.repo, pr_number, state.trusted_review_authors) do
        {:ok, %{actionable: [], non_blocking: [_ | _]} = review_state} ->
          if conductor_managed do
            {:auto_label, pr, review_state}
          else
            nil
          end

        {:ok, review_state} ->
          {:dispatch, pr, review_state}

        {:error, reason} ->
          Logger.warning("[fern] failed to fetch review threads for PR ##{pr_number}: #{reason}")
          {:dispatch, pr, empty_review_state()}
      end
    end
  end

  defp auto_label_ready_pr(state, pr, review_state) do
    pr_number = pr["number"]

    Logger.info(
      "[fern] PR ##{pr_number} only has non-blocking trusted external threads, adding lgtm"
    )

    case code_host_mod().add_label(state.repo, pr_number, "lgtm") do
      :ok ->
        Store.record_event("polisher", "polisher_auto_lgtm", %{
          pr_number: pr_number,
          non_blocking_review_threads: length(review_state.non_blocking)
        })

        %{state | lgtm_pending: MapSet.put(state.lgtm_pending, pr_number)}

      {:error, reason} ->
        Logger.warning("[fern] failed to add lgtm to PR ##{pr_number}: #{reason}")
        state
    end
  end

  defp dispatch_polisher(state, pr, review_state) do
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
        workspace_root: workspace,
        actionable_review_threads: review_state.actionable,
        non_blocking_review_threads: review_state.non_blocking
      )

    Store.record_event("polisher", "polisher_dispatched", %{
      pr_number: pr_number,
      sprite: state.polisher_sprite
    })

    task =
      Task.async(fn ->
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

    %{state | in_flight: Map.put(state.in_flight, pr_number, task.ref)}
  end

  defp review_state(repo, pr_number, trusted_review_authors) do
    case code_host_mod().pr_review_threads(repo, pr_number) do
      {:ok, threads} ->
        {:ok, Conductor.GitHub.classify_review_threads(threads, trusted_review_authors)}

      {:error, _reason} = error ->
        error
    end
  end

  defp empty_review_state, do: %{actionable: [], non_blocking: []}

  defp reconcile_lgtm_pending(state, prs) do
    still_pending =
      Enum.reduce(prs, MapSet.new(), fn pr, acc ->
        label_names =
          pr["labels"]
          |> List.wrap()
          |> Enum.map(&String.downcase(&1["name"] || ""))

        pr_number = pr["number"]

        if MapSet.member?(state.lgtm_pending, pr_number) and "lgtm" not in label_names do
          MapSet.put(acc, pr_number)
        else
          acc
        end
      end)

    %{state | lgtm_pending: still_pending}
  end

  defp complete_task(state, ref) do
    {pr_number, in_flight} =
      Enum.reduce(state.in_flight, {nil, %{}}, fn {pr, r}, {found, acc} ->
        if r == ref, do: {pr, acc}, else: {found, Map.put(acc, pr, r)}
      end)

    if pr_number do
      Logger.info("[fern] completed work on PR ##{pr_number}")
      Store.record_event("polisher", "polisher_complete", %{pr_number: pr_number})
    end

    %{state | in_flight: in_flight}
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
