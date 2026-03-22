defmodule Conductor.PhaseWorker.Roles.Fixer do
  @moduledoc false

  @behaviour Conductor.PhaseWorker.Role

  require Logger

  alias Conductor.{Config, Prompt}

  @impl true
  def role, do: :fixer

  @impl true
  def persona_role, do: :thorn

  @impl true
  def event_prefix, do: "fixer"

  @impl true
  def find_work(repo, code_host), do: code_host.open_prs(repo)

  @impl true
  def eligible?(pr, _state) do
    checks = pr["statusCheckRollup"] |> List.wrap() |> Enum.filter(&is_map/1)
    Conductor.GitHub.evaluate_checks_failed(checks)
  end

  @impl true
  def enrich_context(pr, repo, code_host) do
    ci_logs =
      case code_host.pr_ci_failure_logs(repo, work_ref(pr)) do
        {:ok, logs} ->
          logs

        {:error, reason} ->
          Logger.warning("[thorn] failed to fetch CI logs for PR ##{work_ref(pr)}: #{reason}")
          "(CI logs unavailable)"
      end

    %{ci_logs: ci_logs, issue_body: pr["body"] || ""}
  end

  @impl true
  def build_prompt(pr, context, opts) do
    Prompt.build_fixer_prompt(pr, context.ci_logs, context.issue_body, opts)
  end

  @impl true
  def dispatch_opts(_pr), do: [timeout: Config.fixer_timeout()]

  @impl true
  def work_ref(pr), do: pr["number"]

  @impl true
  def dispatch_log_message(pr), do: "PR ##{work_ref(pr)} has red CI, dispatching Thorn"
end
