defmodule Conductor.GitHubTest do
  use ExUnit.Case, async: true

  alias Conductor.GitHub

  describe "checks_green?/1 (unit, no CLI)" do
    # Test the pure logic extracted into evaluate_checks/1

    test "all SUCCESS → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "mixed case conclusions → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "success", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "NEUTRAL and SKIPPED are non-blocking → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Optional", "conclusion" => "NEUTRAL", "status" => "COMPLETED"},
        %{"name" => "Skipped", "conclusion" => "SKIPPED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "null conclusions are filtered out → true when remaining pass" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => nil, "conclusion" => nil, "status" => nil},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "only null conclusions → false (no real signal)" do
      checks = [
        %{"name" => nil, "conclusion" => nil, "status" => nil},
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "FAILURE among real checks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Tests", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "FAILURE plus null → false (real failure takes precedence)" do
      checks = [
        %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"},
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "empty list → false" do
      assert GitHub.evaluate_checks([]) == false
    end

    test "in-progress check (nil conclusion, active status) blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "IN_PROGRESS"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "queued check blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "QUEUED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "waiting check (environment protection) blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "WAITING"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "requested check blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "REQUESTED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "annotation (nil conclusion AND nil status) is ignored → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end
  end

  describe "find_open_pr/2 — branch prefix filtering" do
    # Test the pure filter logic extracted from find_open_pr.
    # The function finds the first PR whose headRefName starts with "factory/<issue>-".

    defp filter_open_pr(prs, issue_number) do
      prefix = "factory/#{issue_number}-"
      Enum.find(prs, fn pr -> String.starts_with?(pr["headRefName"] || "", prefix) end)
    end

    test "matches PR with correct issue prefix" do
      prs = [
        %{
          "number" => 10,
          "headRefName" => "factory/42-1234567890",
          "url" => "http://example.com/10"
        },
        %{
          "number" => 11,
          "headRefName" => "factory/99-9999999999",
          "url" => "http://example.com/11"
        }
      ]

      result = filter_open_pr(prs, 42)
      assert result["number"] == 10
    end

    test "returns nil when no PR matches the issue number" do
      prs = [
        %{
          "number" => 10,
          "headRefName" => "factory/99-1234567890",
          "url" => "http://example.com/10"
        }
      ]

      assert filter_open_pr(prs, 42) == nil
    end

    test "does not match a different issue number that shares a prefix" do
      prs = [
        %{
          "number" => 10,
          "headRefName" => "factory/420-1234567890",
          "url" => "http://example.com/10"
        }
      ]

      # issue 42 should NOT match factory/420-... (dash is the delimiter)
      assert filter_open_pr(prs, 42) == nil
    end

    test "handles nil headRefName gracefully" do
      prs = [%{"number" => 10, "headRefName" => nil, "url" => "http://example.com/10"}]
      assert filter_open_pr(prs, 42) == nil
    end

    test "returns nil for empty list" do
      assert filter_open_pr([], 42) == nil
    end
  end

  describe "operator directive normalization" do
    test "label_present?/2 treats nil labels as empty" do
      refute GitHub.label_present?(%{"labels" => nil}, "hold")
    end

    test "label_present?/2 finds matching label names" do
      assert GitHub.label_present?(%{"labels" => [%{"name" => "hold"}]}, "hold")
    end

    test "normalize_issue_comments/1 treats nil comments as empty" do
      assert GitHub.normalize_issue_comments(nil) == []
    end

    test "normalize_issue_comments/1 keeps binary comment bodies" do
      assert GitHub.normalize_issue_comments([%{"body" => "bb: cancel"}]) == [
               %{"body" => "bb: cancel"}
             ]
    end

    test "normalize_issue_comments/1 extracts nested text bodies" do
      assert GitHub.normalize_issue_comments([%{"body" => %{"text" => "bb: cancel"}}]) == [
               %{"body" => "bb: cancel"}
             ]
    end

    test "normalize_issue_comments/1 extracts nested body bodies" do
      assert GitHub.normalize_issue_comments([%{"body" => %{"body" => "bb: cancel"}}]) == [
               %{"body" => "bb: cancel"}
             ]
    end
  end

  describe "checks_failed?/1 (unit, no CLI)" do
    test "FAILURE among checks → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Tests", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "all SUCCESS → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == false
    end

    test "pending checks with no conclusion → false (not failed yet)" do
      checks = [
        %{"name" => "CI", "conclusion" => nil, "status" => "IN_PROGRESS"},
        %{"name" => "Lint", "conclusion" => nil, "status" => "QUEUED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == false
    end

    test "empty list → false" do
      assert GitHub.evaluate_checks_failed([]) == false
    end

    test "ERROR conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "ERROR", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "CANCELLED conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "CANCELLED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "null conclusions only → false" do
      checks = [
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks_failed(checks) == false
    end

    test "STARTUP_FAILURE conclusion → true (Cerberus bootstrap failure)" do
      checks = [
        %{
          "name" => "review / Cerberus · preflight",
          "conclusion" => "STARTUP_FAILURE",
          "status" => "COMPLETED"
        }
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "TIMED_OUT conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "TIMED_OUT", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "STALE conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "STALE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "ACTION_REQUIRED conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "ACTION_REQUIRED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end
  end

  describe "evaluate_checks/1 — workflow bootstrap failure modes" do
    test "STARTUP_FAILURE blocks merge (non-green)" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{
          "name" => "review / Cerberus · preflight",
          "conclusion" => "STARTUP_FAILURE",
          "status" => "COMPLETED"
        }
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "TIMED_OUT blocks merge" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => "TIMED_OUT", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "STALE blocks merge" do
      checks = [
        %{"name" => "CI", "conclusion" => "STALE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "ACTION_REQUIRED blocks merge" do
      checks = [
        %{"name" => "CI", "conclusion" => "ACTION_REQUIRED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end
  end
end
