defmodule Conductor.ConfigDispatchEnvTest do
  use ExUnit.Case, async: false

  alias Conductor.Config

  setup do
    original_env =
      for key <- ~w(CODEX_HOME OPENAI_API_KEY GITHUB_TOKEN EXA_API_KEY), into: %{} do
        {key, System.get_env(key)}
      end

    codex_home =
      Path.join(System.tmp_dir!(), "codex_home_#{System.unique_integer([:positive])}")

    File.mkdir_p!(codex_home)
    System.put_env("CODEX_HOME", codex_home)
    System.delete_env("OPENAI_API_KEY")
    System.delete_env("GITHUB_TOKEN")
    System.delete_env("EXA_API_KEY")

    on_exit(fn ->
      File.rm_rf(codex_home)
      Enum.each(original_env, fn {key, value} -> restore_env(key, value) end)
    end)

    :ok
  end

  describe "dispatch_env/0" do
    test "omits OPENAI_API_KEY and CODEX_API_KEY when ChatGPT auth cache is present" do
      write_auth_json(%{"auth_mode" => "chatgpt", "refresh_token" => "rt-test"})
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      env = Config.dispatch_env()

      refute {"OPENAI_API_KEY", "sk-test-123"} in env
      refute {"CODEX_API_KEY", "sk-test-123"} in env
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

    test "does not inject GITHUB_TOKEN even when API key fallback is active" do
      System.put_env("GITHUB_TOKEN", "ghp_test")
      System.put_env("OPENAI_API_KEY", "sk-test")

      env = Config.dispatch_env()

      refute {"GITHUB_TOKEN", "ghp_test"} in env
      assert {"OPENAI_API_KEY", "sk-test"} in env
    end

    test "maps OPENAI_API_KEY to CODEX_API_KEY for Codex CLI in fallback mode" do
      System.put_env("OPENAI_API_KEY", "sk-test-codex")

      env = Config.dispatch_env()

      assert {"CODEX_API_KEY", "sk-test-codex"} in env
    end

    test "includes EXA_API_KEY when set" do
      System.put_env("EXA_API_KEY", "exa-test-key")

      env = Config.dispatch_env()

      assert {"EXA_API_KEY", "exa-test-key"} in env
    end
  end

  defp write_auth_json(payload) do
    path = Path.join(System.fetch_env!("CODEX_HOME"), "auth.json")
    File.write!(path, Jason.encode!(payload))
  end

  defp restore_env(key, nil), do: System.delete_env(key)
  defp restore_env(key, value), do: System.put_env(key, value)
end
