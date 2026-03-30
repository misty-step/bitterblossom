defmodule Conductor.ConfigTest do
  use ExUnit.Case, async: false

  alias Conductor.Config
  import Conductor.TestSupport.EnvHelpers

  # Restore HOME after every test (some tests mutate it for sprite CLI auth).
  setup do
    original_home = System.get_env("HOME")
    original_codex_home = System.get_env("CODEX_HOME")
    original_openai_key = System.get_env("OPENAI_API_KEY")

    on_exit(fn ->
      restore_home(original_home)
      restore_env("CODEX_HOME", original_codex_home)
      restore_env("OPENAI_API_KEY", original_openai_key)
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

  describe "normalize_workers/1" do
    test "coalesces nil capability tags to an empty list" do
      assert [%{name: "sprite-1", capability_tags: []}] =
               Config.normalize_workers([
                 %{name: "sprite-1", capability_tags: nil}
               ])
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

  describe "github_token!/0" do
    test "returns token when set" do
      System.put_env("GITHUB_TOKEN", "ghp_test123")
      assert Config.github_token!() == "ghp_test123"
    after
      System.delete_env("GITHUB_TOKEN")
    end

    test "raises when missing" do
      System.delete_env("GITHUB_TOKEN")

      assert_raise System.EnvError, fn ->
        Config.github_token!()
      end
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

    test "returns token when FLY_API_TOKEN set" do
      System.delete_env("SPRITE_TOKEN")
      System.put_env("FLY_API_TOKEN", "fly_test")
      assert Config.sprite_auth_available?() == "fly_test"
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
    end

    test "returns sprite-cli when sprite CLI authenticated" do
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")

      home =
        make_sprite_cli_home(%{
          "current_selection" => %{"url" => "https://api.machines.dev", "org" => "personal"},
          "urls" => %{}
        })

      System.put_env("HOME", home)
      assert Config.sprite_auth_available?() == "sprite-cli"
    after
      System.delete_env("SPRITE_TOKEN")
      System.delete_env("FLY_API_TOKEN")
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
end
