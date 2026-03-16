defmodule Conductor.Config do
  @moduledoc "Runtime configuration from environment and application config."

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

  @spec ci_timeout() :: pos_integer()
  def ci_timeout do
    Application.get_env(:conductor, :ci_timeout_minutes, 15)
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

  @spec replay_delay_ms() :: pos_integer()
  def replay_delay_ms do
    Application.get_env(:conductor, :replay_delay_seconds, 120) * 1_000
  end

  @spec prompt_template() :: binary()
  def prompt_template do
    System.get_env("CONDUCTOR_PROMPT_TEMPLATE") ||
      Path.expand("../scripts/builder-prompt-template.md")
  end

  @spec dispatch_env() :: [{binary(), binary()}]
  def dispatch_env do
    # Pass only the runtime API keys the harness still needs. GitHub auth is
    # persisted on the sprite during setup and should not rely on env injection.
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
      {"SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth",
       fn ->
         System.get_env("SPRITE_TOKEN") ||
           System.get_env("FLY_API_TOKEN") ||
           (Conductor.SpriteCLIAuth.authenticated?() && "sprite-cli")
       end},
      {"gh", fn -> find_executable("gh") end},
      {"sprite", fn -> find_executable("sprite") end}
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

  defp find_executable(name) do
    System.find_executable(name)
  end
end
