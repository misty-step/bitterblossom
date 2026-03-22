defmodule Conductor.Muse do
  @moduledoc """
  Reflection and synthesis worker for Bitterblossom's learning loop.

  Muse replaces the per-run retro path with a two-phase flow:

  1. Observe after merged runs and journal a structured reflection locally.
  2. Synthesize on a daily cadence and take at most three conservative actions.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Prompt, Shell, Store, Workspace}

  defstruct [
    :repo,
    :muse_sprite,
    :synthesis_interval_ms,
    :timer_ref,
    queue: [],
    pending_synthesis: false,
    in_flight: %{},
    failure_count: 0,
    health: :healthy
  ]

  @journal_root ".bb/muse"
  @reflection_dir Path.join(@journal_root, "reflections")
  @synthesis_dir Path.join(@journal_root, "syntheses")
  @max_actions 3

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec observe(binary()) :: :ok
  def observe(run_id) do
    cast_if_started({:observe, run_id})
  end

  @spec synthesize() :: :ok
  def synthesize do
    cast_if_started(:synthesize)
  end

  @spec status() :: map()
  def status do
    call_if_started(:status, %{
      repo: nil,
      muse_sprite: nil,
      queue_length: 0,
      in_flight: %{},
      pending_synthesis: false,
      health: :disabled,
      failure_count: 0
    })
  end

  @impl true
  def init(opts) do
    repo = Keyword.fetch!(opts, :repo)
    muse_sprite = Keyword.fetch!(opts, :muse_sprite)

    synthesis_interval_ms =
      Keyword.get(opts, :synthesis_interval_ms, Config.muse_synthesis_interval_ms())

    state = %__MODULE__{
      repo: repo,
      muse_sprite: muse_sprite,
      synthesis_interval_ms: synthesis_interval_ms
    }

    {:ok, schedule_synthesis(state)}
  end

  @impl true
  def handle_cast({:observe, run_id}, state) do
    state =
      state
      |> enqueue_run(run_id)
      |> maybe_dispatch()

    {:noreply, state}
  end

  @impl true
  def handle_cast(:synthesize, state) do
    state =
      %{state | pending_synthesis: true}
      |> maybe_dispatch()

    {:noreply, state}
  end

  @impl true
  def handle_call(:status, _from, state) do
    {:reply,
     %{
       repo: state.repo,
       muse_sprite: state.muse_sprite,
       queue_length: length(state.queue),
       in_flight: state.in_flight,
       pending_synthesis: state.pending_synthesis,
       health: state.health,
       failure_count: state.failure_count
     }, state}
  end

  @impl true
  def handle_info(:synthesis_tick, state) do
    state =
      state
      |> schedule_synthesis()
      |> Map.put(:pending_synthesis, true)
      |> maybe_dispatch()

    {:noreply, state}
  end

  @impl true
  def handle_info(:retry, state) do
    {:noreply, maybe_dispatch(state)}
  end

  @impl true
  def handle_info({ref, result}, state) when is_reference(ref) do
    Process.demonitor(ref, [:flush])
    {:noreply, complete_task(state, ref, result)}
  end

  @impl true
  def handle_info({:DOWN, _ref, :process, _pid, reason}, state)
      when reason in [:normal, :shutdown] do
    {:noreply, state}
  end

  @impl true
  def handle_info({:DOWN, ref, :process, _pid, reason}, state) do
    Logger.warning("[muse] dispatch task crashed: #{inspect(reason)}")
    {:noreply, complete_task(state, ref, {:error, "task_crashed: #{inspect(reason)}", 1})}
  end

  @impl true
  def handle_info(_msg, state), do: {:noreply, state}

  defp cast_if_started(message) do
    case Process.whereis(__MODULE__) do
      nil ->
        Logger.debug("[muse] not started, skipping #{inspect(message)}")
        :ok

      _pid ->
        GenServer.cast(__MODULE__, message)
    end

    :ok
  end

  defp call_if_started(message, fallback) do
    case Process.whereis(__MODULE__) do
      nil -> fallback
      _pid -> GenServer.call(__MODULE__, message)
    end
  end

  defp schedule_synthesis(state) do
    if state.timer_ref, do: Process.cancel_timer(state.timer_ref)

    ref = Process.send_after(self(), :synthesis_tick, state.synthesis_interval_ms)
    %{state | timer_ref: ref}
  end

  defp enqueue_run(state, run_id) do
    if run_id in state.queue or Map.has_key?(state.in_flight, {:observe, run_id}) do
      state
    else
      %{state | queue: state.queue ++ [run_id]}
    end
  end

  defp maybe_dispatch(%{in_flight: in_flight} = state) when map_size(in_flight) > 0, do: state

  defp maybe_dispatch(%{queue: [run_id | rest]} = state) do
    Logger.info("[muse] dispatching observation for #{run_id}")

    task =
      Task.Supervisor.async_nolink(Conductor.TaskSupervisor, fn ->
        observe_run(state.repo, state.muse_sprite, run_id)
      end)

    %{state | queue: rest, in_flight: %{{:observe, run_id} => task.ref}}
  end

  defp maybe_dispatch(%{pending_synthesis: true} = state) do
    Logger.info("[muse] dispatching synthesis")

    task =
      Task.Supervisor.async_nolink(Conductor.TaskSupervisor, fn ->
        run_synthesis(state.repo, state.muse_sprite)
      end)

    %{state | pending_synthesis: false, in_flight: %{synthesis: task.ref}}
  end

  defp maybe_dispatch(state), do: state

  defp complete_task(state, ref, result) do
    case Enum.find(state.in_flight, fn {_key, task_ref} -> task_ref == ref end) do
      {{:observe, run_id}, _task_ref} ->
        state
        |> Map.put(:in_flight, %{})
        |> handle_observation_result(run_id, result)
        |> maybe_dispatch()

      {:synthesis, _task_ref} ->
        state
        |> Map.put(:in_flight, %{})
        |> handle_synthesis_result(result)
        |> maybe_dispatch()

      nil ->
        state
    end
  end

  defp handle_observation_result(state, run_id, {:ok, metadata}) do
    Store.record_event(run_id, "muse_observation_complete", %{
      summary: metadata.summary,
      reflection_path: metadata.path
    })

    reset_health(state)
  end

  defp handle_observation_result(state, run_id, {:error, reason}) do
    Logger.warning("[muse] observation failed for #{run_id}: #{inspect(reason)}")
    Store.record_event(run_id, "muse_observation_failed", %{error: inspect(reason)})

    state
    |> enqueue_run(run_id)
    |> apply_backoff()
  end

  defp handle_synthesis_result(state, {:ok, metadata}) do
    Store.record_event("muse", "muse_synthesis_complete", %{
      summary: metadata.summary,
      action_count: length(metadata.actions_taken),
      actions_taken: metadata.actions_taken,
      synthesis_path: metadata.path
    })

    reset_health(state)
  end

  defp handle_synthesis_result(state, {:error, reason}) do
    Logger.warning("[muse] synthesis failed: #{inspect(reason)}")
    Store.record_event("muse", "muse_synthesis_failed", %{error: inspect(reason)})

    state
    |> Map.put(:pending_synthesis, true)
    |> apply_backoff()
  end

  defp apply_backoff(state) do
    count = state.failure_count + 1
    backoff_ms = min(trunc(Config.poll_seconds() * 1_000 * :math.pow(2, count)), 600_000)
    health = if count >= 3, do: :unavailable, else: :degraded

    Logger.info(
      "[muse] backoff: failures=#{count}, next_attempt=#{backoff_ms}ms, health=#{health}"
    )

    Process.send_after(self(), :retry, backoff_ms)
    %{state | failure_count: count, health: health}
  end

  defp reset_health(state) do
    if state.failure_count > 0 do
      Logger.info("[muse] recovered, resetting to healthy")
    end

    %{state | failure_count: 0, health: :healthy}
  end

  defp observe_run(repo, muse_sprite, run_id) do
    with {:ok, run} <- Store.get_run(run_id),
         events <- Store.list_events(run_id),
         :ok <- ensure_muse_dirs(),
         prompt <- Prompt.build_muse_observe_prompt(run, events, workspace_root(repo)),
         :ok <- workspace_mod().sync_persona(muse_sprite, workspace_root(repo), :muse),
         {:ok, output} <-
           worker_mod().dispatch(
             muse_sprite,
             prompt,
             repo,
             workspace: workspace_root(repo),
             persona_role: :muse,
             timeout: Config.muse_observation_timeout()
           ),
         {:ok, parsed} <- parse_observation(output),
         {:ok, path} <- write_reflection(run_id, run, parsed) do
      {:ok, %{summary: parsed.summary, path: path}}
    else
      {:error, msg, code} -> {:error, "dispatch failed (#{code}): #{msg}"}
      {:error, reason} -> {:error, reason}
    end
  end

  defp run_synthesis(repo, muse_sprite) do
    with :ok <- ensure_muse_dirs(),
         reflections <- read_recent_reflections(),
         prompt <-
           Prompt.build_muse_synthesis_prompt(
             repo,
             synthesis_context(repo, reflections),
             workspace_root(repo)
           ),
         :ok <- workspace_mod().sync_persona(muse_sprite, workspace_root(repo), :muse),
         {:ok, output} <-
           worker_mod().dispatch(
             muse_sprite,
             prompt,
             repo,
             workspace: workspace_root(repo),
             persona_role: :muse,
             timeout: Config.muse_synthesis_timeout(),
             harness_opts: [reasoning_effort: "high"]
           ),
         {:ok, parsed} <- parse_synthesis(output),
         actions_taken <- execute_actions(repo, parsed.actions |> Enum.take(@max_actions)),
         {:ok, path} <- write_synthesis(parsed.summary, actions_taken) do
      {:ok, %{summary: parsed.summary, actions_taken: actions_taken, path: path}}
    else
      {:error, msg, code} -> {:error, "dispatch failed (#{code}): #{msg}"}
      {:error, reason} -> {:error, reason}
    end
  end

  defp parse_observation(output) do
    with {:ok, decoded} <- decode_json(output),
         summary when is_binary(summary) <- Map.get(decoded, "summary"),
         reflection when is_binary(reflection) <- Map.get(decoded, "reflection") do
      {:ok, %{summary: summary, reflection: reflection}}
    else
      _ -> {:error, :invalid_observation_payload}
    end
  end

  defp parse_synthesis(output) do
    with {:ok, decoded} <- decode_json(output),
         summary when is_binary(summary) <- Map.get(decoded, "summary"),
         actions when is_list(actions) <- Map.get(decoded, "actions", []) do
      {:ok, %{summary: summary, actions: actions}}
    else
      _ -> {:error, :invalid_synthesis_payload}
    end
  end

  defp decode_json(output) do
    output
    |> String.trim()
    |> Jason.decode()
  end

  defp write_reflection(run_id, run, parsed) do
    path = reflection_path(run_id)

    body = """
    # Muse Reflection

    - Run ID: #{run_id}
    - Issue: ##{run["issue_number"]} - #{run["issue_title"]}
    - Summary: #{parsed.summary}
    - Recorded At: #{DateTime.utc_now() |> DateTime.to_iso8601()}

    #{parsed.reflection}
    """

    case File.write(path, body) do
      :ok -> {:ok, path}
      {:error, reason} -> {:error, reason}
    end
  end

  defp write_synthesis(summary, actions_taken) do
    timestamp =
      DateTime.utc_now()
      |> DateTime.to_iso8601()
      |> String.replace(":", "-")

    path = Path.join([repo_root(), @synthesis_dir, "#{timestamp}.md"])

    body = """
    # Muse Synthesis

    - Summary: #{summary}
    - Recorded At: #{DateTime.utc_now() |> DateTime.to_iso8601()}

    ## Actions

    #{Enum.map_join(actions_taken, "\n", &format_action/1)}
    """

    case File.write(path, body) do
      :ok -> {:ok, path}
      {:error, reason} -> {:error, reason}
    end
  end

  defp format_action(action) do
    issue =
      case action[:existing_issue] do
        nil -> ""
        value -> " (#{value})"
      end

    "- #{action.action}: #{action.title}#{issue}"
  end

  defp execute_actions(repo, actions) do
    open_issues = open_issues(repo)

    Enum.map(actions, fn action ->
      case Map.get(action, "action") do
        "comment_issue" ->
          comment_existing_issue(repo, action)

        "create_issue" ->
          maybe_create_issue(repo, action, open_issues)

        _ ->
          %{action: "none", title: Map.get(action, "title")}
      end
    end)
  end

  defp comment_existing_issue(repo, action) do
    case Map.get(action, "issue_number") do
      issue_number when is_integer(issue_number) ->
        :ok = issue_client_comment(repo, issue_number, Map.get(action, "body", ""))

        %{
          action: "comment_issue",
          title: Map.get(action, "title"),
          existing_issue: "##{issue_number}"
        }

      _ ->
        %{action: "none", title: Map.get(action, "title")}
    end
  end

  defp maybe_create_issue(repo, action, open_issues) do
    title = Map.get(action, "title", "Muse follow-up")
    body = Map.get(action, "body", "")

    case find_duplicate_issue(open_issues, title) do
      nil ->
        create_issue(repo, title, body)
        %{action: "create_issue", title: title}

      issue ->
        note = "Muse synthesis matched an existing open issue.\n\n#{body}"
        :ok = issue_client_comment(repo, issue.number, note)

        %{
          action: "comment_issue",
          title: title,
          existing_issue: "##{issue.number}",
          deduplicated: true
        }
    end
  end

  defp create_issue(repo, title, body) do
    tmp = Path.join(System.tmp_dir!(), "muse-issue-#{System.unique_integer([:positive])}.md")
    File.write!(tmp, body)

    try do
      case Shell.cmd("gh", [
             "issue",
             "create",
             "--repo",
             repo,
             "--title",
             "[muse] #{title}",
             "--label",
             "source/muse,p2",
             "--body-file",
             tmp
           ]) do
        {:ok, _url} -> :ok
        {:error, msg, _code} -> raise "issue creation failed: #{msg}"
      end
    after
      File.rm(tmp)
    end
  end

  defp open_issues(repo) do
    issue_client = issue_client_mod()

    if function_exported?(issue_client, :list_issues, 2) do
      case issue_client.list_issues(repo, limit: 100) do
        {:ok, issues} -> issues
        _ -> []
      end
    else
      []
    end
  end

  defp find_duplicate_issue(open_issues, title) do
    normalized = normalize_title(title)

    Enum.find(open_issues, fn issue ->
      issue_title = Map.get(issue, :title) || Map.get(issue, "title", "")
      normalize_title(issue_title) in [normalized, normalize_title("[muse] #{title}")]
    end)
  end

  defp normalize_title(title) do
    title
    |> to_string()
    |> String.downcase()
    |> String.replace(~r/^\[muse\]\s*/, "")
    |> String.trim()
  end

  defp issue_client_comment(repo, issue_number, body) do
    issue_client = issue_client_mod()

    cond do
      function_exported?(issue_client, :create_issue_comment, 3) ->
        issue_client.create_issue_comment(repo, issue_number, body)

      function_exported?(issue_client, :comment, 3) ->
        issue_client.comment(repo, issue_number, body)

      true ->
        :ok
    end
  end

  defp ensure_muse_dirs do
    File.mkdir_p!(Path.join(repo_root(), @reflection_dir))
    File.mkdir_p!(Path.join(repo_root(), @synthesis_dir))
    :ok
  end

  defp reflection_path(run_id) do
    Path.join([repo_root(), @reflection_dir, "#{Date.utc_today()}-#{run_id}.md"])
  end

  defp read_recent_reflections do
    Path.join(repo_root(), Path.join(@reflection_dir, "*.md"))
    |> Path.wildcard()
    |> Enum.sort(:desc)
    |> Enum.take(20)
    |> Enum.map(fn path ->
      %{path: path, body: File.read!(path)}
    end)
  end

  defp synthesis_context(repo, reflections) do
    %{
      reflections: reflections,
      project_context: read_repo_file("project.md", 4_000),
      backlog: read_repo_file(".groom/BACKLOG.md", 2_000),
      open_issues: open_issues(repo)
    }
  end

  defp read_repo_file(path, max_bytes) do
    case File.read(Path.join(repo_root(), path)) do
      {:ok, content} -> String.slice(content, 0, max_bytes)
      _ -> nil
    end
  end

  defp repo_root, do: Config.repo_root()
  defp workspace_root(repo), do: Workspace.repo_root(repo)
  defp worker_mod, do: Application.get_env(:conductor, :worker_module, Conductor.Sprite)
  defp workspace_mod, do: Application.get_env(:conductor, :workspace_module, Workspace)
  defp issue_client_mod, do: Application.get_env(:conductor, :tracker_module, Conductor.GitHub)
end
