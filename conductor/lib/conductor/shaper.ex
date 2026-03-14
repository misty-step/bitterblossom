defmodule Conductor.Shaper do
  @moduledoc """
  LLM-powered issue enrichment.

  Reads a GitHub issue, gathers project context (codebase structure, recent
  commits, related issues), and uses an LLM to rewrite the body with
  structured sections that satisfy `Issue.ready?/1`.

  Idempotent: if the issue already passes `Issue.ready?/1`, returns
  `{:ok, :already_shaped}` without modifying anything.

  Preserves existing structured content — only fills missing sections.
  """

  require Logger

  alias Conductor.{GitHub, Issue, Shell}

  @anthropic_url "https://api.anthropic.com/v1/messages"
  @model "claude-sonnet-4-20250514"
  @max_tokens 4096

  # Repo root is three levels up from __DIR__ (conductor/lib/conductor/)
  @repo_root Path.expand("../../..", __DIR__)

  @doc """
  Shape a GitHub issue by enriching its body with structured sections.

  Returns `{:ok, :already_shaped}` if the issue is already ready,
  `{:ok, :shaped}` on success, or `{:error, reason}` on failure.
  """
  @spec shape(binary(), pos_integer(), keyword()) ::
          {:ok, :already_shaped | :shaped} | {:error, term()}
  def shape(repo, issue_number, opts \\ []) do
    with {:ok, issue} <- GitHub.get_issue(repo, issue_number) do
      case Issue.ready?(issue) do
        :ok ->
          Logger.info("[shaper] ##{issue_number} already shaped, skipping")
          {:ok, :already_shaped}

        {:error, missing} ->
          Logger.info(
            "[shaper] shaping ##{issue_number}: #{issue.title} (missing: #{inspect(missing)})"
          )

          do_shape(repo, issue, opts)
      end
    end
  end

  @doc """
  Build the enrichment prompt for the LLM.

  Exposed for testing.
  """
  @spec build_prompt(Issue.t(), map()) :: binary()
  def build_prompt(%Issue{} = issue, context) do
    """
    You are shaping a GitHub issue for autonomous implementation by a coding agent.

    Your job: rewrite the issue body with clear structured sections.
    Preserve any existing structured content — only fill in what is missing.

    Required output sections (use these exact headings):

    ## Problem
    What is broken or missing, and why it matters. Be specific.

    ## Acceptance Criteria
    Checkable items. Tag each: [behavioral], [code], or [test].
    Example: - [ ] [behavioral] `mix conductor shape --repo R --issue N` rewrites the body

    ## Affected Files
    Specific files the implementation will touch. Use real paths from the file tree below.

    ## Verification
    Exact shell commands to verify the change works end-to-end.

    ---

    ## Issue Being Shaped

    Title: #{issue.title}
    Number: ##{issue.number}

    Current body:
    #{issue.body || "(empty)"}

    ---

    ## Project Context

    ### Architecture (CLAUDE.md)
    #{context.claude_md || "(not found)"}

    ### Repo Overview (AGENTS.md)
    #{context.agents_md || "(not found)"}

    ### Recent Commits
    #{context.git_log}

    ### File Tree
    #{context.file_tree}

    ### Open Issues (for cross-reference)
    #{context.open_issues}

    ---

    ## Output Instructions

    1. If a section already exists in the current body, copy it verbatim.
    2. Fill in any missing sections using the project context above.
    3. In Affected Files, cite real paths from the file tree.
    4. Return ONLY the shaped issue body as markdown — no preamble, no explanation.
    5. The body MUST contain `## Problem` and `## Acceptance Criteria` headings.
    """
  end

  # --- Private ---

  defp do_shape(repo, issue, _opts) do
    context = gather_context(repo)
    prompt = build_prompt(issue, context)

    with {:ok, new_body} <- call_llm(prompt),
         :ok <- GitHub.update_issue_body(repo, issue.number, new_body) do
      Logger.info("[shaper] ##{issue.number} shaped successfully")
      {:ok, :shaped}
    end
  end

  defp gather_context(repo) do
    %{
      claude_md: read_file(Path.join(@repo_root, "CLAUDE.md"), 3_000),
      agents_md: read_file(Path.join(@repo_root, "AGENTS.md"), 2_000),
      git_log: recent_git_log(),
      file_tree: file_tree(),
      open_issues: open_issue_titles(repo)
    }
  end

  defp call_llm(prompt) do
    api_key = api_key()

    if is_nil(api_key) do
      {:error, "no ANTHROPIC_API_KEY or CONDUCTOR_SHAPER_API_KEY set"}
    else
      body =
        Jason.encode!(%{
          model: @model,
          max_tokens: @max_tokens,
          messages: [%{role: "user", content: prompt}]
        })

      tmp = Path.join(System.tmp_dir!(), "shaper-#{System.unique_integer([:positive])}.json")
      File.write!(tmp, body)

      try do
        result =
          Shell.cmd("curl", [
            "-sS",
            "-X",
            "POST",
            @anthropic_url,
            "-H",
            "content-type: application/json",
            "-H",
            "x-api-key: #{api_key}",
            "-H",
            "anthropic-version: 2023-06-01",
            "-d",
            "@#{tmp}"
          ])

        case result do
          {:ok, json} ->
            case Jason.decode(json) do
              {:ok, %{"content" => [%{"text" => text} | _]}} ->
                {:ok, strip_code_fence(text)}

              {:ok, %{"error" => err}} ->
                {:error, "API error: #{inspect(err)}"}

              {:error, _} ->
                {:error, "invalid JSON response from LLM"}
            end

          {:error, msg, code} ->
            {:error, "curl failed (#{code}): #{String.slice(msg, 0, 200)}"}
        end
      after
        File.rm(tmp)
      end
    end
  end

  # Strip a single outer ```...``` wrapper the LLM may add around the entire body.
  # Only removes the outermost fence — does NOT touch embedded code blocks.
  defp strip_code_fence(text) do
    lines =
      text
      |> String.trim()
      |> String.split("\n")

    if length(lines) >= 2 and
         String.match?(hd(lines), ~r/^```[a-zA-Z]*$/) and
         List.last(lines) == "```" do
      lines
      |> Enum.drop(1)
      |> Enum.drop(-1)
      |> Enum.join("\n")
      |> String.trim()
    else
      String.trim(text)
    end
  end

  defp api_key do
    System.get_env("CONDUCTOR_SHAPER_API_KEY") ||
      System.get_env("ANTHROPIC_API_KEY")
  end

  defp read_file(path, limit) do
    case File.read(path) do
      {:ok, content} -> String.slice(content, 0, limit)
      _ -> nil
    end
  end

  defp recent_git_log do
    case Shell.cmd("git", ["log", "--oneline", "-20"], cd: @repo_root) do
      {:ok, output} -> output
      _ -> "(git log unavailable)"
    end
  end

  defp file_tree do
    case Shell.cmd(
           "find",
           [
             "conductor/lib",
             "cmd/bb",
             "-name",
             "*.ex",
             "-o",
             "-name",
             "*.go"
           ],
           cd: @repo_root
         ) do
      {:ok, output} -> String.slice(output, 0, 2_000)
      _ -> "(file tree unavailable)"
    end
  end

  defp open_issue_titles(repo) do
    case Shell.cmd("gh", [
           "issue",
           "list",
           "--repo",
           repo,
           "--state",
           "open",
           "--json",
           "number,title",
           "--limit",
           "20",
           "--jq",
           ".[] | \"#\\(.number) \\(.title)\""
         ]) do
      {:ok, output} -> String.slice(output, 0, 2_000)
      _ -> "(could not fetch issue titles)"
    end
  end
end
