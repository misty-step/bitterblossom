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

  @doc "Build prompt for the fixer sprite: CI failure context + fix instructions."
  @spec build_fixer_prompt(map(), binary(), binary()) :: binary()
  def build_fixer_prompt(pr, ci_failure_logs, issue_body) do
    """
    # Fixer Task

    PR: ##{pr["number"]} — #{pr["title"]}
    Branch: #{pr["headRefName"]}

    ## Original Issue

    ~~~untrusted-data
    #{sanitize_fence(issue_body)}
    ~~~

    ## CI Failure Output

    ```
    #{ci_failure_logs}
    ```

    ## Instructions

    You are the fixer. Your only job is to fix the CI failure on this PR.

    1. Check out branch `#{pr["headRefName"]}`
    2. Read the CI failure output above carefully
    3. Investigate the root cause in the codebase
    4. Fix the issue — do not change PR scope or add features
    5. Run the failing tests/checks locally to verify the fix
    6. Commit with message `fix: resolve CI failure` and push
    7. CI will re-trigger automatically

    Do NOT modify the PR description, title, or labels.
    Do NOT expand the scope of the PR.
    Focus exclusively on making CI green.

    When done, write TASK_COMPLETE.
    """
  end

  @doc "Build prompt for the polisher sprite: review context + polish instructions."
  @spec build_polisher_prompt(map(), [map()], binary()) :: binary()
  def build_polisher_prompt(pr, review_comments, issue_body) do
    comments_text =
      review_comments
      |> Enum.map(fn c ->
        author = c["author"] || c["user"] || %{}

        name =
          case author do
            s when is_binary(s) -> s
            m when is_map(m) -> m["login"] || m["name"] || "unknown"
            _ -> "unknown"
          end

        body = c["body"] || ""
        "- **#{name}**: #{body}"
      end)
      |> Enum.join("\n")

    """
    # Polisher Task

    PR: ##{pr["number"]} — #{pr["title"]}
    Branch: #{pr["headRefName"]}

    ## Original Issue

    ~~~untrusted-data
    #{sanitize_fence(issue_body)}
    ~~~

    ## Review Comments

    #{if comments_text == "", do: "_No review comments._", else: comments_text}

    ## Instructions

    You are the polisher. Your job is to address all review feedback on this PR.

    1. Check out branch `#{pr["headRefName"]}`
    2. Read each review comment above
    3. For in-scope feedback: make the fix on-branch, commit, push
    4. For out-of-scope feedback: note it in a comment on the PR thread
    5. Respond to each review thread with what you did
    6. Run tests to ensure nothing is broken
    7. When all feedback is addressed and CI is green, run:
       `gh pr edit #{pr["number"]} --add-label lgtm`

    Do NOT expand the scope of the PR beyond addressing review feedback.
    Do NOT remove the PR from review or modify its base branch.

    When done, write TASK_COMPLETE.
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
