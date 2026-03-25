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
  @repo_root Path.expand("../../..", __DIR__)
  @sprite_home "/home/sprite"
  @sprite_claude_dir Path.join(@sprite_home, ".claude")
  @sprite_codex_dir Path.join(@sprite_home, ".codex")
  @sprite_codex_auth_path Path.join(@sprite_codex_dir, "auth.json")
  @sprite_runtime_dir Path.join(@sprite_home, ".bitterblossom")
  @sprite_runtime_env_path Path.join(@sprite_runtime_dir, "runtime.env")
  @sprite_workspace_root Path.join(@sprite_home, "workspace")
  @sprite_persona_path Path.join(@sprite_workspace_root, "PERSONA.md")
  @sprite_prompt_template_path Path.join(@sprite_workspace_root, ".builder-prompt-template.md")
  @workspace_metadata_rel_path ".bb/workspace.json"
  @log_file "ralph.log"
  @probe_marker "__bb_probe__"
  @wake_marker "__bb_wake__"

  @impl Conductor.Worker
  @spec exec(binary(), binary(), keyword()) :: {:ok, binary()} | {:error, binary(), integer()}
  def exec(sprite, command, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 60_000)
    org = keyword_fetch_or(opts, :org, &Config.sprites_org!/0)
    files = Keyword.get(opts, :files, [])
    transport = Keyword.get(opts, :transport, :websocket)
    shell_cmd_fn = Keyword.get(opts, :shell_cmd_fn, &Shell.cmd/3)

    case shell_cmd_fn.(
           "sprite",
           exec_args(org, sprite, files, command, transport: transport),
           timeout: timeout
         ) do
      {:ok, _} = ok ->
        ok

      {:error, msg, _code} = error ->
        if wake_recoverable?(msg) and transport == :websocket do
          with :ok <-
                 wake(sprite,
                   org: org,
                   timeout: 15_000,
                   shell_cmd_fn: shell_cmd_fn
                 ) do
            shell_cmd_fn.(
              "sprite",
              exec_args(org, sprite, files, command, transport: :http_post),
              timeout: timeout
            )
          else
            {:error, _reason} -> error
          end
        else
          error
        end
    end
  end

  @doc false
  @spec exec_args(binary(), binary(), binary()) :: [binary()]
  def exec_args(org, sprite, command), do: exec_args(org, sprite, [], command, [])

  @doc false
  @spec exec_args(binary(), binary(), list(), binary()) :: [binary()]
  def exec_args(org, sprite, files, command), do: exec_args(org, sprite, files, command, [])

  @doc false
  @spec exec_args(binary(), binary(), list(), binary(), keyword()) :: [binary()]
  def exec_args(org, sprite, files, command, opts) do
    transport_args =
      case Keyword.get(opts, :transport, :websocket) do
        :http_post -> ["--http-post"]
        _ -> []
      end

    # "--" separates sprite CLI flags from the bash command.
    # Without it, bash's "-lc" is parsed as a sprite CLI flag.
    ["-o", org, "-s", sprite, "exec"] ++
      transport_args ++ file_args(files) ++ ["--", "bash", "-lc", command]
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

  @spec wake(binary(), keyword()) :: :ok | {:error, term()}
  def wake(sprite, opts \\ []) do
    case marker_exec(
           sprite,
           @wake_marker,
           Keyword.merge([timeout: 15_000, transport: :http_post], opts)
         ) do
      {:ok, _} -> :ok
      {:error, msg} -> {:error, msg}
    end
  end

  @spec provision(binary(), keyword()) :: :ok | {:error, term()}
  def provision(sprite, opts \\ []) do
    repo = Keyword.get(opts, :repo)
    persona = Keyword.get(opts, :persona)
    force = Keyword.get(opts, :force, false)
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    with :ok <- ensure_remote_dirs(sprite, exec_fn),
         :ok <- upload_base_configs(sprite, persona, exec_fn),
         :ok <- ensure_codex(sprite, force, exec_fn),
         :ok <- maybe_sync_codex_auth(sprite, exec_fn),
         :ok <- upload_runtime_env(sprite, exec_fn),
         :ok <- configure_git_auth(sprite, exec_fn),
         :ok <- maybe_setup_repo(sprite, repo, persona, force, exec_fn) do
      :ok
    end
  end

  @spec logs(binary(), keyword()) :: :ok | {:error, term()}
  def logs(sprite, opts \\ []) do
    lines = Keyword.get(opts, :lines, 0)
    follow = Keyword.get(opts, :follow, false)
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)
    runner_fn = Keyword.get(opts, :runner_fn, &stream_exec/3)

    with :ok <- validate_log_lines(lines),
         {:ok, workspace} <- find_workspace(sprite, opts, exec_fn),
         :ok <- ensure_logs_available(sprite, workspace, exec_fn) do
      case runner_fn.(sprite, logs_command(workspace, follow, lines), opts) do
        {:ok, _} -> :ok
        {:error, msg, _code} -> {:error, msg}
      end
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
        codex_auth_ready = codex_auth_ready?(sprite, harness, exec_fn)
        gh_authenticated = gh_authenticated?(sprite, exec_fn)
        git_credential_helper = git_credential_helper_ready?(sprite, exec_fn)

        {:ok,
         %{
           sprite: sprite,
           reachable: true,
           harness_ready: harness_ready,
           codex_auth_ready: codex_auth_ready,
           gh_authenticated: gh_authenticated,
           git_credential_helper: git_credential_helper,
           healthy:
             harness_ready and codex_auth_ready and gh_authenticated and git_credential_helper
         }}

      {:error, reason} ->
        {:error, reason}
    end
  end

  @spec probe(binary(), keyword()) :: {:ok, map()} | {:error, term()}
  def probe(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case marker_exec(exec_fn, sprite, @probe_marker, timeout: 15_000, transport: :http_post) do
      {:ok, _} -> {:ok, %{sprite: sprite, reachable: true}}
      {:error, msg} -> {:error, msg}
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

  defp codex_auth_ready?(_sprite, harness, _exec_fn) when harness not in ["codex"], do: true

  defp codex_auth_ready?(sprite, "codex", exec_fn) do
    remote_codex_auth_present?(sprite, exec_fn) or
      match?({:api_key, _}, Config.codex_auth_source())
  end

  defp remote_codex_auth_present?(sprite, exec_fn) do
    match?(
      {:ok, _},
      exec_fn.(sprite, "test -s #{shell_quote(@sprite_codex_auth_path)}", timeout: 15_000)
    )
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
    log_path = Path.join(workspace, @log_file)
    command_suffix = harness_command(harness, cmd_str, workspace, prompt_path, persona_role)

    "cd #{shell_quote(workspace)} && : > #{shell_quote(log_path)} && if [ -f #{shell_quote(runtime_env_path)} ]; then set -a; . #{shell_quote(runtime_env_path)}; set +a; fi && set -o pipefail && #{command_suffix} 2>&1 | tee -a #{shell_quote(log_path)}"
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

  defp ensure_remote_dirs(sprite, exec_fn) do
    dirs =
      base_uploads()
      |> Enum.map(fn {_src, dest} -> Path.dirname(dest) end)
      |> Kernel.++([
        @sprite_claude_dir,
        Path.join(@sprite_claude_dir, "hooks"),
        Path.join(@sprite_claude_dir, "skills"),
        Path.join(@sprite_claude_dir, "commands"),
        Path.join(@sprite_claude_dir, "prompts"),
        @sprite_codex_dir,
        @sprite_runtime_dir,
        @sprite_workspace_root
      ])
      |> Enum.uniq()
      |> Enum.sort()

    cmd =
      "mkdir -p " <>
        Enum.map_join(dirs, " ", &shell_quote/1)

    case exec_fn.(sprite, cmd, timeout: 30_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, msg}
    end
  end

  defp upload_base_configs(sprite, persona, exec_fn) do
    settings_contents =
      Path.join([@repo_root, "base", "settings.json"])
      |> File.read!()
      |> String.replace(
        "__SET_VIA_OPENROUTER_API_KEY_ENV__",
        System.get_env("OPENROUTER_API_KEY", "")
      )

    prompt_template_path = Config.prompt_template()
    persona_contents = persona_contents(sprite, persona)

    with_temp_file("sprite-settings", settings_contents, fn settings_file ->
      with_temp_file("sprite-persona", persona_contents, fn persona_file ->
        files =
          base_uploads() ++
            [
              {settings_file, Path.join(@sprite_claude_dir, "settings.json")},
              {prompt_template_path, @sprite_prompt_template_path},
              {persona_file, @sprite_persona_path}
            ]

        case exec_fn.(sprite, "true", files: files, timeout: 60_000) do
          {:ok, _} -> :ok
          {:error, msg, _code} -> {:error, msg}
        end
      end)
    end)
  end

  defp ensure_codex(sprite, force, exec_fn) do
    install_cmd =
      if force do
        "npm i -g @openai/codex"
      else
        "command -v codex >/dev/null 2>&1 || npm i -g @openai/codex"
      end

    case exec_fn.(sprite, install_cmd, timeout: 120_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, msg}
    end
  end

  defp maybe_sync_codex_auth(sprite, exec_fn) do
    case Config.codex_auth_source() do
      {:chatgpt, local_auth_path} ->
        case exec_fn.(sprite, "test -s #{shell_quote(@sprite_codex_auth_path)}", timeout: 15_000) do
          {:ok, _} ->
            :ok

          {:error, "", 1} ->
            case exec_fn.(
                   sprite,
                   "chmod 600 #{shell_quote(@sprite_codex_auth_path)}",
                   files: [{local_auth_path, @sprite_codex_auth_path}],
                   timeout: 30_000
                 ) do
              {:ok, _} -> :ok
              {:error, msg, _code} -> {:error, msg}
            end

          {:error, msg, _code} ->
            {:error, msg}
        end

      _ ->
        :ok
    end
  end

  defp upload_runtime_env(sprite, exec_fn) do
    with_temp_file("sprite-runtime-env", runtime_env_contents(), fn runtime_env_file ->
      case exec_fn.(sprite, "true",
             files: [{runtime_env_file, @sprite_runtime_env_path}],
             timeout: 30_000
           ) do
        {:ok, _} -> :ok
        {:error, msg, _code} -> {:error, msg}
      end
    end)
  end

  defp configure_git_auth(sprite, exec_fn) do
    token = Config.github_token!()
    token_path = "/tmp/bb-gh-token-#{System.unique_integer([:positive])}"

    with_temp_file("sprite-gh-token", token <> "\n", fn token_file ->
      case exec_fn.(sprite, persist_git_auth_script(token_path),
             files: [{token_file, token_path}],
             timeout: 30_000
           ) do
        {:ok, _} -> :ok
        {:error, msg, _code} -> {:error, msg}
      end
    end)
  end

  defp maybe_setup_repo(_sprite, nil, _persona, _force, _exec_fn), do: :ok
  defp maybe_setup_repo(_sprite, "", _persona, _force, _exec_fn), do: :ok

  defp maybe_setup_repo(sprite, repo, persona, force, exec_fn) do
    with :ok <- validate_repo(repo) do
      repo_dir = sprite_repo_workspace(repo)

      setup_cmd =
        [
          repo_setup_script(repo_dir, repo, force) |> String.trim(),
          "mkdir -p #{shell_quote(Path.join(repo_dir, ".claude/skills"))} #{shell_quote(Path.join(repo_dir, ".claude/commands"))} #{shell_quote(Path.join(repo_dir, ".bb"))}"
        ]
        |> Enum.join(" && ")

      setup_result =
        case exec_fn.(sprite, setup_cmd, timeout: 120_000) do
          {:ok, _} -> :ok
          {:error, msg, _code} -> {:error, msg}
        end

      with :ok <- setup_result do
        metadata =
          Jason.encode!(%{
            schema_version: 1,
            repo: repo,
            repo_dir: repo_dir,
            sprite: sprite,
            persona: persona_contents(sprite, persona),
            configured_at:
              DateTime.utc_now() |> DateTime.truncate(:second) |> DateTime.to_iso8601()
          }) <> "\n"

        with_temp_file("sprite-workspace", metadata, fn metadata_file ->
          case exec_fn.(sprite, "true",
                 files: [{metadata_file, Path.join(repo_dir, @workspace_metadata_rel_path)}],
                 timeout: 30_000
               ) do
            {:ok, _} -> :ok
            {:error, msg, _code} -> {:error, msg}
          end
        end)
      end
    end
  end

  defp validate_log_lines(lines) when is_integer(lines) and lines >= 0, do: :ok
  defp validate_log_lines(_lines), do: {:error, "--lines must be >= 0"}

  defp find_workspace(sprite, opts, exec_fn) do
    case Keyword.get(opts, :workspace) do
      workspace when is_binary(workspace) ->
        {:ok, workspace}

      _ ->
        case active_worktree(sprite) do
          {:ok, path} ->
            {:ok, path}

          :error ->
            case exec_fn.(sprite, workspace_discovery_script(), timeout: 15_000) do
              {:ok, output} ->
                workspace = String.trim(output)

                if workspace == "" do
                  {:error,
                   ~s(sprite "#{sprite}" has no workspace repo; reconcile the fleet before tailing logs)}
                else
                  {:ok, String.trim_trailing(workspace, "/")}
                end

              {:error, msg, _code} ->
                {:error, msg}
            end
        end
    end
  end

  defp active_worktree(sprite) do
    Conductor.Store.list_runs(limit: 50)
    |> Enum.find(fn run ->
      run["builder_sprite"] == sprite and run["worktree_path"] not in [nil, ""] and
        run["status"] not in ["merged", "blocked", "failed"]
    end)
    |> case do
      %{"worktree_path" => path} -> {:ok, path}
      _ -> :error
    end
  end

  defp ensure_logs_available(sprite, workspace, exec_fn) do
    log_path = Path.join(workspace, @log_file)
    busy = busy?(sprite, exec_fn: exec_fn)

    case exec_fn.(sprite, "test -s #{shell_quote(log_path)}", timeout: 15_000) do
      {:ok, _} ->
        :ok

      {:error, "", 1} when busy ->
        :ok

      {:error, "", 1} ->
        {:error,
         "No active task on #{inspect(sprite)}.\nThe sprite is reachable, but no agent is running and the dispatch log is empty.\nTry: mix conductor fleet"}

      {:error, msg, _code} ->
        {:error, msg}
    end
  end

  defp logs_command(workspace, follow, lines) do
    log_path = Path.join(workspace, @log_file)

    cond do
      follow and lines > 0 ->
        "touch #{shell_quote(log_path)} && tail -n #{lines} -f #{shell_quote(log_path)}"

      follow ->
        "touch #{shell_quote(log_path)} && tail -n 50 -f #{shell_quote(log_path)}"

      lines > 0 ->
        "touch #{shell_quote(log_path)} && tail -n #{lines} #{shell_quote(log_path)}"

      true ->
        "touch #{shell_quote(log_path)} && cat #{shell_quote(log_path)}"
    end
  end

  defp stream_exec(sprite, command, opts) do
    org = keyword_fetch_or(opts, :org, &Config.sprites_org!/0)
    args = exec_args(org, sprite, Keyword.get(opts, :files, []), command)
    into = Keyword.get(opts, :into, IO.stream(:stdio, :line))
    {output, code} = System.cmd("sprite", args, stderr_to_stdout: true, into: into)

    case code do
      0 -> {:ok, output}
      _ -> {:error, output, code}
    end
  end

  defp base_uploads do
    required_files = [
      {Path.join([@repo_root, "base", "CLAUDE.md"]), Path.join(@sprite_claude_dir, "CLAUDE.md")},
      {Path.join([@repo_root, "base", "codex-config.toml"]),
       Path.join(@sprite_codex_dir, "config.toml")},
      {Path.join([@repo_root, "base", "codex-instructions.md"]),
       Path.join(@sprite_codex_dir, "instructions.md")}
    ]

    missing_required =
      required_files
      |> Enum.reject(fn {source, _dest} -> File.regular?(source) end)
      |> Enum.map(&elem(&1, 0))

    if missing_required != [] do
      raise "missing required sprite base assets: #{Enum.join(missing_required, ", ")}"
    end

    optional_files =
      wildcard_uploads("base/hooks/*.py", Path.join(@sprite_claude_dir, "hooks")) ++
        wildcard_uploads("base/commands/*.md", Path.join(@sprite_claude_dir, "commands")) ++
        wildcard_uploads("base/prompts/*.md", Path.join(@sprite_claude_dir, "prompts")) ++
        recursive_uploads("base/skills", Path.join(@sprite_claude_dir, "skills"))

    required_files ++ Enum.filter(optional_files, fn {source, _dest} -> File.regular?(source) end)
  end

  defp wildcard_uploads(pattern, dest_root) do
    Path.wildcard(Path.join(@repo_root, pattern))
    |> Enum.map(fn source -> {source, Path.join(dest_root, Path.basename(source))} end)
  end

  defp recursive_uploads(relative_root, dest_root) do
    local_root = Path.join(@repo_root, relative_root)

    if File.dir?(local_root) do
      Path.wildcard(Path.join(local_root, "**/*"))
      |> Enum.filter(&File.regular?/1)
      |> Enum.map(fn source ->
        relative = Path.relative_to(source, local_root)
        {source, Path.join(dest_root, relative)}
      end)
    else
      []
    end
  end

  defp persona_contents(sprite, nil), do: default_persona(sprite)
  defp persona_contents(sprite, ""), do: default_persona(sprite)
  defp persona_contents(_sprite, persona), do: persona <> "\n"

  defp default_persona(sprite) do
    "You are #{sprite}. Read CLAUDE.md and WORKFLOW.md before acting.\n"
  end

  defp persist_git_auth_script(token_path) do
    """
    set -e
    trap 'rm -f #{shell_quote(token_path)}' EXIT
    gh auth login --with-token < #{shell_quote(token_path)} >/dev/null
    gh auth status >/dev/null
    git config --global credential.helper '!gh auth git-credential'
    test "$(git config --global --get credential.helper)" = "!gh auth git-credential"
    git config --global user.name "bitterblossom[bot]"
    git config --global user.email "bitterblossom@misty-step.dev"
    git config --global --add safe.directory '*'
    """
  end

  defp validate_repo(repo) when is_binary(repo) do
    with :ok <- Workspace.validate_input(repo),
         [owner, name] <- String.split(repo, "/", parts: 2),
         true <- valid_repo_segment?(owner),
         true <- valid_repo_segment?(name) do
      :ok
    else
      _ -> {:error, "invalid repo format: #{inspect(repo)}"}
    end
  end

  defp validate_repo(repo), do: {:error, "invalid repo format: #{inspect(repo)}"}

  defp repo_setup_script(repo_dir, repo, true) do
    """
    rm -rf #{shell_quote(repo_dir)} &&
      cd #{shell_quote(@sprite_workspace_root)} &&
      git clone #{shell_quote(repo_clone_url(repo))}
    """
  end

  defp repo_setup_script(repo_dir, repo, false) do
    """
    if [ -d #{shell_quote(repo_dir)} ]; then
      cd #{shell_quote(repo_dir)} &&
        (git checkout master 2>/dev/null || git checkout main 2>/dev/null) &&
        git pull --ff-only
    else
      cd #{shell_quote(@sprite_workspace_root)} &&
        git clone #{shell_quote(repo_clone_url(repo))}
    fi
    """
  end

  defp repo_clone_url(repo), do: "https://github.com/#{repo}.git"

  defp sprite_repo_workspace(repo) do
    repo |> String.split("/") |> List.last() |> then(&Path.join(@sprite_workspace_root, &1))
  end

  defp valid_repo_segment?(segment) do
    Regex.match?(~r/^[A-Za-z0-9_.-]+$/, segment)
  end

  defp workspace_discovery_script do
    """
    set -euo pipefail
    shopt -s globstar nullglob

    meta=$(ls -dt #{@sprite_workspace_root}/**/#{@workspace_metadata_rel_path} 2>/dev/null | head -1 || true)
    if [ -n "$meta" ]; then
      printf '%s\n' "${meta%/#{@workspace_metadata_rel_path}}"
      exit 0
    fi

    prompt=$(ls -dt #{@sprite_workspace_root}/**/PROMPT.md 2>/dev/null | head -1 || true)
    if [ -n "$prompt" ]; then
      printf '%s\n' "${prompt%/PROMPT.md}"
      exit 0
    fi

    log=$(ls -dt #{@sprite_workspace_root}/**/#{@log_file} 2>/dev/null | head -1 || true)
    if [ -n "$log" ]; then
      printf '%s\n' "${log%/#{@log_file}}"
      exit 0
    fi

    ws=$(ls -d #{@sprite_workspace_root}/*/ 2>/dev/null | head -1 || true)
    printf '%s\n' "${ws%/}"
    """
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

  # The sprite CLI currently exposes these transport failures as stderr text, not
  # typed exit codes. Keep the accepted phrases narrow and covered by tests so the
  # wake fallback does not silently drift if the CLI wording changes.
  defp wake_recoverable?(message) when is_binary(message) do
    String.contains?(message, ["bad handshake", "HTTP 502", "failed to connect"])
  end

  defp marker_exec(sprite, marker, opts) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)
    marker_exec(exec_fn, sprite, marker, opts)
  end

  defp marker_exec(exec_fn, sprite, marker, opts) do
    case exec_fn.(sprite, marker_command(marker), opts) do
      {:ok, _} = ok ->
        ok

      {:error, message, _code} ->
        if marker_false_negative?(message, marker) do
          {:ok, message}
        else
          {:error, message}
        end
    end
  end

  defp marker_command(marker) do
    "printf #{shell_quote(marker)}"
  end

  defp marker_false_negative?(message, marker) when is_binary(message) do
    String.contains?(message, marker) and String.contains?(message, "no exit frame received")
  end

  defp keyword_fetch_or(opts, key, fallback) do
    case Keyword.fetch(opts, key) do
      {:ok, value} -> value
      :error -> fallback.()
    end
  end
end
