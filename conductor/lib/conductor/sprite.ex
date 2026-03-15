defmodule Conductor.Sprite do
  @moduledoc """
  Sprite operations via the `sprite` CLI.

  Deep module: hides all sprite protocol details — exec, dispatch,
  artifact retrieval, process cleanup.

  Implements `Conductor.Worker`.

  ## Dispatch sequence

  `dispatch/4` performs the full sequence via direct `sprite exec` calls:

  1. Kill stale agent processes (all known harnesses)
  2. Upload prompt to workspace (base64-encoded to avoid shell quoting issues)
  3. Run agent via `Conductor.Harness` (e.g. `claude -p < PROMPT.md`)
  4. On non-zero exit, retry once using the harness `continue_command`
  """

  @behaviour Conductor.Worker

  alias Conductor.{Shell, Config, Workspace}

  @impl Conductor.Worker
  @spec exec(binary(), binary(), keyword()) :: {:ok, binary()} | {:error, binary(), integer()}
  def exec(sprite, command, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 60_000)
    org = Keyword.get(opts, :org, Config.sprites_org!())

    Shell.cmd("sprite", ["-o", org, "-s", sprite, "exec", "bash", "-lc", command],
      timeout: timeout
    )
  end

  @spec exec!(binary(), binary(), keyword()) :: binary()
  def exec!(sprite, command, opts \\ []) do
    case exec(sprite, command, opts) do
      {:ok, output} -> output
      {:error, output, code} -> raise "sprite exec failed (#{code}): #{output}"
    end
  end

  @impl Conductor.Worker
  @spec dispatch(binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, binary(), integer()}
  def dispatch(sprite, prompt, _repo, opts \\ []) do
    timeout_minutes = Keyword.get(opts, :timeout, Config.builder_timeout())
    workspace = Keyword.fetch!(opts, :workspace)
    harness = Keyword.get(opts, :harness, Conductor.Codex)
    harness_opts = Keyword.get(opts, :harness_opts, [])
    # Injected in tests to capture exec calls without a real sprite
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    timeout_ms = timeout_minutes * 60_000

    # 1. Kill stale agent processes from prior dispatches
    exec_fn.(sprite, kill_agents_cmd(), timeout: 15_000)

    # 2. Upload prompt (base64 to avoid shell quoting; base64 alphabet is shell-safe)
    prompt_path = Path.join(workspace, "PROMPT.md")
    encoded = Base.encode64(prompt)

    case exec_fn.(sprite, "echo #{encoded} | base64 -d > '#{prompt_path}'", timeout: 30_000) do
      {:error, msg, code} ->
        {:error, "prompt upload failed: #{msg}", code}

      {:ok, _} ->
        # 3. Run agent
        run_agent(sprite, workspace, prompt_path, harness, harness_opts, exec_fn, timeout_ms)
    end
  end

  @impl Conductor.Worker
  @spec read_artifact(binary(), binary(), keyword()) :: {:ok, map()} | {:error, term()}
  def read_artifact(sprite, path, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 30_000)

    case exec(sprite, "cat '#{path}'", timeout: timeout) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} -> {:ok, data}
          {:error, _} -> {:error, "invalid JSON in artifact: #{String.slice(json, 0, 200)}"}
        end

      {:error, output, _} ->
        {:error, "artifact not found: #{output}"}
    end
  end

  @impl Conductor.Worker
  @spec cleanup(binary(), binary(), binary()) :: :ok | {:error, term()}
  def cleanup(sprite, repo, run_id) do
    Workspace.cleanup(sprite, repo, run_id)
  end

  @spec kill(binary()) :: :ok | {:error, term()}
  def kill(sprite) do
    case exec(sprite, kill_agents_cmd(), timeout: 15_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec status(binary(), keyword()) :: {:ok, map()} | {:error, term()}
  def status(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)
    harness = Keyword.get(opts, :harness)

    case exec_fn.(sprite, "echo ok", timeout: 15_000) do
      {:ok, _} ->
        harness_ready = harness_ready?(sprite, harness, exec_fn)
        gh_authenticated = gh_authenticated?(sprite, exec_fn)

        {:ok,
         %{
           sprite: sprite,
           reachable: true,
           harness_ready: harness_ready,
           gh_authenticated: gh_authenticated,
           healthy: harness_ready and gh_authenticated
         }}

      {:error, msg, _} ->
        {:error, msg}
    end
  end

  @spec reachable?(binary()) :: boolean()
  def reachable?(sprite) do
    match?({:ok, _}, exec(sprite, "echo ok", timeout: 15_000))
  end

  @doc "Check if a sprite has active agent processes (busy with a dispatch)."
  @spec busy?(binary(), keyword()) :: boolean()
  def busy?(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case exec_fn.(sprite, detect_agents_cmd(), timeout: 15_000) do
      {:ok, output} -> String.trim(output) != ""
      _ -> false
    end
  end

  # --- Private ---

  # Process names for all known agent harnesses. Used to kill stale processes
  # before dispatch and detect busy sprites. Update when adding a new harness.
  @agent_process_names ~w(claude codex)

  defp kill_agents_cmd do
    @agent_process_names
    |> Enum.map_join("; ", &"pkill -9 -f #{&1} 2>/dev/null")
    |> Kernel.<>("; true")
  end

  defp detect_agents_cmd do
    @agent_process_names
    |> Enum.map_join(" || ", &"pgrep -x #{&1} 2>/dev/null")
    |> Kernel.<>(" || pgrep -f 'ralph\\.sh' 2>/dev/null")
  end

  defp harness_ready?(_sprite, nil, _exec_fn), do: true
  defp harness_ready?(_sprite, "", _exec_fn), do: true

  defp harness_ready?(sprite, harness, exec_fn) do
    harness_cmd =
      case harness do
        "codex" -> "command -v codex"
        "claude-code" -> "command -v claude"
        _ -> "echo ok"
      end

    match?({:ok, _}, exec_fn.(sprite, harness_cmd, timeout: 15_000))
  end

  defp gh_authenticated?(sprite, exec_fn) do
    match?({:ok, _}, exec_fn.(sprite, "gh auth status >/dev/null 2>&1", timeout: 15_000))
  end

  defp run_agent(sprite, workspace, prompt_path, harness, harness_opts, exec_fn, timeout_ms) do
    cmd = agent_command(harness.dispatch_command(harness_opts), workspace, prompt_path)

    case exec_fn.(sprite, cmd, timeout: timeout_ms) do
      {:ok, output} ->
        {:ok, output}

      {:error, _output, _code} ->
        # Retry with session resumption if the harness supports it
        case harness.continue_command(harness_opts) do
          nil ->
            {:error, "agent exited non-zero; harness does not support continuation", 1}

          continue_parts ->
            retry_cmd = agent_command(continue_parts, workspace, prompt_path)
            exec_fn.(sprite, retry_cmd, timeout: timeout_ms)
        end
    end
  end

  defp agent_command(cmd_parts, workspace, prompt_path) do
    cmd_str = Enum.join(cmd_parts, " ")

    env_exports =
      Config.dispatch_env()
      |> Enum.map(fn {k, v} -> "#{k}=#{shell_quote(v)}" end)
      |> Enum.join(" ")

    prefix = if env_exports == "", do: "", else: env_exports <> " "
    "cd '#{workspace}' && #{prefix}LEFTHOOK=0 #{cmd_str} < '#{prompt_path}'"
  end

  defp shell_quote(value) do
    escaped = value |> to_string() |> String.replace("'", "'\"'\"'")
    "'#{escaped}'"
  end
end
