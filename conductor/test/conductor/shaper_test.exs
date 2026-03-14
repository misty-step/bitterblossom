defmodule Conductor.ShaperTest do
  use ExUnit.Case, async: true

  alias Conductor.{Issue, Shaper}

  describe "build_prompt/2" do
    test "includes issue title and number" do
      issue = %Issue{number: 42, title: "Fix widget", body: "Something is broken.", url: "u"}

      context = %{
        claude_md: nil,
        agents_md: nil,
        git_log: "(log)",
        file_tree: "(tree)",
        open_issues: "(none)"
      }

      prompt = Shaper.build_prompt(issue, context)

      assert String.contains?(prompt, "Fix widget")
      assert String.contains?(prompt, "#42")
    end

    test "includes existing issue body" do
      issue = %Issue{number: 1, title: "t", body: "## Problem\nAlready has problem.", url: "u"}
      context = %{claude_md: nil, agents_md: nil, git_log: "", file_tree: "", open_issues: ""}

      prompt = Shaper.build_prompt(issue, context)

      assert String.contains?(prompt, "Already has problem.")
    end

    test "includes project context" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u"}

      context = %{
        claude_md: "This is CLAUDE.md content",
        agents_md: "This is AGENTS.md content",
        git_log: "abc123 feat: something",
        file_tree: "conductor/lib/conductor/cli.ex",
        open_issues: "#100 Some other issue"
      }

      prompt = Shaper.build_prompt(issue, context)

      assert String.contains?(prompt, "This is CLAUDE.md content")
      assert String.contains?(prompt, "This is AGENTS.md content")
      assert String.contains?(prompt, "abc123 feat: something")
      assert String.contains?(prompt, "conductor/lib/conductor/cli.ex")
      assert String.contains?(prompt, "#100 Some other issue")
    end

    test "instructs LLM to preserve existing sections" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u"}
      context = %{claude_md: nil, agents_md: nil, git_log: "", file_tree: "", open_issues: ""}

      prompt = Shaper.build_prompt(issue, context)

      assert String.contains?(prompt, "Preserve")
      assert String.contains?(prompt, "## Problem")
      assert String.contains?(prompt, "## Acceptance Criteria")
    end

    test "requires output to have ## Problem and ## Acceptance Criteria headings" do
      issue = %Issue{number: 1, title: "t", body: "", url: "u"}
      context = %{claude_md: nil, agents_md: nil, git_log: "", file_tree: "", open_issues: ""}

      prompt = Shaper.build_prompt(issue, context)

      # Should instruct the LLM about required headings
      assert String.contains?(prompt, "## Problem")
      assert String.contains?(prompt, "## Acceptance Criteria")
    end
  end

  describe "shape/3 error handling" do
    test "returns error when GitHub CLI is unavailable" do
      # gh CLI will fail against a fake repo — this tests the error propagation path
      result = Shaper.shape("definitely-fake-org/definitely-fake-repo", 999_999_999)
      assert {:error, _reason} = result
    end
  end
end
