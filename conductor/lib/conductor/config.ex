defmodule Conductor.Config do
  @moduledoc "Runtime configuration from environment and application config."

  require Logger

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

  @spec session_timeout_minutes() :: pos_integer() | :infinity
  def session_timeout_minutes do
    Application.get_env(:conductor, :session_timeout_minutes, 60)
  end

  @spec spellbook_repo() :: binary()
  def spellbook_repo do
    Application.get_env(:conductor, :spellbook_repo, "phrazzld/spellbook")
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

  @spec codex_auth_file() :: binary() | nil
  def codex_auth_file do
    case codex_home() do
      nil ->
        nil

      home ->
        home
        |> Path.join("auth.json")
        |> Path.expand()
    end
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

  @spec check_env!(keyword()) :: :ok
  def check_env!(opts \\ []) do
    checks =
      [
        {"GITHUB_TOKEN", fn -> System.get_env("GITHUB_TOKEN") end},
        {"SPRITE_TOKEN, FLY_API_TOKEN, or sprite CLI auth", fn -> sprite_auth_available?() end}
      ] ++
        maybe_codex_auth_check(opts) ++
        [
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
  Returns a truthy value if sprite CLI auth is available.

  Checks SPRITE_TOKEN env, then verifies the sprite CLI can actually
  list sprites for the configured org. FLY_API_TOKEN is not checked —
  the sprite CLI ignores it.
  """
  @spec sprite_auth_available?() :: binary() | false
  def sprite_auth_available? do
    System.get_env("SPRITE_TOKEN") ||
      sprite_cli_auth_live?() ||
      false
  end

  defp sprite_cli_auth_live? do
    org = System.get_env("SPRITES_ORG") || System.get_env("FLY_ORG")

    case org do
      nil ->
        Conductor.SpriteCLIAuth.authenticated?() && "sprite-cli"

      org ->
        case System.cmd("sprite", ["ls", "-o", org], stderr_to_stdout: true) do
          {_, 0} -> "sprite-cli"
          _ -> false
        end
    end
  end

  defp chatgpt_auth_file do
    case codex_auth_file() do
      nil ->
        {:error, :missing}

      path ->
        with true <- File.regular?(path),
             {:ok, body} <- File.read(path),
             {:ok, auth} <- Jason.decode(body),
             :ok <- validate_chatgpt_auth(path, auth) do
          {:ok, path}
        else
          false ->
            {:error, :missing}

          {:error, _reason} = error ->
            error

          {:invalid, details} ->
            Logger.debug("chatgpt_auth_file invalid Codex auth cache at #{path}: #{details}")
            {:error, :invalid}
        end
    end
  end

  defp validate_chatgpt_auth(_path, auth) when is_map(auth) do
    auth_mode = auth["auth_mode"]
    refresh_token = auth_refresh_token(auth)

    if auth_mode == "chatgpt" and is_binary(refresh_token) and refresh_token != "" do
      :ok
    else
      {:invalid,
       "auth_mode=#{inspect(auth_mode)} refresh_token=#{inspect(refresh_token)} auth=#{inspect(auth)}"}
    end
  end

  defp auth_refresh_token(auth) when is_map(auth) do
    auth["refresh_token"] || get_in(auth, ["tokens", "refresh_token"])
  end

  defp maybe_codex_auth_check(opts) do
    if Keyword.get(opts, :require_codex_auth, true) do
      [{"Codex ChatGPT auth cache or OPENAI_API_KEY", fn -> codex_auth_available?() end}]
    else
      []
    end
  end

  defp codex_home do
    nonempty_env("CODEX_HOME") ||
      case System.user_home() do
        nil -> nil
        home -> Path.join(home, ".codex")
      end
  end

  defp find_executable(name) do
    System.find_executable(name)
  end
end
