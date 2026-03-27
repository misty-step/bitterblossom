defmodule Conductor.GitHub do
  @moduledoc """
  GitHub operations via the `gh` CLI.

  GitHub operations via the `gh` CLI. Used by infrastructure (fleet, health)
  and available to agents via the CLI directly.
  """

  alias Conductor.Shell
  require Logger

  @default_labeled_limit 25
  @default_unfiltered_limit 1000
  @issues_page_size 100

  @spec get_issue(binary(), pos_integer()) :: {:ok, map()} | {:error, term()}
  def get_issue(repo, number) do
    case Shell.cmd("gh", [
           "issue",
           "view",
           to_string(number),
           "--repo",
           repo,
           "--json",
           "number,title,body,url,labels,state"
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

  @spec list_issues(binary(), keyword()) :: {:ok, [map()]} | {:error, term()}
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
          {:ok, list}
        end
    end
  end

  @spec list_eligible(binary(), keyword()) :: [map()]
  def list_eligible(repo, opts \\ []), do: eligible_issues(repo, opts)

  @spec eligible_issues(binary(), keyword()) :: [map()]
  def eligible_issues(repo, opts \\ []) do
    case list_issues(repo, opts) do
      {:ok, issues} -> Enum.sort_by(issues, & &1["number"])
      {:error, reason} ->
        Logger.warning("failed to list issues: #{inspect(reason)}")
        []
    end
  end

  defp list_issue_args(repo, opts) do
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
      "number,title,body,url,labels,state",
      "--limit",
      to_string(limit)
    ] ++ maybe_label_filter(label)
  end

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
      empty_page_budget = ceil(limit / @issues_page_size)
      fetch_issue_pages(owner, name, 1, limit, empty_page_budget, empty_page_budget, [])
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

  defp fetch_issue_pages(
         _owner,
         _name,
         _page,
         remaining,
         _max_empty_pages,
         _empty_pages_left,
         acc
       )
       when remaining <= 0 do
    {:ok, Enum.reverse(acc)}
  end

  defp fetch_issue_pages(
         _owner,
         _name,
         _page,
         _remaining,
         _max_empty_pages,
         empty_pages_left,
         acc
       )
       when empty_pages_left <= 0 do
    {:ok, Enum.reverse(acc)}
  end

  defp fetch_issue_pages(owner, name, page, remaining, max_empty_pages, empty_pages_left, acc) do
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

      if issues == [] do
        {:ok, Enum.reverse(acc)}
      else
        fetch_issue_pages(
          owner,
          name,
          page + 1,
          remaining - length(page_issues),
          max_empty_pages,
          next_empty_page_budget(max_empty_pages, empty_pages_left, page_issues),
          Enum.reverse(page_issues) ++ acc
        )
      end
    end
  end

  defp next_empty_page_budget(_max_empty_pages, empty_pages_left, []), do: empty_pages_left - 1

  defp next_empty_page_budget(max_empty_pages, _empty_pages_left, _page_issues),
    do: max_empty_pages

  defp repo_parts(repo) do
    case String.split(repo, "/") do
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

  @spec ci_status(binary(), pos_integer()) :: {:ok, map()} | {:error, term()}
  def ci_status(repo, pr_number) do
    case get_pr_checks(repo, pr_number) do
      {:ok, checks} -> {:ok, summarize_checks(checks)}
      {:error, _reason} = error -> error
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

  @doc false
  @spec summarize_checks([map()]) :: map()
  def summarize_checks(checks) do
    normalized =
      checks
      |> Enum.map(&normalize_check/1)
      |> Enum.reject(&ignored_check?/1)

    pending = Enum.filter(normalized, &pending_check?/1)
    failed = Enum.filter(normalized, &failed_check?/1)

    state =
      cond do
        normalized == [] -> :unknown
        failed != [] -> :failed
        pending != [] -> :pending
        Enum.all?(normalized, &green_check?/1) -> :green
        true -> :unknown
      end

    %{
      state: state,
      checks: normalized,
      pending: pending,
      failed: failed,
      summary: summarize_check_state(state, normalized, pending, failed)
    }
  end

  defp normalize_check(check) do
    status_context_state = normalize_check_value(check["state"])

    %{
      name:
        first_present([
          check["name"],
          check["context"],
          check["displayName"],
          check["workflowName"]
        ])
        |> sanitize_check_name()
        |> case do
          nil -> "unnamed check"
          name -> name
        end,
      status:
        normalize_check_value(check["status"]) || status_context_status(status_context_state),
      conclusion:
        normalize_check_value(check["conclusion"]) ||
          status_context_conclusion(status_context_state),
      url:
        first_present([
          check["detailsUrl"],
          check["targetUrl"],
          check["url"]
        ])
        |> sanitize_check_url()
    }
  end

  defp normalize_check_value(nil), do: nil
  defp normalize_check_value(value) when is_binary(value), do: String.upcase(value)
  defp normalize_check_value(value), do: value |> to_string() |> String.upcase()

  defp first_present(values) do
    Enum.find(values, fn
      value when is_binary(value) -> value != ""
      _ -> false
    end)
  end

  defp sanitize_check_name(nil), do: nil

  defp sanitize_check_name(value) when is_binary(value) do
    value
    |> String.replace(~r/[\x00-\x1F\x7F]/u, " ")
    |> String.replace(~r/\s+/u, " ")
    |> String.trim()
    |> case do
      "" -> nil
      sanitized -> sanitized
    end
  end

  defp sanitize_check_url(nil), do: nil

  defp sanitize_check_url(value) when is_binary(value) do
    value
    |> String.replace(~r/[\x00-\x1F\x7F]/u, "")
    |> String.trim()
    |> case do
      "" -> nil
      sanitized -> sanitized
    end
  end

  defp status_context_status(state) when state in ["PENDING", "EXPECTED"], do: "PENDING"
  defp status_context_status(_state), do: nil

  defp status_context_conclusion("SUCCESS"), do: "SUCCESS"
  defp status_context_conclusion(state) when state in ["FAILURE", "ERROR"], do: "FAILURE"
  defp status_context_conclusion(_state), do: nil

  defp ignored_check?(check) do
    is_nil(check.conclusion) and check.status not in @active_statuses
  end

  defp pending_check?(check) do
    is_nil(check.conclusion) and check.status in @active_statuses
  end

  defp failed_check?(check) do
    not is_nil(check.conclusion) and check.conclusion in @failed
  end

  defp green_check?(check) do
    not is_nil(check.conclusion) and check.conclusion in @green
  end

  defp summarize_check_state(:green, checks, _pending, _failed) do
    "#{length(checks)} checks green"
  end

  defp summarize_check_state(:pending, _checks, pending, _failed) do
    "waiting on " <> Enum.map_join(pending, "; ", &format_check/1)
  end

  defp summarize_check_state(:failed, _checks, _pending, failed) do
    "failed checks: " <> Enum.map_join(failed, "; ", &format_check/1)
  end

  defp summarize_check_state(:unknown, _checks, _pending, _failed) do
    "no actionable CI signal yet"
  end

  defp format_check(check) do
    state = check.conclusion || check.status || "UNKNOWN"

    case check.url do
      nil -> "#{check.name} (#{state})"
      url -> "#{check.name} (#{state}) #{url}"
    end
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

  @doc "List all open PRs with CI status and labels."
  @spec open_prs(binary()) :: {:ok, [map()]} | {:error, term()}
  def open_prs(repo) do
    case Shell.cmd("gh", [
           "pr",
           "list",
           "--repo",
           repo,
           "--state",
           "open",
           "--limit",
           to_string(@default_unfiltered_limit),
           "--json",
           "number,title,body,headRefName,url,labels,mergeable,statusCheckRollup"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, prs} when is_list(prs) ->
            {:ok, Enum.filter(prs, &(is_map(&1) and is_binary(&1["headRefName"])))}

          {:ok, _other} ->
            {:error, "invalid JSON"}

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

  @doc "Remove a label from a PR."
  @spec remove_label(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def remove_label(repo, pr_number, label) do
    case Shell.cmd("gh", [
           "pr",
           "edit",
           to_string(pr_number),
           "--repo",
           repo,
           "--remove-label",
           label
         ]) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
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

  @doc "Close an issue."
  @spec close_issue(binary(), pos_integer()) :: :ok | {:error, term()}
  def close_issue(repo, issue_number) do
    case Shell.cmd("gh", [
           "issue",
           "close",
           to_string(issue_number),
           "--repo",
           repo
         ]) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @doc "Close a pull request without merging it."
  @spec close_pr(binary(), pos_integer(), keyword()) :: :ok | {:error, term()}
  def close_pr(repo, pr_number, opts \\ []) do
    args =
      [
        "pr",
        "close",
        to_string(pr_number),
        "--repo",
        repo,
        "--delete-branch=false"
      ] ++ if(comment = Keyword.get(opts, :comment), do: ["--comment", comment], else: [])

    case Shell.cmd("gh", args) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  # Convenience alias for create_issue_comment/3.
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

  @doc "Find the first open PR for an issue, optionally constrained to an exact branch."
  @spec find_open_pr(binary(), pos_integer(), binary() | nil) ::
          {:ok, map()} | {:error, :not_found | :api_error}
  def find_open_pr(repo, issue_number, expected_branch \\ nil) do
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
           "number,title,body,headRefName,url"
         ]) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, prs} when is_list(prs) ->
            case Enum.find(prs, &matching_open_pr?(&1, issue_number, expected_branch)) do
              nil -> {:error, :not_found}
              pr -> {:ok, pr}
            end

          {:ok, _other} ->
            {:error, :not_found}

          {:error, reason} ->
            Logger.warning("[github] failed to decode PR list: #{inspect(reason)}")
            {:error, :api_error}
        end

      {:error, msg, _} ->
        Logger.warning("[github] failed to list PRs: #{msg}")
        {:error, :api_error}
    end
  end

  @doc "List all open PRs that map to the issue by branch or closing keywords."
  @spec issue_open_prs(binary(), pos_integer()) :: {:ok, [map()]} | {:error, term()}
  def issue_open_prs(repo, issue_number) do
    case open_prs(repo) do
      {:ok, prs} -> {:ok, Enum.filter(prs, &pr_matches_issue?(&1, issue_number))}
      {:error, reason} -> {:error, reason}
    end
  end

  defp matching_open_pr?(pr, issue_number, nil) do
    pr_matches_issue?(pr, issue_number)
  end

  defp matching_open_pr?(pr, _issue_number, expected_branch) do
    pr["headRefName"] == expected_branch
  end

  defp pr_matches_issue?(pr, issue_number) do
    branch_matches_issue?(pr["headRefName"], issue_number) or
      body_closes_issue?(pr["body"], issue_number)
  end

  defp branch_matches_issue?(branch, issue_number) when is_binary(branch) do
    branch
    |> String.split("/")
    |> List.last()
    |> String.split("-", parts: 2)
    |> case do
      [issue_str | _] ->
        case Integer.parse(issue_str) do
          {^issue_number, ""} -> true
          _ -> false
        end

      _ ->
        false
    end
  end

  defp branch_matches_issue?(_, _issue_number), do: false

  defp body_closes_issue?(body, issue_number) when is_binary(body) do
    Regex.match?(
      ~r/\b(?:close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved)\s+(?:[[:alnum:]._-]+\/[[:alnum:]._-]+)?##{issue_number}\b/i,
      body
    )
  end

  defp body_closes_issue?(_, _issue_number), do: false

  @spec get_pr(binary(), pos_integer()) :: {:ok, map()} | {:error, term()}
  def get_pr(repo, pr_number) do
    case Shell.cmd("gh", [
           "pr",
           "view",
           to_string(pr_number),
           "--repo",
           repo,
           "--json",
           "number,title,state,merged,mergeable,headRefName,url"
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

  @spec pr_state(binary(), pos_integer()) :: {:ok, binary()} | {:error, term()}
  def pr_state(repo, pr_number) do
    case get_pr(repo, pr_number) do
      {:ok, %{"merged" => true}} -> {:ok, "MERGED"}
      {:ok, %{"state" => state}} -> {:ok, String.upcase(state)}
      {:error, reason} -> {:error, reason}
    end
  end
end
