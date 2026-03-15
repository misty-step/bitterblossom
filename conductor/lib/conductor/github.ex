defmodule Conductor.GitHub do
  @moduledoc """
  GitHub operations via the `gh` CLI.

  Deep module: hides all GitHub API details, argument construction,
  and JSON parsing. Callers see Elixir structs and maps.

  Implements `Conductor.Tracker` and `Conductor.CodeHost`.
  """

  @behaviour Conductor.Tracker
  @behaviour Conductor.CodeHost

  alias Conductor.{Shell, Issue}
  require Logger

  @spec get_issue(binary(), pos_integer()) :: {:ok, Issue.t()} | {:error, term()}
  def get_issue(repo, number) do
    case Shell.cmd("gh", [
           "issue",
           "view",
           to_string(number),
           "--repo",
           repo,
           "--json",
           "number,title,body,url,labels"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} -> {:ok, Issue.from_github(data)}
          {:error, _} -> {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @spec list_issues(binary(), keyword()) :: {:ok, [Issue.t()]} | {:error, term()}
  def list_issues(repo, opts \\ []) do
    label = Keyword.get(opts, :label, "autopilot")
    limit = Keyword.get(opts, :limit, 25)

    case Shell.cmd("gh", [
           "issue",
           "list",
           "--repo",
           repo,
           "--label",
           label,
           "--state",
           "open",
           "--json",
           "number,title,body,url,labels",
           "--limit",
           to_string(limit)
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, list} -> {:ok, Enum.map(list, &Issue.from_github/1)}
          {:error, _} -> {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  # Conductor.Tracker callback — delegates to eligible_issues/2.
  @spec list_eligible(binary(), keyword()) :: [Issue.t()]
  def list_eligible(repo, opts \\ []), do: eligible_issues(repo, opts)

  @spec eligible_issues(binary(), keyword()) :: [Issue.t()]
  def eligible_issues(repo, opts \\ []) do
    case list_issues(repo, opts) do
      {:ok, issues} ->
        issues
        |> Enum.filter(fn issue -> Issue.ready?(issue) == :ok end)
        |> Enum.sort_by(& &1.number)

      {:error, reason} ->
        Logger.warning("failed to list issues: #{inspect(reason)}")
        []
    end
  end

  @spec get_pr_checks(binary(), pos_integer()) :: {:ok, [map()]} | {:error, term()}
  def get_pr_checks(repo, pr_number) do
    case Shell.cmd("gh", [
           "pr",
           "view",
           to_string(pr_number),
           "--repo",
           repo,
           "--json",
           "statusCheckRollup"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} -> {:ok, Map.get(data, "statusCheckRollup", [])}
          {:error, _} -> {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @green ~w(SUCCESS success NEUTRAL neutral SKIPPED skipped)

  @spec checks_green?(binary(), pos_integer()) :: boolean()
  def checks_green?(repo, pr_number) do
    case get_pr_checks(repo, pr_number) do
      {:ok, checks} -> evaluate_checks(checks)
      _ -> false
    end
  end

  @failed ~w(FAILURE failure ERROR error CANCELLED cancelled TIMED_OUT timed_out ACTION_REQUIRED action_required STALE stale STARTUP_FAILURE startup_failure)

  @spec checks_failed?(binary(), pos_integer()) :: boolean()
  def checks_failed?(repo, pr_number) do
    case get_pr_checks(repo, pr_number) do
      {:ok, checks} -> evaluate_checks_failed(checks)
      _ -> false
    end
  end

  @doc """
  Return true when at least one completed check has a non-green conclusion.
  Returns false for pending/queued/no-checks states.
  """
  @spec evaluate_checks_failed([map()]) :: boolean()
  def evaluate_checks_failed(checks) do
    Enum.any?(checks, fn c ->
      not is_nil(c["conclusion"]) and c["conclusion"] in @failed
    end)
  end

  @active_statuses ~w(IN_PROGRESS QUEUED PENDING WAITING REQUESTED in_progress queued pending waiting requested)

  @doc """
  Pure evaluation of a check list. Distinguishes three categories:

  1. Completed checks (non-nil conclusion) — must all be in @green
  2. In-progress checks (nil conclusion but active status) — block merge
  3. Annotations (nil conclusion AND nil/inactive status) — ignored

  Returns false when no real checks remain or any are still running.
  """
  @spec evaluate_checks([map()]) :: boolean()
  def evaluate_checks(checks) do
    real =
      Enum.filter(checks, fn c ->
        not is_nil(c["conclusion"]) or c["status"] in @active_statuses
      end)

    pending = Enum.any?(real, fn c -> is_nil(c["conclusion"]) end)
    real != [] and not pending and Enum.all?(real, fn c -> c["conclusion"] in @green end)
  end

  # Conductor.CodeHost callback — delegates to merge_pr/3.
  @spec merge(binary(), pos_integer(), keyword()) :: :ok | {:error, term()}
  def merge(repo, pr_number, opts \\ []), do: merge_pr(repo, pr_number, opts)

  @spec merge_pr(binary(), pos_integer(), keyword()) :: :ok | {:error, term()}
  def merge_pr(repo, pr_number, opts \\ []) do
    method = Keyword.get(opts, :method, "squash")
    delete_branch = if Keyword.get(opts, :delete_branch, true), do: ["--delete-branch"], else: []

    case Shell.cmd(
           "gh",
           [
             "pr",
             "merge",
             to_string(pr_number),
             "--repo",
             repo,
             "--#{method}"
           ] ++ delete_branch
         ) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @doc "List open PRs with a specific label."
  @spec labeled_prs(binary(), binary()) :: {:ok, [map()]} | {:error, term()}
  def labeled_prs(repo, label) do
    case Shell.cmd("gh", [
           "pr",
           "list",
           "--repo",
           repo,
           "--state",
           "open",
           "--label",
           label,
           "--json",
           "number,title,headRefName"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, prs} -> {:ok, prs}
          {:error, _} -> {:error, "invalid JSON"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @doc "List open factory/* PRs with CI status and labels."
  @spec factory_prs(binary()) :: {:ok, [map()]} | {:error, term()}
  def factory_prs(repo) do
    case Shell.cmd("gh", [
           "pr",
           "list",
           "--repo",
           repo,
           "--state",
           "open",
           "--json",
           "number,title,body,headRefName,labels,statusCheckRollup"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, prs} ->
            factory =
              Enum.filter(prs, fn pr ->
                branch = pr["headRefName"] || ""
                String.starts_with?(branch, "factory/")
              end)

            {:ok, factory}

          {:error, _} ->
            {:error, "invalid JSON"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @doc "Fetch review comments on a PR."
  @spec pr_review_comments(binary(), pos_integer()) :: {:ok, [map()]} | {:error, term()}
  def pr_review_comments(repo, pr_number) do
    case Shell.cmd("gh", [
           "pr",
           "view",
           to_string(pr_number),
           "--repo",
           repo,
           "--json",
           "reviews,comments"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} ->
            reviews = Map.get(data, "reviews", [])
            comments = Map.get(data, "comments", [])
            {:ok, reviews ++ comments}

          {:error, _} ->
            {:error, "invalid JSON"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @doc "Fetch CI failure logs for a PR."
  @spec pr_ci_failure_logs(binary(), pos_integer()) :: {:ok, binary()} | {:error, term()}
  def pr_ci_failure_logs(repo, pr_number) do
    case Shell.cmd("gh", [
           "pr",
           "checks",
           to_string(pr_number),
           "--repo",
           repo
         ]) do
      {:ok, output} -> {:ok, output}
      {:error, output, _} -> {:ok, output}
    end
  end

  @doc "Add a label to a PR."
  @spec add_label(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def add_label(repo, pr_number, label) do
    case Shell.cmd("gh", [
           "pr",
           "edit",
           to_string(pr_number),
           "--repo",
           repo,
           "--add-label",
           label
         ]) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  # Conductor.Tracker callback — delegates to create_issue_comment/3.
  @spec comment(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def comment(repo, issue_number, body), do: create_issue_comment(repo, issue_number, body)

  @spec create_issue_comment(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def create_issue_comment(repo, issue_number, body) do
    tmp = Path.join(System.tmp_dir!(), "conductor-comment-#{:rand.uniform(999_999)}.md")
    File.write!(tmp, body)

    result =
      case Shell.cmd("gh", [
             "issue",
             "comment",
             to_string(issue_number),
             "--repo",
             repo,
             "--body-file",
             tmp
           ]) do
        {:ok, _} -> :ok
        {:error, msg, _} -> {:error, msg}
      end

    File.rm(tmp)
    result
  end

  @spec update_issue_body(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def update_issue_body(repo, issue_number, body) do
    tmp = Path.join(System.tmp_dir!(), "conductor-body-#{System.unique_integer([:positive])}.md")
    File.write!(tmp, body)

    result =
      case Shell.cmd("gh", [
             "issue",
             "edit",
             to_string(issue_number),
             "--repo",
             repo,
             "--body-file",
             tmp
           ]) do
        {:ok, _} -> :ok
        {:error, msg, _} -> {:error, msg}
      end

    File.rm(tmp)
    result
  end

  @spec get_pr(binary(), pos_integer()) :: {:ok, map()} | {:error, term()}
  def get_pr(repo, pr_number) do
    case Shell.cmd("gh", [
           "pr",
           "view",
           to_string(pr_number),
           "--repo",
           repo,
           "--json",
           "number,title,state,mergeable,headRefName,url"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} -> {:ok, data}
          {:error, _} -> {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end
end
