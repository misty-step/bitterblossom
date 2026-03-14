defmodule Conductor.Sprite do
  @moduledoc """
  Sprite operations via the `sprite` CLI.

  Deep module: hides all sprite protocol details — exec, dispatch,
  artifact retrieval, process cleanup.

  Implements `Conductor.Worker`.

  ## Dispatch sequence

  `dispatch/4` performs the full sequence via direct `sprite exec` calls:

  1. Kill stale agent processes (`pkill -9 -f claude`)
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
    harness = Keyword.get(opts, :harness, Conductor.ClaudeCode)
    # Injected in tests to capture exec calls without a real sprite
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    timeout_ms = timeout_minutes * 60_000

    # 1. Kill stale agent processes from prior dispatches
    exec_fn.(sprite, "pkill -9 -f claude 2>/dev/null; true", timeout: 15_000)

    # 2. Upload prompt (base64 to avoid shell quoting; base64 alphabet is shell-safe)
    prompt_path = Path.join(workspace, "PROMPT.md")
    encoded = Base.encode64(prompt)

    case exec_fn.(sprite, "echo #{encoded} | base64 -d > '#{prompt_path}'", timeout: 30_000) do
      {:error, msg, code} ->
        {:error, "prompt upload failed: #{msg}", code}

      {:ok, _} ->
        # 3. Run agent
        run_agent(sprite, workspace, prompt_path, harness, exec_fn, timeout_ms)
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
    case exec(sprite, "pkill -9 -f claude 2>/dev/null; true", timeout: 15_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec status(binary()) :: {:ok, map()} | {:error, term()}
  def status(sprite) do
    case exec(sprite, "echo ok", timeout: 15_000) do
      {:ok, _} -> {:ok, %{sprite: sprite, reachable: true}}
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec reachable?(binary()) :: boolean()
  def reachable?(sprite) do
    match?({:ok, _}, exec(sprite, "echo ok", timeout: 15_000))
  end

  @doc """
  Wakes a sleeping sprite and confirms it is responsive.

  Calls `sprite exec echo ok` which auto-wakes suspended machines on Fly.io.
  Returns `:ok` on success, `{:error, reason}` if the sprite is unreachable.
  """
  @spec wake(binary()) :: :ok | {:error, binary()}
  def wake(sprite) do
    case exec(sprite, "echo ok", timeout: 30_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  # --- Private ---

  defp run_agent(sprite, workspace, prompt_path, harness, exec_fn, timeout_ms) do
    cmd = agent_command(harness.dispatch_command([]), workspace, prompt_path)

    case exec_fn.(sprite, cmd, timeout: timeout_ms) do
      {:ok, output} ->
        {:ok, output}

      {:error, _output, _code} ->
        # Retry with session resumption if the harness supports it
        case harness.continue_command([]) do
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
    "cd '#{workspace}' && LEFTHOOK=0 #{cmd_str} < '#{prompt_path}'"
  end
end
