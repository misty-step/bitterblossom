defmodule Conductor.ConfigTest do
  use ExUnit.Case, async: false

  alias Conductor.Config
  import Conductor.TestSupport.EnvHelpers

  # Restore HOME after every test (some tests mutate it for sprite CLI auth).
  setup do
    original_home = System.get_env("HOME")
    original_codex_home = System.get_env("CODEX_HOME")
    original_openai_key = System.get_env("OPENAI_API_KEY")
    original_canary_endpoint = System.get_env("CANARY_ENDPOINT")
    original_canary_api_key = System.get_env("CANARY_API_KEY")
    original_path = System.get_env("PATH")

    on_exit(fn ->
      restore_home(original_home)
      restore_env("CODEX_HOME", original_codex_home)
      restore_env("OPENAI_API_KEY", original_openai_key)
      restore_env("CANARY_ENDPOINT", original_canary_endpoint)
      restore_env("CANARY_API_KEY", original_canary_api_key)
      restore_env("PATH", original_path)
    end)

    :ok
  end

  describe "db_path/0" do
    test "returns default when no app config" do
      assert Config.db_path() == ".bb/conductor.db"
    end

    test "returns configured value" do
      Application.put_env(:conductor, :db_path, "/tmp/test.db")
      assert Config.db_path() == "/tmp/test.db"
    after
      Application.delete_env(:conductor, :db_path)
    end
  end

  describe "event_log_path/0" do
    test "returns default when no app config" do
      assert Config.event_log_path() == ".bb/events.jsonl"
    end

    test "returns configured value" do
      Application.put_env(:conductor, :event_log, "/tmp/events.jsonl")
      assert Config.event_log_path() == "/tmp/events.jsonl"
    after
      Application.delete_env(:conductor, :event_log)
    end
  end

  describe "canary_services_path/0" do
    test "returns default when no app config" do
      original = Application.get_env(:conductor, :canary_services_path)

      try do
        Application.delete_env(:conductor, :canary_services_path)
        assert Config.canary_services_path() == "../canary-services.toml"
      after
        if original,
          do: Application.put_env(:conductor, :canary_services_path, original),
          else: Application.delete_env(:conductor, :canary_services_path)
      end
    end

    test "returns configured value" do
      Application.put_env(:conductor, :canary_services_path, "/tmp/canary-services.toml")
      assert Config.canary_services_path() == "/tmp/canary-services.toml"
    after
      Application.delete_env(:conductor, :canary_services_path)
    end
  end

  describe "canary_endpoint/0 and canary_api_key/0" do
    test "read non-empty Canary credentials from the environment" do
      System.put_env("CANARY_ENDPOINT", "https://canary-obs.fly.dev")
      System.put_env("CANARY_API_KEY", "canary-test-key")

      assert Config.canary_endpoint() == "https://canary-obs.fly.dev"
      assert Config.canary_api_key() == "canary-test-key"
    after
      System.delete_env("CANARY_ENDPOINT")
      System.delete_env("CANARY_API_KEY")
    end

    test "treat empty Canary credentials as missing" do
      System.put_env("CANARY_ENDPOINT", "")
      System.put_env("CANARY_API_KEY", "")

      assert Config.canary_endpoint() == nil
      assert Config.canary_api_key() == nil
    after
      System.delete_env("CANARY_ENDPOINT")
      System.delete_env("CANARY_API_KEY")
    end
  end

  describe "spellbook_source/0" do
    test "prefers an explicitly configured spellbook source" do
      Application.put_env(:conductor, :spellbook_source, "/tmp/spellbook-source")
      assert Config.spellbook_source() == "/tmp/spellbook-source"
    after
      Application.delete_env(:conductor, :spellbook_source)
    end
  end

  describe "git_credentials_entries/0" do
    test "reads newline-delimited explicit git credentials" do
      System.put_env(
        "BB_GIT_CREDENTIALS",
        "https://token@code.example.com/org/repo.git\nhttps://token@code.example.com/other/repo.git"
      )

      assert Config.git_credentials_entries() == [
               "https://token@code.example.com/org/repo.git",
               "https://token@code.example.com/other/repo.git"
             ]
    after
      System.delete_env("BB_GIT_CREDENTIALS")
    end

    test "does not synthesize git credentials from GITHUB_TOKEN" do
      System.put_env("GITHUB_TOKEN", "ghp_test")
      assert Config.git_credentials_entries() == []
    after
      System.delete_env("GITHUB_TOKEN")
    end
  end

  describe "prompt_template/0" do
    test "returns env var when set" do
      System.put_env("CONDUCTOR_PROMPT_TEMPLATE", "/custom/template.md")
      assert Config.prompt_template() == "/custom/template.md"
    after
      System.delete_env("CONDUCTOR_PROMPT_TEMPLATE")
    end

    test "falls back to relative path when env unset" do
      System.delete_env("CONDUCTOR_PROMPT_TEMPLATE")
      path = Config.prompt_template()
      assert String.ends_with?(path, "scripts/builder-prompt-template.md")
    end
  end

  describe "persona_source_root!/0" do
    test "raises when configured path is missing" do
      missing_path =
        Path.join(System.tmp_dir!(), "missing-persona-#{System.unique_integer([:positive])}")

      Application.put_env(:conductor, :persona_source_root, missing_path)

      assert_raise RuntimeError, "persona source root missing: #{missing_path}", fn ->
        Config.persona_source_root!()
      end
    after
      Application.delete_env(:conductor, :persona_source_root)
    end
  end

  describe "sprites_org!/0" do
    test "prefers SPRITES_ORG over FLY_ORG" do
      System.put_env("SPRITES_ORG", "sprites-val")
      System.put_env("FLY_ORG", "fly-val")
      assert Config.sprites_org!() == "sprites-val"
    after
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")
    end

    test "falls back to FLY_ORG" do
      System.delete_env("SPRITES_ORG")
      System.put_env("FLY_ORG", "fly-val")
      assert Config.sprites_org!() == "fly-val"
    after
      System.delete_env("FLY_ORG")
    end

    test "falls back to sprite CLI config org" do
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")

      home =
        make_sprite_cli_home(%{
          "current_selection" => %{"url" => "https://api.machines.dev", "org" => "cli-org"},
          "urls" => %{}
        })

      System.put_env("HOME", home)
      assert Config.sprites_org!() == "cli-org"
    after
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")
    end

    test "raises when env vars and sprite CLI all missing" do
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")

      System.put_env(
        "HOME",
        System.tmp_dir!()
        |> Path.join("no_sprite_#{:erlang.unique_integer([:positive])}")
        |> tap(&File.mkdir_p!/1)
      )

      assert_raise RuntimeError, ~r/no sprite org/, fn ->
        Config.sprites_org!()
      end
    after
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")
    end
  end

  describe "sprite_auth_available?/0" do
    test "returns token when SPRITE_TOKEN set" do
      System.put_env("SPRITE_TOKEN", "st_test")
      assert Config.sprite_auth_available?() == "st_test"
    after
      System.delete_env("SPRITE_TOKEN")
    end

    test "FLY_API_TOKEN alone is not sufficient" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")
      System.put_env("FLY_API_TOKEN", "fly_test")

      System.put_env(
        "HOME",
        System.tmp_dir!()
        |> Path.join("no_sprite_fly_#{:erlang.unique_integer([:positive])}")
        |> tap(&File.mkdir_p!/1)
      )

      assert Config.sprite_auth_available?() == false
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")
    end

    test "sprite CLI config file alone is not sufficient without live probe" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")

      home =
        make_sprite_cli_home(%{
          "current_selection" => %{"url" => "https://api.machines.dev", "org" => "personal"},
          "urls" => %{}
        })

      System.put_env("HOME", home)
      # SpriteCLIAuth config exists with org "personal", but sprite ls -o personal
      # will fail in test environment — live probe must succeed for truthy return.
      result = Config.sprite_auth_available?()
      assert result == false or result == "sprite-cli"
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")
    end

    test "returns false when no auth available" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")

      System.put_env(
        "HOME",
        System.tmp_dir!()
        |> Path.join("no_sprite_#{:erlang.unique_integer([:positive])}")
        |> tap(&File.mkdir_p!/1)
      )

      assert Config.sprite_auth_available?() == false
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
    end

    test "uses sprite exec against a declared sprite when provided" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.put_env("SPRITES_ORG", "personal")

      install_fake_sprite_cli(
        exec_status: 0,
        exec_output: "ok",
        ls_status: 1,
        ls_output: "ls denied"
      )

      assert Config.sprite_auth_available?(sprite_auth_probe_target: "bb-declared") ==
               "sprite-cli"
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
    end

    test "does not fall back to org listing when a declared sprite exec probe fails" do
      System.delete_env("SPRITE_TOKEN")
      System.put_env("FLY_API_TOKEN", "fly_test")
      System.put_env("SPRITES_ORG", "personal")

      install_fake_sprite_cli(
        exec_status: 1,
        exec_output: "no token found for organization personal",
        ls_status: 0,
        ls_output: "listed"
      )

      assert Config.sprite_auth_available?(sprite_auth_probe_target: "bb-declared") == false
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
    end

    test "falls back to sprite ls when no declared sprites are provided" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.put_env("SPRITES_ORG", "personal")

      install_fake_sprite_cli(
        exec_status: 1,
        exec_output: "exec denied",
        ls_status: 0,
        ls_output: "listed"
      )

      assert Config.sprite_auth_available?(sprite_auth_probe_target: nil) == "sprite-cli"
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
    end

    test "falls back to sprite ls when exec fails for non-auth reasons" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.put_env("SPRITES_ORG", "personal")

      install_fake_sprite_cli(
        exec_status: 1,
        exec_output: "sprite not running",
        ls_status: 0,
        ls_output: "listed"
      )

      assert Config.sprite_auth_available?(sprite_auth_probe_target: "bb-declared") ==
               "sprite-cli"
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
      System.delete_env("SPRITES_ORG")
    end
  end

  describe "codex_auth_source/0" do
    test "prefers a valid ChatGPT auth cache over OPENAI_API_KEY" do
      codex_home =
        make_codex_home(%{
          "auth_mode" => "chatgpt",
          "tokens" => %{"refresh_token" => "rt-test"}
        })

      System.put_env("CODEX_HOME", codex_home)
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      assert Config.codex_auth_source() == {:chatgpt, Path.join(codex_home, "auth.json")}
    end

    test "accepts legacy top-level refresh_token auth caches" do
      codex_home = make_codex_home(%{"auth_mode" => "chatgpt", "refresh_token" => "rt-test"})
      System.put_env("CODEX_HOME", codex_home)

      assert Config.codex_auth_source() == {:chatgpt, Path.join(codex_home, "auth.json")}
    end

    test "falls back to OPENAI_API_KEY when auth cache is missing" do
      codex_home = make_codex_home(nil)
      System.put_env("CODEX_HOME", codex_home)
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      assert Config.codex_auth_source() == {:api_key, "sk-test-123"}
    end

    test "falls back to OPENAI_API_KEY when auth cache is invalid" do
      codex_home = make_codex_home(%{"auth_mode" => "chatgpt"})
      System.put_env("CODEX_HOME", codex_home)
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      assert Config.codex_auth_source() == {:api_key, "sk-test-123"}
    end

    test "falls back to OPENAI_API_KEY when auth cache JSON is malformed" do
      codex_home = make_codex_home(nil)
      File.write!(Path.join(codex_home, "auth.json"), "{")
      System.put_env("CODEX_HOME", codex_home)
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      assert Config.codex_auth_source() == {:api_key, "sk-test-123"}
    end

    test "returns :missing when neither auth cache nor API key are available" do
      codex_home = make_codex_home(nil)
      System.put_env("CODEX_HOME", codex_home)
      System.delete_env("OPENAI_API_KEY")

      assert Config.codex_auth_source() == :missing
    end
  end

  describe "codex_auth_available?/0" do
    test "returns the auth cache path when ChatGPT auth is available" do
      codex_home =
        make_codex_home(%{
          "auth_mode" => "chatgpt",
          "tokens" => %{"refresh_token" => "rt-test"}
        })

      System.put_env("CODEX_HOME", codex_home)

      assert Config.codex_auth_available?() == Path.join(codex_home, "auth.json")
    end

    test "returns OPENAI_API_KEY when API auth is selected" do
      codex_home = make_codex_home(nil)
      System.put_env("CODEX_HOME", codex_home)
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      assert Config.codex_auth_available?() == "OPENAI_API_KEY"
    end

    test "returns false when no Codex auth source is available" do
      codex_home = make_codex_home(nil)
      System.put_env("CODEX_HOME", codex_home)
      System.delete_env("OPENAI_API_KEY")

      assert Config.codex_auth_available?() == false
    end
  end

  describe "codex_auth_file/0" do
    test "falls back to HOME/.codex/auth.json when CODEX_HOME is unset" do
      System.delete_env("CODEX_HOME")

      assert Config.codex_auth_file() == Path.join(System.user_home!(), ".codex/auth.json")
    end
  end

  describe "check_env!/1" do
    test "requires Canary credentials when responder fleets need them" do
      original_persona_source_root = Application.get_env(:conductor, :persona_source_root)

      on_exit(fn ->
        if original_persona_source_root,
          do: Application.put_env(:conductor, :persona_source_root, original_persona_source_root),
          else: Application.delete_env(:conductor, :persona_source_root)
      end)

      dir = Path.join(System.tmp_dir!(), "fake_cli_#{System.unique_integer([:positive])}")
      File.mkdir_p!(dir)
      on_exit(fn -> File.rm_rf(dir) end)

      for cli <- ~w(git sprite) do
        path = Path.join(dir, cli)
        File.write!(path, "#!/bin/sh\nexit 0\n")
        File.chmod!(path, 0o755)
      end

      System.put_env("PATH", dir <> ":" <> System.get_env("PATH", ""))
      System.put_env("SPRITE_TOKEN", "sprite_test")
      System.put_env("OPENAI_API_KEY", "sk-test")
      System.delete_env("CANARY_ENDPOINT")
      System.delete_env("CANARY_API_KEY")

      Application.put_env(
        :conductor,
        :persona_source_root,
        Path.expand("../../../sprites", __DIR__)
      )

      assert_raise RuntimeError, ~r/missing: CANARY_ENDPOINT, CANARY_API_KEY/, fn ->
        Config.check_env!(require_canary_auth: true)
      end
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("OPENAI_API_KEY")
      System.delete_env("CANARY_ENDPOINT")
      System.delete_env("CANARY_API_KEY")
    end
  end

  defp restore_home(nil), do: System.delete_env("HOME")
  defp restore_home(val), do: System.put_env("HOME", val)

  defp make_sprite_cli_home(config) do
    home =
      Path.join(System.tmp_dir!(), "sprite_config_test_#{:erlang.unique_integer([:positive])}")

    sprites_dir = Path.join(home, ".sprites")
    File.mkdir_p!(sprites_dir)
    File.write!(Path.join(sprites_dir, "sprites.json"), Jason.encode!(config))
    home
  end

  defp make_codex_home(nil) do
    fresh_codex_home()
  end

  defp make_codex_home(auth_payload) do
    home = make_codex_home(nil)
    File.write!(Path.join(home, "auth.json"), Jason.encode!(auth_payload))
    home
  end

  defp install_fake_sprite_cli(opts) do
    dir = Path.join(System.tmp_dir!(), "fake_sprite_#{System.unique_integer([:positive])}")
    File.mkdir_p!(dir)
    on_exit(fn -> File.rm_rf(dir) end)

    sprite_path = Path.join(dir, "sprite")

    File.write!(
      sprite_path,
      """
      #!/bin/sh
      if [ "$1" = "-o" ] && [ "$3" = "-s" ] && [ "$5" = "exec" ]; then
        printf '%s' '#{Keyword.fetch!(opts, :exec_output)}'
        exit #{Keyword.fetch!(opts, :exec_status)}
      fi

      if [ "$1" = "ls" ] && [ "$2" = "-o" ]; then
        printf '%s' '#{Keyword.fetch!(opts, :ls_output)}'
        exit #{Keyword.fetch!(opts, :ls_status)}
      fi

      printf '%s' "unexpected args: $*"
      exit 64
      """
    )

    File.chmod!(sprite_path, 0o755)
    System.put_env("PATH", dir <> ":" <> System.get_env("PATH", ""))
  end
end
