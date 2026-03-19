defmodule Conductor.PromptTest do
  use ExUnit.Case, async: true

  alias Conductor.{Issue, Prompt}

  @issue %Issue{
    number: 99,
    title: "Add caching layer",
    body: "## Product Spec\nCache all the things.\n\n### Intent Contract\n- MUST cache",
    url: "https://github.com/org/repo/issues/99"
  }

  describe "build_builder_prompt/4 initial (no feedback)" do
    setup do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-123",
          "factory/99-123"
        )

      %{prompt: prompt}
    end

    test "includes run metadata", %{prompt: prompt} do
      assert prompt =~ "# Builder Task"
      assert prompt =~ "Run ID: run-99-123"
      assert prompt =~ "Issue: #99"
      assert prompt =~ "Add caching layer"
      assert prompt =~ "Branch: factory/99-123"
    end

    test "includes issue URL", %{prompt: prompt} do
      assert prompt =~ "https://github.com/org/repo/issues/99"
    end

    test "wraps issue body in untrusted-data fence", %{prompt: prompt} do
      assert prompt =~ "~~~untrusted-data"
      assert prompt =~ "Cache all the things."
    end

    test "omits artifact instructions and includes completion handoff", %{prompt: prompt} do
      refute prompt =~ "builder-result.json"
      refute prompt =~ "write JSON"
      refute prompt =~ "Result Artifact"
      assert prompt =~ "TASK_COMPLETE"
    end

    test "includes initial implementation phases", %{prompt: prompt} do
      assert prompt =~ "Phase 1: Implementation"
      assert prompt =~ "Phase 2: Review & Revision"
      assert prompt =~ "Phase 3: Handoff"
    end

    test "does not include revision section", %{prompt: prompt} do
      refute prompt =~ "Revision Required"
    end

    test "does not include existing PR line", %{prompt: prompt} do
      refute prompt =~ "Existing PR:"
    end

    test "does not carry inline weaver identity prose", %{prompt: prompt} do
      refute prompt =~ "You are Weaver"
    end

    test "treats the branch as pre-created and forbids switching lanes", %{prompt: prompt} do
      assert prompt =~ "Stay on the pre-created branch `factory/99-123`"
      refute prompt =~ "Create branch `factory/99-123`"
    end
  end

  describe "build_builder_prompt/4 with workspace_root" do
    test "includes repo root metadata" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-321",
          "factory/99-321",
          workspace_root: "/tmp/bb/run-99"
        )

      assert prompt =~ "Repository Root: /tmp/bb/run-99"
      refute prompt =~ "Change to repo root"
    end
  end

  describe "build_builder_prompt/4 with existing PR" do
    test "includes existing PR number" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-200",
          "factory/99-200",
          pr_number: 42
        )

      assert prompt =~ "Existing PR: #42"
    end
  end

  describe "build_builder_prompt/4 revision (with feedback)" do
    setup do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-456",
          "factory/99-456",
          feedback: "Please add error handling for nil inputs."
        )

      %{prompt: prompt}
    end

    test "includes revision section", %{prompt: prompt} do
      assert prompt =~ "Revision Required"
    end

    test "includes feedback in review-feedback fence", %{prompt: prompt} do
      assert prompt =~ "```review-feedback"
      assert prompt =~ "Please add error handling for nil inputs."
    end

    test "does not include initial implementation phases", %{prompt: prompt} do
      refute prompt =~ "Phase 1: Implementation"
      refute prompt =~ "Phase 2: Review & Revision"
    end
  end

  describe "build_builder_prompt/4 with both PR and feedback" do
    test "includes both existing PR and revision" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-789",
          "factory/99-789",
          pr_number: 55,
          feedback: "Fix the tests."
        )

      assert prompt =~ "Existing PR: #55"
      assert prompt =~ "Revision Required"
      assert prompt =~ "Fix the tests."
    end
  end

  describe "build_builder_prompt/4 handoff contract" do
    test "uses TASK_COMPLETE and BLOCKED.md instead of artifact JSON" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-100",
          "factory/99-100"
        )

      assert prompt =~ "write TASK_COMPLETE"
      assert prompt =~ "write BLOCKED.md"
      refute prompt =~ ~s("pr_number")
      refute prompt =~ ~s("pr_url")
    end
  end

  describe "governance restrictions in builder prompt" do
    test "prohibits gh pr merge" do
      prompt = Prompt.build_builder_prompt(@issue, "run-99-gov", "factory/99-gov")
      assert prompt =~ "gh pr merge"
      assert prompt =~ "MUST NOT"
    end

    test "prohibits gh pr close" do
      prompt = Prompt.build_builder_prompt(@issue, "run-99-gov", "factory/99-gov")
      assert prompt =~ "gh pr close"
      assert prompt =~ "MUST NOT"
    end
  end

  describe "governance restrictions in builder revision prompt" do
    test "prohibits gh pr merge even with feedback" do
      prompt =
        Prompt.build_builder_prompt(@issue, "run-99-rev", "factory/99-rev",
          feedback: "Fix the tests."
        )

      assert prompt =~ "gh pr merge"
      assert prompt =~ "MUST NOT"
    end
  end

  describe "governance restrictions in fixer prompt" do
    test "prohibits gh pr merge" do
      pr = %{"number" => 10, "title" => "Fix CI", "headRefName" => "factory/10-fix"}
      prompt = Prompt.build_fixer_prompt(pr, "test failed", "issue body")
      assert prompt =~ "# Fixer Task"
      assert prompt =~ "gh pr merge"
      assert prompt =~ "MUST NOT"
      refute prompt =~ "You are Thorn"
    end

    test "prohibits weakening safeguards just to clear CI" do
      pr = %{"number" => 10, "title" => "Fix CI", "headRefName" => "factory/10-fix"}
      prompt = Prompt.build_fixer_prompt(pr, "test failed", "issue body")

      assert prompt =~ "Do NOT weaken tests, security gates, review protections"
      assert prompt =~ "Restore the intended behavior and let CI prove the fix."
      refute prompt =~ "Focus exclusively on making CI green."
    end

    test "keeps Thorn workflow guidance in persona files instead of inline prompt text" do
      pr = %{"number" => 10, "title" => "Fix CI", "headRefName" => "factory/10-fix"}
      prompt = Prompt.build_fixer_prompt(pr, "test failed", "issue body")

      assert prompt =~ "Fix the CI failure on this PR."
      assert prompt =~ "Investigate the root cause in the codebase"
      refute prompt =~ "/gather-pr-context"
      refute prompt =~ "/diagnose-ci"
      refute prompt =~ "/plan-fix"
      refute prompt =~ "/verify-invariants"
      refute prompt =~ ".claude/skills"
    end
  end

  describe "governance restrictions in polisher prompt" do
    test "prohibits gh pr merge" do
      pr = %{"number" => 10, "title" => "Fix CI", "headRefName" => "factory/10-fix"}
      prompt = Prompt.build_polisher_prompt(pr, [], "issue body")
      assert prompt =~ "# Polisher Task"
      assert prompt =~ "gh pr merge"
      assert prompt =~ "MUST NOT"
      refute prompt =~ "You are Fern"
    end

    test "prohibits gh pr close" do
      pr = %{"number" => 10, "title" => "Fix CI", "headRefName" => "factory/10-fix"}
      prompt = Prompt.build_polisher_prompt(pr, [], "issue body")
      assert prompt =~ "gh pr close"
      assert prompt =~ "MUST NOT"
    end
  end

  describe "build_builder_prompt/4 with repo_context (CLAUDE.md)" do
    @claude_md_content """
    # CLAUDE.md

    ## What This Is
    Bitterblossom — Elixir/OTP conductor for a sprite-based software factory.

    ## Coding Standards
    - Elixir 1.16+, mix format, deep modules (Ousterhout)
    - Semantic commits: feat:, fix:, test:, docs:, refactor:
    """

    test "includes Repository Context section before the task header" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-300",
          "factory/99-300",
          repo_context: @claude_md_content
        )

      assert prompt =~ "## Repository Context"
      assert prompt =~ "Elixir/OTP conductor"
      context_pos = :binary.match(prompt, "## Repository Context") |> elem(0)
      task_pos = :binary.match(prompt, "# Builder Task") |> elem(0)
      assert context_pos < task_pos, "Repository Context must appear before Builder Task"
    end

    test "includes CLAUDE.md content in the prompt" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-301",
          "factory/99-301",
          repo_context: @claude_md_content
        )

      assert prompt =~ "Coding Standards"
      assert prompt =~ "mix format"
    end
  end

  describe "build_builder_prompt/4 with repo_context (project.md)" do
    @project_md_content """
    # Project: Bitterblossom

    ## Vision
    Single-repo software factory conductor. Routes GitHub work to sprites, drives implementation, merges when governance says done.

    ## Quality Bar
    - Every issue the conductor leases is runnable by sprites without clarification loops.
    - Run state tells the truth.
    """

    test "includes project vision and quality bar from project.md" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-400",
          "factory/99-400",
          repo_context: @project_md_content
        )

      assert prompt =~ "## Repository Context"
      assert prompt =~ "software factory conductor"
      assert prompt =~ "Quality Bar"
    end
  end

  describe "build_builder_prompt/4 without repo_context" do
    test "omits Repository Context section when not provided" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-500",
          "factory/99-500"
        )

      refute prompt =~ "## Repository Context"
    end
  end
end
