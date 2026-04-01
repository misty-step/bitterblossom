defmodule Conductor.Sprite do
  @moduledoc """
  Sprite operations via the `sprite` CLI.

  Deep module: hides all sprite protocol details — exec, lifecycle,
  and process management.

  Core operations: `exec/3`, `start_loop/4`, `stop_loop/1`,
  `pause/1`, `resume/1`, `provision/2`, `status/2`, `logs/2`.
  """

  alias Conductor.{Shell, Config, Workspace}
  @runtime_env_file ".bb-runtime-env"
  @repo_root Path.expand("../../..", __DIR__)
  @sprite_home "/home/sprite"
  @sprite_claude_dir Path.join(@sprite_home, ".claude")
  @sprite_codex_dir Path.join(@sprite_home, ".codex")
  @sprite_codex_auth_path Path.join(@sprite_codex_dir, "auth.json")
  @sprite_runtime_dir Path.join(@sprite_home, ".bitterblossom")
  @sprite_runtime_env_path Path.join(@sprite_runtime_dir, "runtime.env")
  @sprite_pause_path Path.join(@sprite_runtime_dir, "paused")
  @sprite_loop_lock_path Path.join(@sprite_runtime_dir, "loop.lock")
  @sprite_loop_pid_path Path.join(@sprite_runtime_dir, "loop.pid")
  @sprite_workspace_root Path.join(@sprite_home, "workspace")
  @sprite_persona_path Path.join(@sprite_workspace_root, "PERSONA.md")
  @sprite_prompt_template_path Path.join(@sprite_workspace_root, ".builder-prompt-template.md")
  @workspace_metadata_rel_path ".bb/workspace.json"
  @log_file "ralph.log"
  @probe_marker "__bb_probe__"
  @wake_marker "__bb_wake__"
  @start_loop_started_prefix "__bb_started__:"
  @start_loop_paused_marker "__bb_paused__"
  @start_loop_busy_marker "__bb_busy__"

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
                   timeout: 45_000,
                   shell_cmd_fn: shell_cmd_fn
                 ) do
            shell_cmd_fn.(
              "sprite",
              exec_args(org, sprite, files, command, transport: :websocket),
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

  @spec start_loop(binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, binary(), integer()}
  def start_loop(sprite, prompt, repo, opts \\ []) do
    workspace = Keyword.fetch!(opts, :workspace)
    harness = Keyword.get(opts, :harness, Conductor.Codex)
    harness_opts = Keyword.get(opts, :harness_opts, [])
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    with {:ok, persona_role} <- normalize_optional_persona_role(Keyword.get(opts, :persona_role)) do
      prompt_path = Path.join(workspace, "PROMPT.md")
      runtime_env_path = Path.join(workspace, @runtime_env_file)

      case upload_dispatch_files(exec_fn, sprite, prompt_path, prompt, runtime_env_path, repo) do
        {:error, msg, code} ->
          {:error, "dispatch file upload failed: #{msg}", code}

        {:ok, _} ->
          detached_cmd =
            detached_agent_command(
              harness,
              harness.dispatch_command(harness_opts),
              workspace,
              prompt_path,
              persona_role
            )

          case exec_fn.(sprite, detached_cmd, timeout: 30_000) do
            {:ok, output} when is_binary(output) ->
              parse_start_loop_output(output)

            {:error, msg, code} ->
              {:error, msg, code}
          end
      end
    else
      {:error, :invalid_role} ->
        {:error, "invalid persona role: #{inspect(Keyword.get(opts, :persona_role))}", 1}
    end
  end

  @spec stop_loop(binary(), keyword()) :: :ok | {:error, term()}
  def stop_loop(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    command = """
    set -e
    if [ -s #{shell_quote(@sprite_loop_pid_path)} ]; then
      pid=$(cat #{shell_quote(@sprite_loop_pid_path)})
      kill "$pid" 2>/dev/null || true
      sleep 1
      kill -9 "$pid" 2>/dev/null || true
    fi
    #{kill_agents_cmd()}
    sleep 1
    rm -f #{shell_quote(@sprite_loop_pid_path)} #{shell_quote(@sprite_loop_lock_path)}
    """

    case exec_fn.(sprite, command, timeout: 15_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, msg}
    end
  end

  @spec pause(binary(), keyword()) :: :ok | {:error, term()}
  def pause(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case exec_fn.(
           sprite,
           "mkdir -p #{shell_quote(@sprite_runtime_dir)} && touch #{shell_quote(@sprite_pause_path)}",
           timeout: 15_000
         ) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, msg}
    end
  end

  @spec resume(binary(), keyword()) :: :ok | {:error, term()}
  def resume(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case exec_fn.(sprite, "rm -f #{shell_quote(@sprite_pause_path)}", timeout: 15_000) do
      {:ok, _} -> :ok
      {:error, msg, _code} -> {:error, msg}
    end
  end

  @spec paused?(binary(), keyword()) :: boolean()
  def paused?(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case exec_fn.(
           sprite,
           "if [ -e #{shell_quote(@sprite_pause_path)} ]; then printf 'paused'; fi",
           timeout: 15_000
         ) do
      {:ok, output} -> String.trim(output) == "paused"
      _ -> false
    end
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
           Keyword.merge([timeout: 45_000], opts)
         ) do
      {:ok, _} -> :ok
      {:error, msg} -> {:error, msg}
    end
  end

  @spec provision(binary(), keyword()) :: :ok | {:error, term()}
  def provision(sprite, opts \\ []) do
    repo = Keyword.get(opts, :repo)
    persona = Keyword.get(opts, :persona)
    harness = Keyword.get(opts, :harness)
    force = Keyword.get(opts, :force, false)
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    with :ok <- ensure_remote_dirs(sprite, exec_fn),
         :ok <- upload_base_configs(sprite, persona, exec_fn),
         :ok <- ensure_codex(sprite, force, exec_fn),
         :ok <- maybe_sync_codex_auth(sprite, harness, exec_fn),
         :ok <- upload_runtime_env(sprite, repo, exec_fn),
         :ok <- configure_git_auth(sprite, exec_fn),
         :ok <- maybe_setup_repo(sprite, repo, persona, force, exec_fn),
         :ok <- Conductor.Bootstrap.ensure_spellbook(sprite, exec_fn: exec_fn) do
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
  Force-upload the local codex auth.json to a sprite.

  Always overwrites — handles stale tokens from refresh_token_reused errors.
  No-ops if the harness isn't codex or no local auth is available.
  """
  @spec force_sync_codex_auth(binary(), keyword()) :: :ok | {:error, term()}
  def force_sync_codex_auth(sprite, opts \\ []) do
    exec_fn = Keyword.get(opts, :exec_fn, &exec/3)

    case Config.codex_auth_source() do
      {:chatgpt, local_auth_path} ->
        case exec_fn.(
               sprite,
               "chmod 600 #{shell_quote(@sprite_codex_auth_path)}",
               files: [{local_auth_path, @sprite_codex_auth_path}],
               timeout: 30_000
             ) do
          {:ok, _} -> :ok
          {:error, msg, _code} -> {:error, msg}
        end

      _ ->
        :ok
    end
  end

  @doc "Kill agent processes and revoke GitHub auth. Best-effort, always returns :ok."
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
        paused = paused?(sprite, exec_fn: exec_fn)
        busy = busy?(sprite, exec_fn: exec_fn)
        loop_pid = loop_pid(sprite, exec_fn)

        loop_alive = loop_pid != nil

        {:ok,
         %{
           sprite: sprite,
           reachable: true,
           harness_ready: harness_ready,
           codex_auth_ready: codex_auth_ready,
           gh_authenticated: gh_authenticated,
           git_credential_helper: git_credential_helper,
           paused: paused,
           busy: busy,
           loop_pid: loop_pid,
           loop_alive: loop_alive,
           lifecycle_status: lifecycle_status(paused, busy),
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

    case marker_exec(exec_fn, sprite, @probe_marker, timeout: 45_000) do
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

  defp loop_pid(sprite, exec_fn) do
    case exec_fn.(
           sprite,
           """
           if [ -s #{shell_quote(@sprite_loop_pid_path)} ]; then
             pid=$(cat #{shell_quote(@sprite_loop_pid_path)})
             if kill -0 "$pid" 2>/dev/null; then
               printf '%s' "$pid"
             fi
           fi
           """,
           timeout: 15_000
         ) do
      {:ok, output} ->
        output
        |> String.trim()
        |> case do
          "" ->
            nil

          pid ->
            case Integer.parse(pid) do
              {value, ""} -> value
              _ -> nil
            end
        end

      _ ->
        nil
    end
  end

  defp lifecycle_status(true, true), do: "draining"
  defp lifecycle_status(true, false), do: "paused"
  defp lifecycle_status(false, true), do: "running"
  defp lifecycle_status(false, false), do: "idle"

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

  defp codex_auth_ready?(_sprite, harness, _exec_fn) when harness not in [nil, "", "codex"],
    do: true

  defp codex_auth_ready?(sprite, harness, exec_fn) when harness in [nil, "", "codex"] do
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

  defp detached_agent_command(harness, cmd_parts, workspace, prompt_path, persona_role) do
    cmd_str = Enum.join(cmd_parts, " ")
    runtime_env_path = Path.join(workspace, @runtime_env_file)
    log_path = Path.join(workspace, @log_file)
    command_suffix = harness_command(harness, cmd_str, workspace, prompt_path, persona_role)

    agent_cmd =
      "cd #{shell_quote(workspace)} && : > #{shell_quote(log_path)} && if [ -f #{shell_quote(runtime_env_path)} ]; then set -a; . #{shell_quote(runtime_env_path)}; set +a; fi && set -o pipefail && #{command_suffix} 2>&1 | tee -a #{shell_quote(log_path)}"

    """
    set -e
    mkdir -p #{shell_quote(@sprite_runtime_dir)}
    exec 9>#{shell_quote(@sprite_loop_lock_path)}
    if ! flock -n 9; then
      printf '%s' #{shell_quote(@start_loop_busy_marker)}
      exit 0
    fi
    if [ -e #{shell_quote(@sprite_pause_path)} ]; then
      printf '%s' #{shell_quote(@start_loop_paused_marker)}
      exit 0
    fi
    if [ -s #{shell_quote(@sprite_loop_pid_path)} ]; then
      pid=$(cat #{shell_quote(@sprite_loop_pid_path)})
      if kill -0 "$pid" 2>/dev/null; then
        printf '%s' #{shell_quote(@start_loop_busy_marker)}
        exit 0
      fi
    fi
    if #{detect_agents_cmd()}; then
      printf '%s' #{shell_quote(@start_loop_busy_marker)}
      exit 0
    fi
    rm -f #{shell_quote(@sprite_loop_pid_path)}
    nohup bash -lc #{shell_quote("""
    echo $$ > #{@sprite_loop_pid_path}
    trap 'rm -f #{@sprite_loop_pid_path}' EXIT
    #{agent_cmd}
    """)} >/dev/null 2>&1 </dev/null &
    printf '%s%s\\n' #{shell_quote(@start_loop_started_prefix)} "$!"
    """
  end

  defp parse_start_loop_output(output) do
    trimmed = String.trim(output)

    cond do
      trimmed == @start_loop_paused_marker ->
        {:error, "sprite is paused", 1}

      trimmed == @start_loop_busy_marker ->
        {:error, "sprite already has an active loop", 1}

      String.starts_with?(output, @start_loop_started_prefix) ->
        {:ok, String.replace_prefix(output, @start_loop_started_prefix, "")}

      true ->
        {:error, "detached loop launch returned unexpected output: #{trimmed}", 1}
    end
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

  defp maybe_sync_codex_auth(_sprite, harness, _exec_fn) when harness not in [nil, "", "codex"],
    do: :ok

  defp maybe_sync_codex_auth(sprite, _harness, exec_fn) do
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

  defp upload_runtime_env(sprite, repo, exec_fn) do
    with_temp_file("sprite-runtime-env", runtime_env_contents(repo), fn runtime_env_file ->
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
        case exec_fn.(sprite, workspace_discovery_script(), timeout: 15_000) do
          {:ok, output} ->
            workspace = String.trim(output)

            if workspace in ["", "."] do
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
    with :ok <- Workspace.validate_repo(repo) do
      :ok
    else
      _ -> {:error, "invalid repo format: #{inspect(repo)}"}
    end
  end

  defp validate_repo(repo), do: {:error, "invalid repo format: #{inspect(repo)}"}

  defp repo_setup_script(repo_dir, repo, true) do
    """
    mkdir -p #{shell_quote(Path.dirname(repo_dir))} &&
      rm -rf #{shell_quote(repo_dir)} &&
      git clone #{shell_quote(repo_clone_url(repo))} #{shell_quote(repo_dir)}
    """
    |> String.trim()
  end

  defp repo_setup_script(repo_dir, repo, false) do
    """
    if [ -d #{shell_quote(Path.join(repo_dir, ".git"))} ]; then
      cd #{shell_quote(repo_dir)} &&
        git remote set-url origin #{shell_quote(repo_clone_url(repo))} &&
        (git checkout master 2>/dev/null || git checkout main 2>/dev/null) &&
        git pull --ff-only
    else
      mkdir -p #{shell_quote(Path.dirname(repo_dir))} &&
        rm -rf #{shell_quote(repo_dir)} &&
        git clone #{shell_quote(repo_clone_url(repo))} #{shell_quote(repo_dir)}
    fi
    """
    |> String.trim()
  end

  defp repo_clone_url(repo), do: "https://github.com/#{repo}.git"

  defp sprite_repo_workspace(repo) do
    Path.join(@sprite_workspace_root, repo)
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

    """
  end

  defp shell_quote(value), do: Shell.quote_arg(to_string(value))

  defp runtime_env_contents(repo) do
    body =
      (Config.dispatch_env() ++
         repo_env(repo))
      |> Enum.map_join("\n", fn {key, value} -> "export #{key}=#{shell_quote(value)}" end)

    if body == "", do: "# managed by Conductor\n", else: body <> "\n"
  end

  defp upload_dispatch_files(exec_fn, sprite, prompt_path, prompt, runtime_env_path, repo) do
    with_temp_file("sprite-prompt", prompt, fn prompt_file ->
      with_temp_file("sprite-env", runtime_env_contents(repo), fn env_file ->
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

  defp repo_env(nil), do: []
  defp repo_env(""), do: []
  defp repo_env(repo), do: [{"REPO", repo}]

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
