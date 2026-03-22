defmodule Conductor.IssueLifecycle do
  @moduledoc """
  Issue lifecycle policy for resolved-but-open issues.
  """

  require Logger

  alias Conductor.Issue

  @spec issue_numbers([Issue.t()]) :: MapSet.t(pos_integer())
  def issue_numbers(issues) do
    issues
    |> Enum.map(& &1.number)
    |> MapSet.new()
  end

  @spec resolved_issues([Issue.t()], MapSet.t(pos_integer())) :: [Issue.t()]
  def resolved_issues(issues, resolved_issue_numbers) do
    Enum.filter(issues, &MapSet.member?(resolved_issue_numbers, &1.number))
  end

  @spec auto_closed_issue_numbers(
          binary(),
          [Issue.t()],
          MapSet.t(pos_integer()),
          (binary(), pos_integer() -> :ok | {:error, term()})
        ) :: MapSet.t(pos_integer())
  def auto_closed_issue_numbers(repo, issues, resolved_issue_numbers, close_issue_fun) do
    issues
    |> resolved_issues(resolved_issue_numbers)
    |> Enum.reduce(MapSet.new(), fn issue, closed_issue_numbers ->
      case close_issue_fun.(repo, issue.number) do
        :ok ->
          MapSet.put(closed_issue_numbers, issue.number)

        {:error, reason} ->
          Logger.warning(
            "[github] failed to auto-close issue ##{issue.number} resolved by a merged PR: #{inspect(reason)}"
          )

          closed_issue_numbers
      end
    end)
  end

  @spec reject_issue_numbers([Issue.t()], MapSet.t(pos_integer())) :: [Issue.t()]
  def reject_issue_numbers(issues, issue_numbers) do
    Enum.reject(issues, &MapSet.member?(issue_numbers, &1.number))
  end

  @spec resolved_issue_numbers_from_pr(map(), MapSet.t(pos_integer())) :: MapSet.t(pos_integer())
  def resolved_issue_numbers_from_pr(pr, remaining_issue_numbers) do
    branch_matches =
      case factory_issue_number_from_branch(pr_branch_name(pr)) do
        nil -> MapSet.new()
        issue_number -> MapSet.new([issue_number])
      end

    keyword_matches =
      pr
      |> pr_local_closing_issue_numbers()
      |> MapSet.intersection(remaining_issue_numbers)

    MapSet.union(branch_matches, keyword_matches)
  end

  defp pr_branch_name(%{"headRefName" => branch}) when is_binary(branch), do: branch
  defp pr_branch_name(%{"head" => %{"ref" => branch}}) when is_binary(branch), do: branch
  defp pr_branch_name(_pr), do: ""

  defp factory_issue_number_from_branch("factory/" <> rest) do
    case String.split(rest, "-", parts: 2) do
      [issue_number, _suffix] ->
        case Integer.parse(issue_number) do
          {value, ""} -> value
          _ -> nil
        end

      _ ->
        nil
    end
  end

  defp factory_issue_number_from_branch(_branch), do: nil

  defp pr_local_closing_issue_numbers(pr) do
    pr
    |> Map.get("body")
    |> to_string()
    |> then(fn body ->
      Regex.scan(
        ~r/\b(?:close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved)\s+#(\d+)\b/i,
        body,
        capture: :all_but_first
      )
    end)
    |> Enum.reduce(MapSet.new(), fn [issue_number], matches ->
      case Integer.parse(issue_number) do
        {value, ""} -> MapSet.put(matches, value)
        _ -> matches
      end
    end)
  end
end
