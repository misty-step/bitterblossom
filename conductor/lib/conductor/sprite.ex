defmodule Conductor.Sprite do
  @moduledoc """
  Sprite operations via the `sprite` CLI.

  Deep module: hides all sprite protocol details — exec, dispatch,
  and process cleanup.

  Implements `Conductor.Worker`.

  ## Dispatch sequence

  `dispatch/4` performs the full sequence via direct `sprite exec` calls:

  1. Kill stale agent processes (all known harnesses)
  2. Upload prompt and runtime env file via `sprite exec --file`
  3. Run agent via `Conductor.Harness` (e.g. `claude -p < PROMPT.md`)
  4. On non-zero exit, retry once using the harness `continue_command`
  """

  @behaviour Conductor.Worker

  alias Conductor.{Shell, Config, Workspace}
  @runtime_env_file ".bb-runtime-env"

  @impl Conductor.Worker
  @spec exec(binary(), binary(), keyword()) :: {:ok, binary()} | {:error, binary(), integer()}
  def exec(sprite, command, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 60_000)
    org = Keyword.get(opts, :org, Config.sprites_org!())
    files = Keyword.get(opts, :files, [])

    Shell.cmd("sprite", exec_args(org, sprite, files, command), timeout: timeout)
  end

  @doc false
  @spec exec_args(binary(), binary(), list(), binary()) :: [binary()]
  def exec_args(org, sprite, files \\ [], command) do
    # "--" separates sprite CLI flags from the bash command.
    # Without it, bash's "-lc" is parsed as a sprite CLI flag.
    ["-o", org, "-s", sprite, "exec"] ++ file_args(files) ++ ["--", "bash", "-lc", command]
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

    with {:ok, persona_role} <- normalize_optional_persona_role(Keyword.get(opts, :persona_role)) do
      # 1. Kill stale agent processes from prior dispatches
      exec_fn.(sprite, kill_agents_cmd(), timeout: 15_000)

      prompt_path = Path.join(workspace, "PROMPT.md")
      runtime_env_path = Path.join(workspace, @runtime_env_file)

      # 2. Upload prompt and runtime env without embedding secrets in the remote argv
      case upload_dispatch_files(exec_fn, sprite, prompt_path, prompt, runtime_env_path) do
        {:error, msg, code} ->
          {:error, "dispatch file upload failed: #{msg}", code}

        {:ok, _} ->
          # 3. Run agent
          run_agent(
            sprite,
            workspace,
            prompt_path,
            persona_role,
            harness,
            harness_opts,
            exec_fn,
            timeout_ms
          )
      end
    else
      {:error, :invalid_role} ->
        {:error, "invalid persona role: #{inspect(Keyword.get(opts, :persona_role))}", 1}
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

  @doc """
  Kill agent processes and revoke GitHub auth on a sprite.

  Used during conductor shutdown to ensure surviving sprite processes
  cannot exercise merge authority. Defense in depth for governance
  invariant: the entity doing the work cannot judge the work.

  Best-effort: logs failures but always returns :ok so shutdown proceeds.
  Uses 5s timeouts to stay within GenServer terminate budget.
  """
  @spec kill_and_revoke(binary(), keyword()) :: :ok
  def kill_and_revoke(sprite, opts \\ []) do
    require Logger
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case exec_fn.(sprite, kill_agents_cmd(), timeout: 5_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> Logger.warning("[shutdown] kill agents on #{sprite}: #{msg}")
    end

    case exec_fn.(sprite, "gh auth logout --hostname github.com", timeout: 5_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> Logger.warning("[shutdown] gh auth logout on #{sprite}: #{msg}")
    end

    :ok
  end

  @spec status(binary(), keyword()) :: {:ok, map()} | {:error, term()}
  def status(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)
    harness = Keyword.get(opts, :harness)

    case probe(sprite, exec_fn: exec_fn) do
      {:ok, %{reachable: true}} ->
        harness_ready = harness_ready?(sprite, harness, exec_fn)
        gh_authenticated = gh_authenticated?(sprite, exec_fn)
        git_credential_helper = git_credential_helper_ready?(sprite, exec_fn)

        {:ok,
         %{
           sprite: sprite,
           reachable: true,
           harness_ready: harness_ready,
           gh_authenticated: gh_authenticated,
           git_credential_helper: git_credential_helper,
           healthy: harness_ready and gh_authenticated and git_credential_helper
         }}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @spec probe(binary(), keyword()) :: {:ok, map()} | {:error, term()}
  def probe(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case exec_fn.(sprite, "echo ok", timeout: 15_000) do
      {:ok, _} -> {:ok, %{sprite: sprite, reachable: true}}
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec reachable?(binary()) :: boolean()
  def reachable?(sprite) do
    match?({:ok, _}, probe(sprite))
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

  defp git_credential_helper_ready?(sprite, exec_fn) do
    case exec_fn.(sprite, "git config --global --get credential.helper", timeout: 15_000) do
      {:ok, output} -> output == "!gh auth git-credential"
      _ -> false
    end
  end

  defp run_agent(
         sprite,
         workspace,
         prompt_path,
         persona_role,
         harness,
         harness_opts,
         exec_fn,
         timeout_ms
       ) do
    cmd =
      agent_command(
        harness,
        harness.dispatch_command(harness_opts),
        workspace,
        prompt_path,
        persona_role
      )

    case exec_fn.(sprite, cmd, timeout: timeout_ms) do
      {:ok, output} ->
        {:ok, output}

      {:error, _output, _code} ->
        # Retry with session resumption if the harness supports it
        case harness.continue_command(harness_opts) do
          nil ->
            {:error, "agent exited non-zero; harness does not support continuation", 1}

          continue_parts ->
            retry_cmd =
              agent_command(harness, continue_parts, workspace, prompt_path, persona_role)

            exec_fn.(sprite, retry_cmd, timeout: timeout_ms)
        end
    end
  end

  defp agent_command(harness, cmd_parts, workspace, prompt_path, persona_role) do
    cmd_str = Enum.join(cmd_parts, " ")
    runtime_env_path = Path.join(workspace, @runtime_env_file)
    command_suffix = harness_command(harness, cmd_str, workspace, prompt_path, persona_role)

    "cd #{shell_quote(workspace)} && if [ -f #{shell_quote(runtime_env_path)} ]; then set -a; . #{shell_quote(runtime_env_path)}; set +a; fi && #{command_suffix}"
  end

  defp harness_command(_harness, cmd_str, _workspace, prompt_path, nil) do
    "LEFTHOOK=0 #{cmd_str} < #{shell_quote(prompt_path)}"
  end

  defp harness_command(Conductor.ClaudeCode, cmd_str, _workspace, prompt_path, _persona_role) do
    "LEFTHOOK=0 #{cmd_str} < #{shell_quote(prompt_path)}"
  end

  defp harness_command(Conductor.Codex, cmd_str, workspace, prompt_path, persona_role) do
    agents_path = persona_file_path(workspace, persona_role, "AGENTS.md")

    "cat #{shell_quote(agents_path)} #{shell_quote(prompt_path)} | LEFTHOOK=0 #{cmd_str}"
  end

  defp harness_command(_harness, cmd_str, workspace, prompt_path, persona_role) do
    agents_path = persona_file_path(workspace, persona_role, "AGENTS.md")

    "cat #{shell_quote(agents_path)} #{shell_quote(prompt_path)} | LEFTHOOK=0 #{cmd_str}"
  end

  defp persona_file_path(workspace, persona_role, filename) do
    workspace
    |> Workspace.persona_launch_dir(persona_role)
    |> Path.join(filename)
  end

  defp normalize_optional_persona_role(nil), do: {:ok, nil}

  defp normalize_optional_persona_role(role) do
    Workspace.normalize_persona_role(role)
  end

  defp shell_quote(value) do
    escaped = value |> to_string() |> String.replace("'", "'\"'\"'")
    "'#{escaped}'"
  end

  defp runtime_env_contents do
    body =
      Config.dispatch_env()
      |> Enum.map_join("\n", fn {key, value} -> "export #{key}=#{shell_quote(value)}" end)

    if body == "", do: "# managed by Conductor\n", else: body <> "\n"
  end

  defp upload_dispatch_files(exec_fn, sprite, prompt_path, prompt, runtime_env_path) do
    with_temp_file("sprite-prompt", prompt, fn prompt_file ->
      with_temp_file("sprite-env", runtime_env_contents(), fn env_file ->
        exec_fn.(sprite, "true",
          files: [{prompt_file, prompt_path}, {env_file, runtime_env_path}],
          timeout: 30_000
        )
      end)
    end)
  end

  defp with_temp_file(prefix, contents, fun) do
    path = Path.join(System.tmp_dir!(), "#{prefix}-#{System.unique_integer([:positive])}")
    File.write!(path, contents)
    File.chmod!(path, 0o600)

    try do
      fun.(path)
    after
      File.rm(path)
    end
  end

  defp file_args(files) do
    Enum.flat_map(files, fn {source, dest} ->
      ["--file", "#{source}:#{dest}"]
    end)
  end
end
