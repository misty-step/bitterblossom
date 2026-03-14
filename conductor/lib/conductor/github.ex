defmodule Conductor.GitHub do
  @moduledoc """
  GitHub operations via the `gh` CLI.

  Deep module: hides all GitHub API details, argument construction,
  and JSON parsing. Callers see Elixir structs and maps.
  """

  alias Conductor.{Shell, Issue}

  @spec get_issue(binary(), pos_integer()) :: {:ok, Issue.t()} | {:error, term()}
  def get_issue(repo, number) do
    case Shell.cmd("gh", [
           "issue", "view", to_string(number),
           "--repo", repo,
           "--json", "number,title,body,url,labels"
         ]) do
      {:ok, json} -> {:ok, Issue.from_github(Jason.decode!(json))}
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec list_issues(binary(), keyword()) :: {:ok, [Issue.t()]} | {:error, term()}
  def list_issues(repo, opts \\ []) do
    label = Keyword.get(opts, :label, "autopilot")
    limit = Keyword.get(opts, :limit, 25)

    case Shell.cmd("gh", [
           "issue", "list",
           "--repo", repo,
           "--label", label,
           "--state", "open",
           "--json", "number,title,body,url,labels",
           "--limit", to_string(limit)
         ]) do
      {:ok, json} ->
        issues = json |> Jason.decode!() |> Enum.map(&Issue.from_github/1)
        {:ok, issues}

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @spec eligible_issues(binary(), keyword()) :: [Issue.t()]
  def eligible_issues(repo, opts \\ []) do
    {:ok, issues} = list_issues(repo, opts)

    issues
    |> Enum.filter(fn issue -> Issue.ready?(issue) == :ok end)
    |> Enum.sort_by(& &1.number)
  end

  @spec get_pr_checks(binary(), pos_integer()) :: {:ok, [map()]} | {:error, term()}
  def get_pr_checks(repo, pr_number) do
    case Shell.cmd("gh", [
           "pr", "view", to_string(pr_number),
           "--repo", repo,
           "--json", "statusCheckRollup"
         ]) do
      {:ok, json} ->
        checks = json |> Jason.decode!() |> Map.get("statusCheckRollup", [])
        {:ok, checks}

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @spec checks_green?(binary(), pos_integer()) :: boolean()
  def checks_green?(repo, pr_number) do
    case get_pr_checks(repo, pr_number) do
      {:ok, checks} ->
        Enum.all?(checks, fn c ->
          c["conclusion"] in ["SUCCESS", "success", "NEUTRAL", "neutral", "SKIPPED", "skipped"]
        end)

      _ ->
        false
    end
  end

  @spec merge_pr(binary(), pos_integer(), keyword()) :: :ok | {:error, term()}
  def merge_pr(repo, pr_number, opts \\ []) do
    method = Keyword.get(opts, :method, "squash")
    delete_branch = if Keyword.get(opts, :delete_branch, true), do: ["--delete-branch"], else: []

    case Shell.cmd("gh", [
           "pr", "merge", to_string(pr_number),
           "--repo", repo,
           "--#{method}"
         ] ++ delete_branch) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec create_issue_comment(binary(), pos_integer(), binary()) :: :ok | {:error, term()}
  def create_issue_comment(repo, issue_number, body) do
    tmp = Path.join(System.tmp_dir!(), "conductor-comment-#{:rand.uniform(999_999)}.md")
    File.write!(tmp, body)

    result =
      case Shell.cmd("gh", [
             "issue", "comment", to_string(issue_number),
             "--repo", repo,
             "--body-file", tmp
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
           "pr", "view", to_string(pr_number),
           "--repo", repo,
           "--json", "number,title,state,mergeable,headRefName,url"
         ]) do
      {:ok, json} -> {:ok, Jason.decode!(json)}
      {:error, msg, _} -> {:error, msg}
    end
  end
end
