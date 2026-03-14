defmodule Conductor.PromptTest do
  use ExUnit.Case, async: true

  alias Conductor.{Issue, Prompt}

  @issue %Issue{
    number: 99,
    title: "Add caching layer",
    body: "## Product Spec\nCache all the things.\n\n### Intent Contract\n- MUST cache",
    url: "https://github.com/org/repo/issues/99"
  }

  describe "build_builder_prompt/5 initial (no feedback)" do
    setup do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-123",
          "factory/99-123",
          "/home/sprite/workspace/repo/.bb/conductor/run-99-123/builder-result.json"
        )

      %{prompt: prompt}
    end

    test "includes run metadata", %{prompt: prompt} do
      assert prompt =~ "Run ID: run-99-123"
      assert prompt =~ "Issue: #99"
      assert prompt =~ "Add caching layer"
      assert prompt =~ "Branch: factory/99-123"
    end

    test "includes issue URL", %{prompt: prompt} do
      assert prompt =~ "https://github.com/org/repo/issues/99"
    end

    test "wraps issue body in untrusted-data fence", %{prompt: prompt} do
      assert prompt =~ "```untrusted-data"
      assert prompt =~ "Cache all the things."
    end

    test "includes artifact path in instructions and json block", %{prompt: prompt} do
      artifact = "/home/sprite/workspace/repo/.bb/conductor/run-99-123/builder-result.json"
      assert prompt =~ "write JSON to `#{artifact}`"
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

    test "includes branch name in implementation instructions", %{prompt: prompt} do
      assert prompt =~ "Create branch `factory/99-123`"
    end
  end

  describe "build_builder_prompt/5 with existing PR" do
    test "includes existing PR number" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-200",
          "factory/99-200",
          "/tmp/result.json",
          pr_number: 42
        )

      assert prompt =~ "Existing PR: #42"
    end
  end

  describe "build_builder_prompt/5 revision (with feedback)" do
    setup do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-456",
          "factory/99-456",
          "/tmp/result.json",
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

  describe "build_builder_prompt/5 with both PR and feedback" do
    test "includes both existing PR and revision" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-789",
          "factory/99-789",
          "/tmp/result.json",
          pr_number: 55,
          feedback: "Fix the tests."
        )

      assert prompt =~ "Existing PR: #55"
      assert prompt =~ "Revision Required"
      assert prompt =~ "Fix the tests."
    end
  end

  describe "build_builder_prompt/5 result artifact" do
    test "specifies expected JSON schema fields" do
      prompt =
        Prompt.build_builder_prompt(
          @issue,
          "run-99-100",
          "factory/99-100",
          "/tmp/result.json"
        )

      assert prompt =~ ~s("status": "ready" or "blocked")
      assert prompt =~ ~s("branch": "factory/99-100")
      assert prompt =~ ~s("pr_number")
      assert prompt =~ ~s("pr_url")
      assert prompt =~ ~s("summary")
      assert prompt =~ ~s("blocking_reason")
    end
  end
end
