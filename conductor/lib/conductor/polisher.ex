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

  @green_conclusions ~w(SUCCESS NEUTRAL SKIPPED success neutral skipped)
  @issue_body_reference ~r/\b(?:close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved)\s+(?:[[:alnum:]._-]+\/[[:alnum:]._-]+)?#(?<issue>\d+)\b/i
  @issue_branch_reference ~r/^(?:issue[-_])?(?<issue>\d+)(?:[-_].*)?$/

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
          log_duplicate_prs(prs)

          # Dispatch at most one PR per poll (single sprite, single workspace)
          case select_pr_to_polish(prs) do
            nil -> state
            pr -> dispatch_polisher(state, pr)
          end

        {:error, reason} ->
          Logger.warning("[fern] failed to list open PRs: #{reason}")
          state
      end
    end
  end

  defp needs_polish?(pr) do
    labels = pr["labels"] || []
    label_names = Enum.map(labels, &String.downcase(&1["name"] || ""))
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)

    "lgtm" not in label_names and Conductor.GitHub.evaluate_checks(checks)
  end

  defp select_pr_to_polish(prs) do
    prs
    |> collapse_duplicate_prs()
    |> Enum.find(&needs_polish?/1)
  end

  defp collapse_duplicate_prs(prs) do
    duplicate_groups = duplicate_issue_groups(prs)

    {collapsed, _seen_issue_numbers} =
      Enum.reduce(prs, {[], MapSet.new()}, fn pr, {acc, seen_issue_numbers} ->
        case referenced_issue_number(pr) do
          issue_number when is_integer(issue_number) ->
            if Map.has_key?(duplicate_groups, issue_number) do
              if MapSet.member?(seen_issue_numbers, issue_number) do
                {acc, seen_issue_numbers}
              else
                candidate =
                  select_best_duplicate_candidate(Map.fetch!(duplicate_groups, issue_number))

                {[candidate | acc], MapSet.put(seen_issue_numbers, issue_number)}
              end
            else
              {[pr | acc], seen_issue_numbers}
            end

          nil ->
            {[pr | acc], seen_issue_numbers}
        end
      end)

    Enum.reverse(collapsed)
  end

  defp log_duplicate_prs(prs) do
    Enum.each(duplicate_issue_groups(prs), fn {issue_number, issue_prs} ->
      chosen_pr = select_best_duplicate_candidate(issue_prs)

      pr_numbers =
        issue_prs
        |> Enum.map(&(&1["number"] || 0))
        |> Enum.sort()

      Logger.warning(
        "[fern] duplicate open PRs for issue ##{issue_number}: #{inspect(pr_numbers)}; selecting PR ##{chosen_pr["number"]}"
      )
    end)
  end

  defp duplicate_issue_groups(prs) do
    prs
    |> Enum.reduce(%{}, fn pr, acc ->
      case referenced_issue_number(pr) do
        issue_number when is_integer(issue_number) ->
          Map.update(acc, issue_number, [pr], &[pr | &1])

        nil ->
          acc
      end
    end)
    |> Enum.reduce(%{}, fn {issue_number, issue_prs}, acc ->
      issue_prs = Enum.reverse(issue_prs)

      if length(issue_prs) > 1 do
        Map.put(acc, issue_number, issue_prs)
      else
        acc
      end
    end)
  end

  defp select_best_duplicate_candidate(issue_prs) do
    Enum.max_by(issue_prs, &duplicate_candidate_score/1)
  end

  defp duplicate_candidate_score(pr) do
    {
      if(needs_polish?(pr), do: 1, else: 0),
      latest_green_check_timestamp(pr),
      commit_count(pr),
      parse_timestamp(pr["updatedAt"]),
      pr["number"] || 0
    }
  end

  defp latest_green_check_timestamp(pr) do
    pr
    |> Map.get("statusCheckRollup", [])
    |> List.wrap()
    |> Enum.filter(&green_check?/1)
    |> Enum.map(&(parse_timestamp(&1["completedAt"]) || parse_timestamp(&1["startedAt"])))
    |> Enum.reject(&is_nil/1)
    |> Enum.max(fn -> 0 end)
  end

  defp green_check?(check) do
    is_map(check) and check["conclusion"] in @green_conclusions
  end

  defp commit_count(%{"commits" => commits}) when is_list(commits), do: length(commits)

  defp commit_count(%{"commits" => %{"totalCount" => total_count}}) when is_integer(total_count),
    do: total_count

  defp commit_count(_pr), do: 0

  defp parse_timestamp(value) when is_binary(value) do
    case DateTime.from_iso8601(value) do
      {:ok, datetime, _offset} -> DateTime.to_unix(datetime)
      _ -> nil
    end
  end

  defp parse_timestamp(_value), do: nil

  defp referenced_issue_number(pr) do
    branch_issue_number(pr["headRefName"]) || body_issue_number(pr["body"])
  end

  defp branch_issue_number(branch) when is_binary(branch) do
    branch
    |> String.split("/")
    |> List.last()
    |> case do
      branch_name when is_binary(branch_name) ->
        case Regex.named_captures(@issue_branch_reference, branch_name) do
          %{"issue" => issue_number} -> parse_issue_number(issue_number)
          _ -> nil
        end

      _ ->
        nil
    end
  end

  defp branch_issue_number(_branch), do: nil

  defp body_issue_number(body) when is_binary(body) do
    case Regex.named_captures(@issue_body_reference, body) do
      %{"issue" => issue_number} -> parse_issue_number(issue_number)
      _ -> nil
    end
  end

  defp body_issue_number(_body), do: nil

  defp parse_issue_number(value) when is_binary(value) do
    case Integer.parse(value) do
      {issue_number, ""} -> issue_number
      _ -> nil
    end
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
