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

  @impl Conductor.Tracker
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

  @impl Conductor.Tracker
  @spec list_eligible(binary(), keyword()) :: [Issue.t()]
  def list_eligible(repo, opts \\ []) do
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

  @doc "Alias for backward compatibility."
  def eligible_issues(repo, opts \\ []), do: list_eligible(repo, opts)

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

  @impl Conductor.CodeHost
  @spec checks_green?(binary(), pos_integer()) :: boolean()
  def checks_green?(repo, pr_number) do
    case get_pr_checks(repo, pr_number) do
      {:ok, checks} -> evaluate_checks(checks)
      _ -> false
    end
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

  @impl Conductor.CodeHost
  @spec merge(binary(), pos_integer(), keyword()) :: :ok | {:error, term()}
  def merge(repo, pr_number, opts \\ []) do
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

  @doc "Alias for backward compatibility."
  def merge_pr(repo, pr_number, opts \\ []), do: merge(repo, pr_number, opts)

  @impl Conductor.Tracker
  @spec comment(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def comment(repo, issue_number, body) do
    create_issue_comment(repo, issue_number, body)
  end

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
