defmodule Conductor.IssueLifecycleTest do
  use ExUnit.Case, async: true
  import ExUnit.CaptureLog

  alias Conductor.{Issue, IssueLifecycle}

  describe "issue_numbers/1" do
    test "collects issue numbers into a set" do
      assert IssueLifecycle.issue_numbers([issue(10), issue(11), issue(10)]) ==
               MapSet.new([10, 11])
    end
  end

  describe "resolved_issues/2" do
    test "selects only issues present in the resolved set" do
      assert IssueLifecycle.resolved_issues([issue(10), issue(11), issue(12)], MapSet.new([11])) ==
               [issue(11)]
    end
  end

  describe "reject_issue_numbers/2" do
    test "removes issues present in the rejected set" do
      assert IssueLifecycle.reject_issue_numbers(
               [issue(10), issue(11), issue(12)],
               MapSet.new([10, 12])
             ) ==
               [issue(11)]
    end
  end

  describe "auto_closed_issue_numbers/4" do
    test "returns only successfully closed issues and logs failures" do
      log =
        capture_log(fn ->
          assert IssueLifecycle.auto_closed_issue_numbers(
                   "misty-step/bitterblossom",
                   [issue(10), issue(11)],
                   MapSet.new([10, 11]),
                   fn
                     _, 10 -> :ok
                     _, 11 -> {:error, :boom}
                   end
                 ) == MapSet.new([10])
        end)

      assert log =~ "failed to auto-close issue #11 resolved by a merged PR"
    end
  end

  describe "resolved_issue_numbers_from_pr/2" do
    test "combines branch, body, merge commit, and commit message signals" do
      pr = %{
        "head" => %{"ref" => "factory/10-1773840330"},
        "body" => "Fixes #11",
        "merge_commit_message" => "Closes #12",
        "commits" => [
          %{"message" => "Resolves #13"},
          %{"commit" => %{"message" => "Closed #14"}}
        ]
      }

      assert IssueLifecycle.resolved_issue_numbers_from_pr(
               pr,
               MapSet.new([10, 11, 12, 13, 14, 99])
             ) ==
               MapSet.new([10, 11, 12, 13, 14])
    end

    test "ignores malformed factory branches" do
      remaining = MapSet.new([10, 11, 12])

      assert IssueLifecycle.resolved_issue_numbers_from_pr(
               %{"head" => %{"ref" => "factory/"}},
               remaining
             ) == MapSet.new()

      assert IssueLifecycle.resolved_issue_numbers_from_pr(
               %{"head" => %{"ref" => "factory/abc-branch"}},
               remaining
             ) == MapSet.new()

      assert IssueLifecycle.resolved_issue_numbers_from_pr(
               %{"head" => %{"ref" => "factory/123-"}},
               remaining
             ) == MapSet.new()
    end
  end

  defp issue(number) do
    %Issue{
      number: number,
      title: "issue #{number}",
      body: "## Problem\nx\n\n## Acceptance Criteria\n- [ ] y",
      url: "https://example.test/issues/#{number}",
      state: "OPEN"
    }
  end
end
