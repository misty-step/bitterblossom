defmodule Conductor.IssueTest do
  use ExUnit.Case, async: true

  alias Conductor.Issue

  describe "from_github/1" do
    test "maps required fields" do
      issue = Issue.from_github(%{
        "number" => 42,
        "title" => "Add widget",
        "body" => "some body",
        "url" => "https://github.com/org/repo/issues/42"
      })

      assert %Issue{number: 42, title: "Add widget", body: "some body"} = issue
      assert issue.url == "https://github.com/org/repo/issues/42"
    end

    test "defaults body to empty string when nil" do
      issue = Issue.from_github(%{
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
      issue = Issue.from_github(%{
        "number" => 1,
        "title" => "Labeled",
        "labels" => [%{"name" => "bug"}, %{"name" => "autopilot"}]
      })

      assert issue.labels == ["bug", "autopilot"]
    end

    test "handles plain string labels" do
      issue = Issue.from_github(%{
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
      issue = Issue.from_github(%{
        "number" => 1,
        "title" => "Nil labels",
        "labels" => nil
      })

      assert issue.labels == []
    end
  end

  describe "ready?/1" do
    test "returns :ok when both sections present" do
      body = """
      ## Product Spec
      Build the thing.

      ### Intent Contract
      - MUST do X
      """

      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      assert :ok = Issue.ready?(issue)
    end

    test "rejects missing Product Spec" do
      body = """
      ### Intent Contract
      - MUST do X
      """

      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      assert {:error, failures} = Issue.ready?(issue)
      assert "missing `## Product Spec` section" in failures
      refute "missing `### Intent Contract` section" in failures
    end

    test "rejects missing Intent Contract" do
      body = """
      ## Product Spec
      Build the thing.
      """

      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      assert {:error, failures} = Issue.ready?(issue)
      assert "missing `### Intent Contract` section" in failures
      refute "missing `## Product Spec` section" in failures
    end

    test "rejects empty body with both failures" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u"}

      assert {:error, failures} = Issue.ready?(issue)
      assert length(failures) == 2
      assert "missing `## Product Spec` section" in failures
      assert "missing `### Intent Contract` section" in failures
    end

    test "heading match is substring-based, not line-anchored" do
      body = "prefix ## Product Spec suffix\nprefix ### Intent Contract suffix"
      issue = %Issue{number: 1, title: "t", body: body, url: "u"}

      assert :ok = Issue.ready?(issue)
    end
  end
end
