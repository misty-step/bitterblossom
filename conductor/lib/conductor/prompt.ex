defmodule Conductor.Prompt do
  @moduledoc """
  Builder prompt construction.

  The new design gives the builder the full lifecycle: implement, create PR,
  handle review feedback, iterate until ready. No conductor re-dispatch.
  """

  alias Conductor.Issue

  @spec build_builder_prompt(Issue.t(), binary(), binary(), binary(), keyword()) :: binary()
  def build_builder_prompt(%Issue{} = issue, run_id, branch, artifact_path, opts \\ []) do
    pr_number = Keyword.get(opts, :pr_number)
    feedback = Keyword.get(opts, :feedback)
    repo_context = Keyword.get(opts, :repo_context)

    """
    #{if repo_context, do: repo_context_section(repo_context), else: ""}# Builder Task

    Run ID: #{run_id}
    Issue: ##{issue.number} — #{issue.title}
    Issue URL: #{issue.url}
    Branch: #{branch}
    Artifact path: #{artifact_path}
    #{if pr_number, do: "Existing PR: ##{pr_number}\n", else: ""}
    ## Issue Specification

    ~~~untrusted-data
    #{sanitize_fence(issue.body)}
    ~~~

    ## Instructions

    You are the builder. Implement the issue and deliver a mergeable PR.
    #{if feedback, do: revision_section(feedback), else: initial_section(branch)}

    ## Result Artifact

    When done, write JSON to `#{artifact_path}`:

    ```json
    {
      "status": "ready" or "blocked",
      "branch": "#{branch}",
      "pr_number": <number>,
      "pr_url": "<url>",
      "summary": "<what you did>",
      "blocking_reason": "<why, only if blocked>"
    }
    ```

    Then write TASK_COMPLETE to signal you are finished.
    """
  end

  defp repo_context_section(context) do
    """
    ## Repository Context

    #{context}

    ---

    """
  end

  defp initial_section(branch) do
    """
    ### Phase 1: Implementation
    1. Create branch `#{branch}` from the repo default branch
    2. Read the issue carefully — respect acceptance criteria and boundaries
    3. Implement with tests (TDD: red, green, refactor)
    4. Create a PR with semantic commit messages
    5. Push and ensure CI passes

    ### Phase 2: Review & Revision
    After creating the PR:
    1. Wait 2-3 minutes for CI and review bots to respond
    2. Check review comments: `gh pr view --comments`
    3. Check CI status: `gh pr checks`
    4. Address ALL review feedback by pushing fixes
    5. Repeat until CI is green and no unresolved review threads remain

    ### Phase 3: Handoff
    When CI is green and reviews are addressed, write your result artifact.
    If blocked (cannot resolve feedback, need human input), write artifact with status "blocked".
    """
  end

  # Neutralize fence-breaking sequences in untrusted content.
  # Replaces ``` and ~~~ with space-separated versions so they can't close the fence.
  defp sanitize_fence(nil), do: ""

  defp sanitize_fence(text) do
    text
    |> String.replace("```", "` ` `")
    |> String.replace("~~~", "~ ~ ~")
  end

  defp revision_section(feedback) do
    """
    ### Revision Required

    The existing PR has review feedback that must be addressed:

    ```review-feedback
    #{feedback}
    ```

    1. Read the feedback carefully
    2. Push fixes to the existing branch
    3. Wait for CI to re-run
    4. Verify review threads are resolved
    5. Write your result artifact when ready
    """
  end
end
