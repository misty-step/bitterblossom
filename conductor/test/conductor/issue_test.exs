defmodule Conductor.IssueTest do
  use ExUnit.Case, async: true

  alias Conductor.Issue

  describe "from_github/1" do
    test "maps required fields" do
      issue =
        Issue.from_github(%{
          "number" => 42,
          "title" => "Add widget",
          "body" => "some body",
          "url" => "https://github.com/org/repo/issues/42"
        })

      assert %Issue{number: 42, title: "Add widget", body: "some body"} = issue
      assert issue.url == "https://github.com/org/repo/issues/42"
    end

    test "defaults body to empty string when nil" do
      issue =
        Issue.from_github(%{
          "number" => 1,
          "title" => "No body",
          "body" => nil,
          "url" => "https://github.com/org/repo/issues/1"
        })

      assert issue.body == ""
    end

    test "defaults body to empty string when missing" do
      issue = Issue.from_github(%{"number" => 1, "title" => "Bare"})

      assert issue.body == ""
    end

    test "synthesizes url when missing" do
      issue = Issue.from_github(%{"number" => 7, "title" => "No URL"})

      assert issue.url == "https://github.com/unknown/issues/7"
    end

    test "extracts label names from map labels" do
      issue =
        Issue.from_github(%{
          "number" => 1,
          "title" => "Labeled",
          "labels" => [%{"name" => "bug"}, %{"name" => "autopilot"}]
        })

      assert issue.labels == ["bug", "autopilot"]
    end

    test "handles plain string labels" do
      issue =
        Issue.from_github(%{
          "number" => 1,
          "title" => "Strings",
          "labels" => ["enhancement", "p1"]
        })

      assert issue.labels == ["enhancement", "p1"]
    end

    test "defaults labels to empty list when missing" do
      issue = Issue.from_github(%{"number" => 1, "title" => "No labels"})

      assert issue.labels == []
    end

    test "defaults labels to empty list when nil" do
      issue =
        Issue.from_github(%{
          "number" => 1,
          "title" => "Nil labels",
          "labels" => nil
        })

      assert issue.labels == []
    end

    test "defaults state to OPEN when missing" do
      issue = Issue.from_github(%{"number" => 1, "title" => "No state"})

      assert issue.state == "OPEN"
    end

    test "normalizes state from github payload" do
      issue =
        Issue.from_github(%{
          "number" => 1,
          "title" => "Closed",
          "state" => "closed"
        })

      assert issue.state == "CLOSED"
    end
  end

  describe "ready?/1" do
    test "accepts groom format (Problem + Acceptance Criteria)" do
      body = """
      ## Problem
      Widget is broken.

      ## Acceptance Criteria
      - [ ] [test] Given X, when Y, then Z
      """

      issue = %Issue{number: 1, title: "t", body: body, url: "u"}
      assert :ok = Issue.ready?(issue)
    end

    test "accepts conductor format (Product Spec + Intent Contract)" do
      body = """
      ## Product Spec

      ### Intent Contract
      - MUST do X
      """

      issue = %Issue{number: 1, title: "t", body: body, url: "u"}
      assert :ok = Issue.ready?(issue)
    end

    test "rejects body with only Problem (no criteria)" do
      body = "## Problem\nSomething is wrong."
      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      assert {:error, failures} = Issue.ready?(issue)
      assert length(failures) == 1
    end

    test "rejects body with only Acceptance Criteria (no problem)" do
      body = "## Acceptance Criteria\n- [ ] test"
      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      assert {:error, failures} = Issue.ready?(issue)
      assert length(failures) == 1
    end

    test "rejects empty body with both failures" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u"}

      assert {:error, failures} = Issue.ready?(issue)
      assert length(failures) == 2
    end

    test "accepts mixed format (Problem + Intent Contract)" do
      body = "## Problem\nBroken.\n\n### Intent Contract\nFix it."
      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      # Has Problem but not Acceptance Criteria, has Intent Contract but not Product Spec
      # Neither complete format matches, so this should fail
      assert {:error, _} = Issue.ready?(issue)
    end
  end

  describe "lifecycle_valid?/1" do
    test "accepts open issues" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u", state: "OPEN"}

      assert :ok = Issue.lifecycle_valid?(issue)
    end

    test "rejects closed issues" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u", state: "CLOSED"}

      assert {:error, ["issue is closed"]} = Issue.lifecycle_valid?(issue)
    end
  end

  describe "revision_id/1" do
    test "returns the same hash for identical bodies" do
      issue = %Issue{number: 1, title: "t", body: "same body", url: "u"}

      assert Issue.revision_id(issue) == Issue.revision_id(issue)
    end

    test "returns different hashes for different bodies" do
      issue_a = %Issue{number: 1, title: "t", body: "alpha", url: "u"}
      issue_b = %Issue{number: 1, title: "t", body: "beta", url: "u"}

      refute Issue.revision_id(issue_a) == Issue.revision_id(issue_b)
    end

    test "treats nil and empty bodies the same" do
      empty_issue = %Issue{number: 1, title: "t", body: "", url: "u"}
      nil_issue = %Issue{number: 1, title: "t", body: nil, url: "u"}

      assert Issue.revision_id(empty_issue) == Issue.revision_id(nil_issue)
    end
  end
end
