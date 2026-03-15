defmodule Conductor.Retro do
  @moduledoc """
  Post-run retrospective analysis. The conductor's fifth authority: learn.

  After every terminal run (merged, failed, blocked), analyzes the run's
  events against the project's architectural direction and produces
  structured backlog actions: issue creation, BACKLOG.md updates, or
  comments on existing issues.

  The architectural guard: "Does fixing this move toward or away from
  the target architecture? Is this a symptom or a root cause?"

  Runs asynchronously via a dedicated GenServer so it doesn't block
  RunServer shutdown. Crashes in analysis are caught and logged — they
  never take down the GenServer.
  """

  use GenServer
  require Logger

  alias Conductor.{Store, Shell, GitHub}

  @anthropic_url "https://api.anthropic.com/v1/messages"
  @model "claude-sonnet-4-20250514"
  @max_tokens 2048
  @supported_actions ~w(create_issue comment_issue update_backlog)

  # Repo root is three levels up from __DIR__ (conductor/lib/conductor/)
  @repo_root Path.expand("../../..", __DIR__)

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @doc "Queue a run for retrospective analysis. Fire-and-forget."
  @spec analyze(binary()) :: :ok
  def analyze(run_id) do
    if enabled?() do
      GenServer.cast(__MODULE__, {:analyze, run_id})
    else
      Logger.debug("[retro] disabled, skipping #{run_id}")
    end

    :ok
  end

  @doc false
  @spec record_complete_event(binary(), map()) :: :ok
  def record_complete_event(run_id, %{"findings" => findings, "summary" => summary})
      when is_list(findings) and is_binary(summary) do
    actionable_findings = Enum.filter(findings, &actionable?/1)

    skipped_count = length(findings) - length(actionable_findings)

    Store.record_event(run_id, "retro_complete", %{
      summary: summary,
      findings: findings,
      finding_count: length(findings),
      action_count: length(actionable_findings),
      actions_taken: Enum.map(actionable_findings, &action_metadata/1),
      skipped_count: skipped_count
    })
  end

  @doc false
  @spec finalize_analysis(binary(), map(), map(), (list(), map() -> any())) :: :ok
  def finalize_analysis(
        run_id,
        %{"findings" => findings} = analysis,
        run,
        execute_fn \\ &execute_actions/2
      )
      when is_list(findings) and is_map(run) do
    try do
      execute_fn.(findings, run)
    rescue
      error ->
        Logger.error(Exception.format(:error, error, __STACKTRACE__))
        reraise error, __STACKTRACE__
    after
      record_complete_event(run_id, analysis)
    end

    :ok
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(_opts) do
    {:ok, %{}}
  end

  @impl true
  def handle_cast({:analyze, run_id}, state) do
    try do
      do_analyze(run_id)
    rescue
      e ->
        Logger.warning("[retro] #{run_id} crashed: #{Exception.message(e)}")
    end

    {:noreply, state}
  end

  # --- Core Logic ---

  defp do_analyze(run_id) do
    Logger.info("[retro] analyzing #{run_id}")

    with {:ok, run} <- Store.get_run(run_id),
         events <- Store.list_events(run_id),
         context <- build_context(run, events),
         {:ok, response} <- call_llm(context),
         {:ok, analysis} <- parse_response(response) do
      finalize_analysis(run_id, analysis, run)
      Logger.info("[retro] #{run_id} complete: #{action_count(analysis["findings"])} action(s)")
    else
      {:error, reason} ->
        Logger.warning("[retro] #{run_id} failed: #{inspect(reason)}")
    end
  end

  defp build_context(run, events) do
    project_context = read_project_context()
    backlog = read_backlog()
    open_issues = list_open_issue_titles(run)

    """
    ## Run Data

    Run ID: #{run["run_id"]}
    Issue: ##{run["issue_number"]} — #{run["issue_title"]}
    Phase: #{run["phase"]} (terminal)
    Duration: #{duration(run)}
    PR: #{run["pr_number"] || "none"}
    Worker: #{run["builder_sprite"]}
    Turns: #{run["turn_count"]}

    ## Events

    #{format_events(events)}

    ## Project Direction

    #{project_context || "(no project.md found)"}

    ## Current Backlog (titles only)

    #{open_issues}

    ## BACKLOG.md (ideas/icebox)

    #{backlog || "(no .groom/BACKLOG.md found)"}
    """
  end

  defp system_prompt do
    """
    You are the conductor's retrospective agent. After every run, you analyze
    what happened and decide whether the backlog needs updating.

    ## Your Role

    You are the architectural immune system. You catch patterns that individual
    runs can't see. You distinguish symptoms from root causes. You prevent the
    codebase from becoming a Winchester Mystery House.

    ## The Architectural Guard

    For every finding, ask:
    1. "Is this a symptom of a deeper architectural gap, or a genuine new concern?"
    2. "Does fixing this move toward or away from the target architecture?"
    3. "If we were building from scratch, would this failure mode exist?"
    4. "Is there an existing issue that already covers this?"
    5. "Does an existing tool/platform already solve this?" (platform-native first)

    ## Output Format

    Respond with ONLY a JSON object (no markdown, no explanation):

    ```json
    {
      "findings": [
        {
          "type": "architectural_gap|operational_bug|process_improvement",
          "title": "concise title",
          "description": "what happened and why it matters",
          "root_cause": "the deeper issue, if any",
          "existing_issue": "#NNN or null",
          "action": "create_issue|comment_issue|update_backlog|none",
          "alignment": "toward|away|neutral",
          "content": "issue body, comment text, or BACKLOG.md entry"
        }
      ],
      "summary": "one-line run retrospective"
    }
    ```

    ## Rules

    - If the run merged cleanly with no friction, return `{"findings": [], "summary": "clean run"}`
    - NEVER create duplicate issues. Check the open issues list first.
    - Prefer commenting on existing issues over creating new ones.
    - Prefer "none" over low-value actions. Silence is fine.
    - For `update_backlog`, the content should be a single BACKLOG.md entry line.
    - For `create_issue`, the content should be a full issue body with Problem, Context, and AC sections.
    - Maximum 3 findings per run. Focus on the highest-leverage insight.
    - Mark alignment "away" if the fix would add complexity without addressing root cause.
    """
  end

  defp call_llm(prompt) do
    api_key = api_key()

    if is_nil(api_key) do
      {:error, "no ANTHROPIC_API_KEY set"}
    else
      body =
        Jason.encode!(%{
          model: @model,
          max_tokens: @max_tokens,
          system: system_prompt(),
          messages: [%{role: "user", content: prompt}]
        })

      tmp = Path.join(System.tmp_dir!(), "retro-#{System.unique_integer([:positive])}.json")
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
              {:ok, %{"content" => [%{"text" => text} | _]}} -> {:ok, text}
              {:ok, %{"error" => err}} -> {:error, "API error: #{inspect(err)}"}
              {:error, _} -> {:error, "invalid JSON response"}
            end

          {:error, msg, code} ->
            {:error, "curl failed (#{code}): #{String.slice(msg, 0, 200)}"}
        end
      after
        File.rm(tmp)
      end
    end
  end

  defp parse_response(text) do
    # Strip markdown code fences if present
    cleaned =
      text
      |> String.replace(~r/^```json\n?/m, "")
      |> String.replace(~r/\n?```$/m, "")
      |> String.trim()

    case Jason.decode(cleaned) do
      {:ok, %{"findings" => findings, "summary" => summary} = analysis}
      when is_list(findings) and is_binary(summary) ->
        Logger.info("[retro] summary: #{summary}")
        {:ok, analysis}

      {:ok, _} ->
        {:error, "unexpected JSON structure"}

      {:error, reason} ->
        {:error, "JSON parse failed: #{inspect(reason)}"}
    end
  end

  defp execute_actions(findings, run) do
    repo = run["repo"]

    Enum.each(findings, fn finding ->
      case finding["action"] do
        "create_issue" ->
          create_issue(repo, finding)

        "comment_issue" ->
          comment_issue(repo, finding)

        "update_backlog" ->
          update_backlog(finding)

        _ ->
          :ok
      end
    end)
  end

  defp create_issue(repo, finding) do
    title = "[retro] #{finding["title"]}"
    body = (finding["content"] || finding["description"]) <> "\n\n---\nCreated by conductor retro"

    tmp = Path.join(System.tmp_dir!(), "retro-issue-#{System.unique_integer([:positive])}.md")
    File.write!(tmp, body)

    try do
      case Shell.cmd("gh", [
             "issue",
             "create",
             "--repo",
             repo,
             "--title",
             title,
             "--label",
             "source/retro,p2",
             "--body-file",
             tmp
           ]) do
        {:ok, url} ->
          Logger.info("[retro] created issue: #{String.trim(url)}")

        {:error, msg, _} ->
          Logger.warning("[retro] issue creation failed: #{msg}")
      end
    after
      File.rm(tmp)
    end
  end

  defp comment_issue(repo, finding) do
    case finding["existing_issue"] do
      "#" <> num ->
        case Integer.parse(num) do
          {issue_number, _} ->
            comment =
              "**Retro finding:** #{finding["description"]}\n\n#{finding["content"] || ""}"

            GitHub.create_issue_comment(repo, issue_number, comment)
            Logger.info("[retro] commented on ##{issue_number}")

          :error ->
            Logger.warning("[retro] invalid issue number: #{num}")
        end

      _ ->
        Logger.warning("[retro] comment_issue but no existing_issue specified")
    end
  end

  defp update_backlog(finding) do
    backlog_path = Path.join(@repo_root, ".groom/BACKLOG.md")

    entry = finding["content"] || "- **#{finding["title"]}** — #{finding["description"]}"

    if File.exists?(backlog_path) do
      content = File.read!(backlog_path)

      updated =
        String.replace(
          content,
          "## Someday / Maybe",
          "- #{entry}\n\n## Someday / Maybe",
          global: false
        )

      if updated != content do
        File.write!(backlog_path, updated)
        Logger.info("[retro] updated BACKLOG.md: #{finding["title"]}")
      else
        # Heading not found — append to end of file instead
        File.write!(backlog_path, content <> "\n- #{entry}\n")
        Logger.info("[retro] appended to BACKLOG.md (heading not found): #{finding["title"]}")
      end
    else
      Logger.warning("[retro] no BACKLOG.md found at #{backlog_path}")
    end
  end

  # --- Helpers ---

  defp format_events(events) do
    events
    |> Enum.map(fn e ->
      "- [#{e["event_type"]}] #{e["created_at"]} #{inspect(e["payload"])}"
    end)
    |> Enum.join("\n")
    |> String.slice(0, 4_000)
  end

  defp duration(run) do
    case {run["picked_at"], run["completed_at"]} do
      {nil, _} -> "unknown"
      {_, nil} -> "still running"
      {picked, completed} -> "#{picked} → #{completed}"
    end
  end

  defp read_project_context do
    path = Path.join(@repo_root, "project.md")

    case File.read(path) do
      {:ok, content} -> String.slice(content, 0, 3_000)
      _ -> nil
    end
  end

  defp read_backlog do
    path = Path.join(@repo_root, ".groom/BACKLOG.md")

    case File.read(path) do
      {:ok, content} -> String.slice(content, 0, 2_000)
      _ -> nil
    end
  end

  defp list_open_issue_titles(run) do
    case Shell.cmd("gh", [
           "issue",
           "list",
           "--repo",
           run["repo"],
           "--state",
           "open",
           "--json",
           "number,title",
           "--limit",
           "30",
           "--jq",
           ".[] | \"#\\(.number) \\(.title)\""
         ]) do
      {:ok, output} -> String.slice(output, 0, 2_000)
      _ -> "(failed to fetch)"
    end
  end

  defp api_key do
    System.get_env("ANTHROPIC_API_KEY") ||
      System.get_env("CONDUCTOR_RETRO_API_KEY")
  end

  defp enabled? do
    Application.get_env(:conductor, :retro_enabled, true) and
      not is_nil(api_key())
  end

  defp action_count(findings) do
    findings
    |> Enum.count(&actionable?/1)
  end

  defp action_metadata(finding) do
    %{
      action: finding["action"],
      title: finding["title"],
      existing_issue: finding["existing_issue"]
    }
  end

  defp actionable?(finding), do: Map.get(finding, "action") in @supported_actions
end
