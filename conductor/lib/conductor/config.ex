defmodule Conductor.Config do
  @moduledoc "Runtime configuration from environment and application config."

  @spec github_token!() :: binary()
  def github_token!, do: System.fetch_env!("GITHUB_TOKEN")

  @spec sprites_org!() :: binary()
  def sprites_org! do
    System.get_env("SPRITES_ORG") || System.fetch_env!("FLY_ORG")
  end

  @spec bb_path() :: binary()
  def bb_path do
    System.get_env("BB_PATH") || resolve_bb_path()
  end

  defp resolve_bb_path do
    # Look for bb in parent directory (conductor lives inside the bitterblossom repo)
    candidates = [
      Path.expand("../bin/bb"),
      Path.expand("./bin/bb"),
      "bb"
    ]

    Enum.find(candidates, "bb", &File.exists?/1)
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
    # ANTHROPIC_API_KEY is intentionally empty — sprites use OPENROUTER_API_KEY
    # (set during bb setup). Clearing it prevents accidental direct Anthropic billing.
    [
      {"GITHUB_TOKEN", github_token!()},
      {"ANTHROPIC_API_KEY", ""}
    ]
  end

  @spec check_env!() :: :ok
  def check_env! do
    checks = [
      {"GITHUB_TOKEN", fn -> System.get_env("GITHUB_TOKEN") end},
      {"SPRITE_TOKEN or FLY_API_TOKEN",
       fn -> System.get_env("SPRITE_TOKEN") || System.get_env("FLY_API_TOKEN") end},
      {"bb", fn -> find_executable("bb") || File.exists?(bb_path()) end},
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
