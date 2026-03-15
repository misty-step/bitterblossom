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

  @default_labeled_limit 25
  @default_unfiltered_limit 1000
  @issues_page_size 100

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

  @spec issue_has_label?(binary(), pos_integer(), binary()) :: {:ok, boolean()} | {:error, term()}
  def issue_has_label?(repo, issue_number, label) do
    case Shell.cmd("gh", [
           "issue",
           "view",
           to_string(issue_number),
           "--repo",
           repo,
           "--json",
           "labels"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} ->
            {:ok, label_present?(data, label)}

          {:error, _} ->
            {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @spec issue_comments(binary(), pos_integer()) :: {:ok, [map()]} | {:error, term()}
  def issue_comments(repo, issue_number) do
    case Shell.cmd("gh", [
           "issue",
           "view",
           to_string(issue_number),
           "--repo",
           repo,
           "--json",
           "comments"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} ->
            {:ok, data |> Map.get("comments") |> normalize_issue_comments()}

          {:error, _} ->
            {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
        end

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @spec list_issues(binary(), keyword()) :: {:ok, [Issue.t()]} | {:error, term()}
  def list_issues(repo, opts \\ []) do
    label = Keyword.get(opts, :label)
    explicit_limit? = Keyword.has_key?(opts, :limit)
    limit = Keyword.get(opts, :limit, default_issue_limit(label))

    case {normalized_label(label), explicit_limit?} do
      {nil, false} ->
        list_all_open_issues(repo, limit)

      {_label, _explicit_limit?} ->
        with {:ok, json} <- run_gh(list_issue_args(repo, opts)),
             {:ok, list} <- decode_issue_list(json) do
          {:ok, Enum.map(list, &Issue.from_github/1)}
        end
    end
  end

  # Conductor.Tracker callback — delegates to eligible_issues/2.
  @spec list_eligible(binary(), keyword()) :: [Issue.t()]
  def list_eligible(repo, opts \\ []), do: eligible_issues(repo, opts)

  @spec eligible_issues(binary(), keyword()) :: [Issue.t()]
  def eligible_issues(repo, opts \\ []) do
    case list_issues(repo, opts) do
      {:ok, issues} ->
        sort_eligible_issues(issues)

      {:error, reason} ->
        Logger.warning("failed to list issues: #{inspect(reason)}")
        []
    end
  end

  @doc false
  def list_issue_args(repo, opts \\ []) do
    label = Keyword.get(opts, :label)
    limit = Keyword.get(opts, :limit, default_issue_limit(label))

    [
      "issue",
      "list",
      "--repo",
      repo,
      "--state",
      "open",
      "--json",
      "number,title,body,url,labels",
      "--limit",
      to_string(limit)
    ] ++ maybe_label_filter(label)
  end

  @doc false
  def sort_eligible_issues(issues), do: Enum.sort_by(issues, & &1.number)

  def label_present?(data, label) do
    data
    |> Map.get("labels")
    |> List.wrap()
    |> Enum.any?(fn item -> item["name"] == label end)
  end

  @doc false
  def normalize_issue_comments(comments) do
    comments
    |> List.wrap()
    |> Enum.map(fn comment ->
      %{"body" => comment_body(comment)}
    end)
  end

  defp comment_body(%{"body" => body}) when is_binary(body), do: body

  defp comment_body(comment),
    do: get_in(comment, ["body", "text"]) || get_in(comment, ["body", "body"]) || ""

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

  defp maybe_label_filter(nil), do: []
  defp maybe_label_filter(""), do: []
  defp maybe_label_filter(label), do: ["--label", label]

  defp default_issue_limit(nil), do: @default_unfiltered_limit
  defp default_issue_limit(""), do: @default_unfiltered_limit
  defp default_issue_limit(_label), do: @default_labeled_limit

  defp normalized_label(nil), do: nil
  defp normalized_label(""), do: nil
  defp normalized_label(label), do: label

  defp list_all_open_issues(repo, limit) do
    with {:ok, {owner, name}} <- repo_parts(repo) do
      max_pages = ceil(limit / @issues_page_size)
      fetch_issue_pages(owner, name, 1, limit, max_pages, [])
    end
  end

  defp decode_issue_list(json) do
    case Jason.decode(json) do
      {:ok, list} when is_list(list) ->
        {:ok, list}

      {:ok, _other} ->
        {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}

      {:error, _reason} ->
        {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
    end
  end

  defp decode_issue_page(json) do
    case Jason.decode(json) do
      {:ok, page} when is_list(page) ->
        {:ok, page}

      {:ok, _other} ->
        {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}

      {:error, _reason} ->
        {:error, "invalid JSON from gh: #{String.slice(json, 0, 200)}"}
    end
  end

  defp fetch_issue_pages(_owner, _name, _page, remaining, _pages_left, acc) when remaining <= 0 do
    {:ok, Enum.reverse(acc)}
  end

  defp fetch_issue_pages(_owner, _name, _page, _remaining, pages_left, acc)
       when pages_left <= 0 do
    {:ok, Enum.reverse(acc)}
  end

  defp fetch_issue_pages(owner, name, page, remaining, pages_left, acc) do
    args = [
      "api",
      "repos/#{owner}/#{name}/issues?state=open&per_page=#{@issues_page_size}&page=#{page}"
    ]

    with {:ok, json} <- run_gh(args),
         {:ok, issues} <- decode_issue_page(json) do
      page_issues =
        issues
        |> Enum.reject(&Map.has_key?(&1, "pull_request"))
        |> Enum.take(remaining)
        |> Enum.map(&Issue.from_github/1)

      if issues == [] do
        {:ok, Enum.reverse(acc)}
      else
        fetch_issue_pages(
          owner,
          name,
          page + 1,
          remaining - length(page_issues),
          pages_left - 1,
          Enum.reverse(page_issues) ++ acc
        )
      end
    end
  end

  defp repo_parts(repo) do
    case String.split(repo, "/", trim: true) do
      [owner, name] when owner != "" and name != "" -> {:ok, {owner, name}}
      _ -> {:error, "expected repo in owner/name format, got: #{inspect(repo)}"}
    end
  end

  defp run_gh(args) do
    case Shell.cmd("gh", args) do
      {:ok, output} -> {:ok, output}
      {:error, msg, _code} -> {:error, msg}
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

  @doc "Find the first open PR whose branch starts with factory/<issue_number>-."
  @spec find_open_pr(binary(), pos_integer()) :: {:ok, map()} | {:error, :not_found}
  def find_open_pr(repo, issue_number) do
    case Shell.cmd("gh", [
           "pr",
           "list",
           "--repo",
           repo,
           "--state",
           "open",
           "--limit",
           "200",
           "--json",
           "number,title,headRefName,url"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, prs} ->
            prefix = "factory/#{issue_number}-"

            case Enum.find(prs, fn pr ->
                   String.starts_with?(pr["headRefName"] || "", prefix)
                 end) do
              nil -> {:error, :not_found}
              pr -> {:ok, pr}
            end

          {:error, reason} ->
            Logger.warning("[github] failed to decode PR list: #{inspect(reason)}")
            {:error, :not_found}
        end

      {:error, msg, _} ->
        Logger.warning("[github] failed to list PRs: #{msg}")
        {:error, :not_found}
    end
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
