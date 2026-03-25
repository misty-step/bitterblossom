defmodule Conductor.Config do
  @moduledoc "Runtime configuration from environment and application config."

  @type worker_config :: %{
          name: binary(),
          capability_tags: [binary()]
        }
  @type codex_auth_source :: {:chatgpt, binary()} | {:api_key, binary()} | :missing

  @spec github_token!() :: binary()
  def github_token!, do: System.fetch_env!("GITHUB_TOKEN")

  @spec sprites_org!() :: binary()
  def sprites_org! do
    System.get_env("SPRITES_ORG") ||
      System.get_env("FLY_ORG") ||
      sprite_cli_org!()
  end

  defp sprite_cli_org! do
    case Conductor.SpriteCLIAuth.current_org() do
      {:ok, org} -> org
      {:error, _} -> raise "no sprite org: set SPRITES_ORG, FLY_ORG, or log in via sprite CLI"
    end
  end

  @spec db_path() :: binary()
  def db_path do
    Application.get_env(:conductor, :db_path, ".bb/conductor.db")
  end

  @spec event_log_path() :: binary()
  def event_log_path do
    Application.get_env(:conductor, :event_log, ".bb/events.jsonl")
  end

  @spec builder_timeout() :: pos_integer()
  def builder_timeout do
    Application.get_env(:conductor, :builder_timeout_minutes, 25)
  end

  @spec builder_retry_max_attempts() :: pos_integer()
  def builder_retry_max_attempts do
    Application.get_env(:conductor, :builder_retry_max_attempts, 3)
  end

  @spec builder_retry_backoff_base_ms() :: pos_integer()
  def builder_retry_backoff_base_ms do
    Application.get_env(:conductor, :builder_retry_backoff_base_ms, 1_000)
  end

  @spec ci_timeout() :: pos_integer()
  def ci_timeout do
    Application.get_env(:conductor, :ci_timeout_minutes, 30)
  end

  @spec ci_status_log_interval() :: non_neg_integer()
  def ci_status_log_interval do
    Application.get_env(:conductor, :ci_status_log_interval_minutes, 5)
  end

  @spec repo_root() :: binary()
  def repo_root do
    case Application.get_env(:conductor, :repo_root) do
      nil -> detect_repo_root()
      "" -> detect_repo_root()
      root -> Path.expand(root)
    end
  end

  @spec pr_minimum_age() :: non_neg_integer()
  def pr_minimum_age do
    Application.get_env(:conductor, :pr_minimum_age_seconds, 300)
  end

  @spec poll_seconds() :: pos_integer()
  def poll_seconds do
    Application.get_env(:conductor, :poll_seconds, 60)
  end

  @spec max_concurrent_runs() :: pos_integer()
  def max_concurrent_runs do
    Application.get_env(:conductor, :max_concurrent_runs, 2)
  end

  @spec max_replays() :: pos_integer()
  def max_replays do
    Application.get_env(:conductor, :max_replays, 3)
  end

  @spec operator_hold_label() :: binary()
  def operator_hold_label do
    Application.get_env(:conductor, :operator_hold_label, "hold")
  end

  @spec operator_cancel_command() :: binary()
  def operator_cancel_command do
    Application.get_env(:conductor, :operator_cancel_command, "bb: cancel")
  end

  @spec fleet_probe_failure_threshold() :: pos_integer()
  def fleet_probe_failure_threshold do
    Application.get_env(:conductor, :fleet_probe_failure_threshold, 3)
  end

  @doc """
  Minutes of heartbeat silence before a run is considered stale and its lease expired.
  Defaults to builder_timeout + ci_timeout + 10 minutes of buffer.
  """
  @spec stale_run_threshold_minutes() :: pos_integer()
  def stale_run_threshold_minutes do
    Application.get_env(
      :conductor,
      :stale_run_threshold_minutes,
      builder_timeout() + ci_timeout() + 10
    )
  end

  @spec fixer_timeout() :: pos_integer()
  def fixer_timeout do
    Application.get_env(:conductor, :fixer_timeout_minutes, 15)
  end

  @spec polisher_timeout() :: pos_integer()
  def polisher_timeout do
    Application.get_env(:conductor, :polisher_timeout_minutes, 15)
  end

  @spec max_starts_per_tick() :: pos_integer()
  def max_starts_per_tick do
    Application.get_env(:conductor, :max_starts_per_tick, 1)
  end

  @spec issue_cooldown_cap_minutes() :: pos_integer()
  def issue_cooldown_cap_minutes do
    Application.get_env(:conductor, :issue_cooldown_cap_minutes, 120)
  end

  @spec fleet_health_check_interval_ms() :: pos_integer()
  def fleet_health_check_interval_ms do
    Application.get_env(:conductor, :fleet_health_check_interval_ms, 120_000)
  end

  @spec fleet_recovery_max_attempts() :: pos_integer()
  def fleet_recovery_max_attempts do
    Application.get_env(:conductor, :fleet_recovery_max_attempts, 3)
  end

  @spec fleet_recovery_backoff_base_ms() :: pos_integer()
  def fleet_recovery_backoff_base_ms do
    Application.get_env(:conductor, :fleet_recovery_backoff_base_ms, 1_000)
  end

  @spec fleet_recovery_backoff_cap_ms() :: pos_integer()
  def fleet_recovery_backoff_cap_ms do
    Application.get_env(:conductor, :fleet_recovery_backoff_cap_ms, 30_000)
  end

  @spec replay_delay_ms() :: pos_integer()
  def replay_delay_ms do
    Application.get_env(:conductor, :replay_delay_seconds, 120) * 1_000
  end

  @spec prompt_template() :: binary()
  def prompt_template do
    System.get_env("CONDUCTOR_PROMPT_TEMPLATE") ||
      Path.expand("../scripts/builder-prompt-template.md")
  end

  @spec persona_source_root!() :: binary()
  def persona_source_root! do
    path = Application.fetch_env!(:conductor, :persona_source_root)

    if File.dir?(path) do
      path
    else
      raise "persona source root missing: #{path}"
    end
  end

  @spec codex_auth_file() :: binary()
  def codex_auth_file do
    codex_home()
    |> Path.join("auth.json")
    |> Path.expand()
  end

  @spec codex_auth_source() :: codex_auth_source
  def codex_auth_source do
    case chatgpt_auth_file() do
      {:ok, path} ->
        {:chatgpt, path}

      {:error, _reason} ->
        case nonempty_env("OPENAI_API_KEY") do
          nil -> :missing
          api_key -> {:api_key, api_key}
        end
    end
  end

  @spec dispatch_env() :: [{binary(), binary()}]
  def dispatch_env do
    # Render only the runtime API keys the harness still needs into the
    # sprite-side env file. GitHub auth is persisted separately during setup.
    []
    |> maybe_codex_api_env()
    |> maybe_env("EXA_API_KEY")
    |> Enum.reverse()
  end

  defp maybe_env(acc, key) do
    case nonempty_env(key) do
      nil -> acc
      val -> [{key, val} | acc]
    end
  end

  defp maybe_codex_api_env(acc) do
    case codex_auth_source() do
      {:api_key, api_key} ->
        [{"CODEX_API_KEY", api_key}, {"OPENAI_API_KEY", api_key} | acc]

      _ ->
        acc
    end
  end

  defp nonempty_env(key) do
    case System.get_env(key) do
      nil -> nil
      "" -> nil
      val -> val
    end
  end

  @spec codex_auth_available?() :: binary() | false
  def codex_auth_available? do
    case codex_auth_source() do
      {:chatgpt, path} -> path
      {:api_key, _} -> "OPENAI_API_KEY"
      :missing -> false
    end
  end

  defp detect_repo_root do
    cwd = File.cwd!()

    case [cwd, Path.expand("..", cwd)] |> Enum.find(&repo_root_candidate?/1) do
      nil ->
        raise """
        unable to detect repository root from #{cwd}; expected WORKFLOW.md and CLAUDE.md
        or set :repo_root explicitly
        """

      root ->
        Path.expand(root)
    end
  end

  defp repo_root_candidate?(path) do
    File.exists?(Path.join(path, "WORKFLOW.md")) and
      File.exists?(Path.join(path, "CLAUDE.md"))
  end

  @spec normalize_workers([binary() | map()]) :: [worker_config()]
  def normalize_workers(workers) do
    Enum.map(workers, fn
      %{name: name} = worker ->
        %{
          name: name,
          capability_tags:
            (Map.get(worker, :capability_tags) || Map.get(worker, "capability_tags") || [])
            |> List.wrap()
        }

      name when is_binary(name) ->
        %{name: name, capability_tags: []}
    end)
  end

  @spec check_env!() :: :ok
  def check_env! do
    checks = [
      {"GITHUB_TOKEN", fn -> System.get_env("GITHUB_TOKEN") end},
      {"SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth", fn -> sprite_auth_available?() end},
      {"Codex ChatGPT auth cache or OPENAI_API_KEY", fn -> codex_auth_available?() end},
      {"gh", fn -> find_executable("gh") end},
      {"sprite", fn -> find_executable("sprite") end},
      {"persona source root",
       fn ->
         case Application.get_env(:conductor, :persona_source_root) do
           path when is_binary(path) -> File.dir?(path)
           _ -> false
         end
       end}
    ]

    results =
      Enum.map(checks, fn {name, check} ->
        case check.() do
          nil -> {:fail, name}
          false -> {:fail, name}
          "" -> {:fail, name}
          _ -> {:ok, name}
        end
      end)

    failures = for {:fail, name} <- results, do: name

    if failures == [] do
      Enum.each(results, fn {:ok, name} -> IO.puts("  ok  #{name}") end)
      IO.puts("all checks passed")
      :ok
    else
      Enum.each(results, fn
        {:ok, name} -> IO.puts("  ok  #{name}")
        {:fail, name} -> IO.puts("  FAIL  #{name}")
      end)

      raise "missing: #{Enum.join(failures, ", ")}"
    end
  end

  @doc """
  Returns a truthy value if any sprite auth method is available:
  SPRITE_TOKEN env, FLY_API_TOKEN env, or sprite CLI session.
  """
  @spec sprite_auth_available?() :: binary() | false
  def sprite_auth_available? do
    System.get_env("SPRITE_TOKEN") ||
      System.get_env("FLY_API_TOKEN") ||
      (Conductor.SpriteCLIAuth.authenticated?() && "sprite-cli") ||
      false
  end

  defp chatgpt_auth_file do
    path = codex_auth_file()

    with true <- File.regular?(path),
         {:ok, body} <- File.read(path),
         {:ok, auth} <- Jason.decode(body),
         "chatgpt" <- auth["auth_mode"],
         refresh when is_binary(refresh) and refresh != "" <- auth_refresh_token(auth) do
      {:ok, path}
    else
      false -> {:error, :missing}
      {:error, _reason} = error -> error
      _ -> {:error, :invalid}
    end
  end

  defp auth_refresh_token(auth) when is_map(auth) do
    auth["refresh_token"] || get_in(auth, ["tokens", "refresh_token"])
  end

  defp codex_home do
    System.get_env("CODEX_HOME") || Path.join(System.user_home!(), ".codex")
  end

  defp find_executable(name) do
    System.find_executable(name)
  end
end
