defmodule Conductor.ConfigDispatchEnvTest do
  use ExUnit.Case, async: false

  alias Conductor.Config
  import Conductor.TestSupport.EnvHelpers

  setup do
    original_env =
      for key <-
            ~w(CODEX_HOME OPENAI_API_KEY GITHUB_TOKEN EXA_API_KEY CANARY_ENDPOINT CANARY_API_KEY),
          into: %{} do
        {key, System.get_env(key)}
      end

    codex_home = fresh_codex_home()
    System.put_env("CODEX_HOME", codex_home)
    System.delete_env("OPENAI_API_KEY")
    System.delete_env("GITHUB_TOKEN")
    System.delete_env("EXA_API_KEY")
    System.delete_env("CANARY_ENDPOINT")
    System.delete_env("CANARY_API_KEY")

    on_exit(fn ->
      File.rm_rf(codex_home)
      Enum.each(original_env, fn {key, value} -> restore_env(key, value) end)
    end)

    :ok
  end

  describe "dispatch_env/0" do
    test "includes OPENAI_API_KEY even when ChatGPT auth cache is present" do
      write_auth_json(%{
        "auth_mode" => "chatgpt",
        "tokens" => %{"refresh_token" => "rt-test"}
      })

      System.put_env("OPENAI_API_KEY", "sk-test-123")

      env = Config.dispatch_env()

      assert {"OPENAI_API_KEY", "sk-test-123"} in env
      assert {"CODEX_API_KEY", "sk-test-123"} in env
    end

    test "includes OPENAI_API_KEY when API key fallback is active" do
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      env = Config.dispatch_env()

      assert {"OPENAI_API_KEY", "sk-test-123"} in env
    end

    test "omits OPENAI_API_KEY when auth cache and API key are both missing" do
      env = Config.dispatch_env()

      refute Enum.any?(env, fn {k, _} -> k == "OPENAI_API_KEY" end)
      refute Enum.any?(env, fn {k, _} -> k == "CODEX_API_KEY" end)
    end

    test "does not inject GITHUB_TOKEN even when API key is set" do
      System.put_env("GITHUB_TOKEN", "ghp_test")
      System.put_env("OPENAI_API_KEY", "sk-test")

      env = Config.dispatch_env()

      refute {"GITHUB_TOKEN", "ghp_test"} in env
      assert {"OPENAI_API_KEY", "sk-test"} in env
    end

    test "maps OPENAI_API_KEY to CODEX_API_KEY for Codex CLI" do
      System.put_env("OPENAI_API_KEY", "sk-test-codex")

      env = Config.dispatch_env()

      assert {"CODEX_API_KEY", "sk-test-codex"} in env
    end

    test "includes EXA_API_KEY when set" do
      System.put_env("EXA_API_KEY", "exa-test-key")

      env = Config.dispatch_env()

      assert {"EXA_API_KEY", "exa-test-key"} in env
    end

    test "includes Canary credentials when set" do
      System.put_env("CANARY_ENDPOINT", "https://canary-obs.fly.dev")
      System.put_env("CANARY_API_KEY", "canary-test-key")

      env = Config.dispatch_env()

      assert {"CANARY_ENDPOINT", "https://canary-obs.fly.dev"} in env
      assert {"CANARY_API_KEY", "canary-test-key"} in env
    end
  end
end
