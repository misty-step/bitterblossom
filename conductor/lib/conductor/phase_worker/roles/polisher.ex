defmodule Conductor.PhaseWorker.Roles.Polisher do
  @moduledoc false

  @behaviour Conductor.PhaseWorker.Role

  require Logger

  alias Conductor.{Config, Prompt, Store}

  @impl true
  def role, do: :polisher

  @impl true
  def persona_role, do: :fern

  @impl true
  def event_prefix, do: "polisher"

  @impl true
  def find_work(repo, code_host), do: code_host.open_prs(repo)

  @impl true
  def eligible?(pr, _state) do
    labels = pr["labels"] || []
    label_names = Enum.map(labels, &String.downcase(&1["name"] || ""))
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)

    "lgtm" not in label_names and Conductor.GitHub.evaluate_checks(checks)
  end

  @impl true
  def enrich_context(pr, repo, code_host) do
    review_comments =
      case code_host.pr_review_comments(repo, work_ref(pr)) do
        {:ok, comments} ->
          comments

        {:error, reason} ->
          Logger.warning("[fern] failed to fetch reviews for PR ##{work_ref(pr)}: #{reason}")
          []
      end

    %{
      issue_body: pr["body"] || "",
      may_label: conductor_managed?(repo, work_ref(pr)),
      review_comments: review_comments
    }
  end

  @impl true
  def build_prompt(pr, context, opts) do
    Prompt.build_polisher_prompt(pr, context.review_comments, context.issue_body,
      may_label: context.may_label,
      workspace_root: Keyword.get(opts, :workspace_root)
    )
  end

  @impl true
  def dispatch_opts(_pr) do
    [timeout: Config.polisher_timeout(), harness_opts: [reasoning_effort: "high"]]
  end

  @impl true
  def work_ref(pr), do: pr["number"]

  @impl true
  def dispatch_log_message(pr), do: "PR ##{work_ref(pr)} is green, dispatching Fern"

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
end
