defmodule Conductor.Config do
  @moduledoc "Runtime configuration from environment and application config."
  require Logger

  @default_trusted_review_authors [
    "github-actions",
    "coderabbitai",
    "chatgpt-codex-connector",
    "chatgpt-codex-connector[bot]"
  ]

  @type worker_config :: %{
          name: binary(),
          capability_tags: [binary()]
        }

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

  @doc """
  Return the trusted review authors allowlist used to classify low-priority
  external threads.

  When unset, the built-in trusted bot list is used. Set
  `:trusted_review_authors` to `[]` to disable trusted external auto-labeling.
  Invalid non-empty values log a warning and fall back to defaults.
  """
  @spec trusted_review_authors() :: [binary()]
  def trusted_review_authors do
    raw_authors =
      Application.get_env(:conductor, :trusted_review_authors, @default_trusted_review_authors)

    case normalize_trusted_review_authors(raw_authors) do
      {:ok, authors} ->
        authors

      {:error, reason} ->
        Logger.warning(
          "invalid :trusted_review_authors config (#{reason}); falling back to defaults"
        )

        @default_trusted_review_authors
    end
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

  @spec dispatch_env() :: [{binary(), binary()}]
  def dispatch_env do
    # Render only the runtime API keys the harness still needs into the
    # sprite-side env file. GitHub auth is persisted separately during setup.
    []
    |> maybe_env("OPENAI_API_KEY")
    |> maybe_env_as("OPENAI_API_KEY", "CODEX_API_KEY")
    |> maybe_env("EXA_API_KEY")
    |> Enum.reverse()
  end

  defp maybe_env(acc, key) do
    case System.get_env(key) do
      nil -> acc
      "" -> acc
      val -> [{key, val} | acc]
    end
  end

  # Inject a local env var under a different name on the sprite.
  # Used for OPENAI_API_KEY → CODEX_API_KEY (Codex CLI reads CODEX_API_KEY).
  defp maybe_env_as(acc, source_key, target_key) do
    case System.get_env(source_key) do
      nil -> acc
      "" -> acc
      val -> [{target_key, val} | acc]
    end
  end

  defp normalize_trusted_review_authors(raw_authors) when is_binary(raw_authors) do
    case normalize_trusted_review_author_list([raw_authors]) do
      [] -> {:error, "empty binary value"}
      authors -> {:ok, authors}
    end
  end

  defp normalize_trusted_review_authors(raw_authors) when is_list(raw_authors) do
    authors = normalize_trusted_review_author_list(raw_authors)

    cond do
      raw_authors == [] -> {:ok, []}
      authors != [] -> {:ok, authors}
      true -> {:error, "no valid author names"}
    end
  end

  defp normalize_trusted_review_authors(_raw_authors), do: {:error, "expected a binary or list"}

  defp normalize_trusted_review_author_list(raw_authors) do
    raw_authors
    |> Enum.filter(&is_binary/1)
    |> Enum.map(&String.trim/1)
    |> Enum.reject(&(&1 == ""))
    |> Enum.map(&String.downcase/1)
    |> Enum.uniq()
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

  defp find_executable(name) do
    System.find_executable(name)
  end
end
