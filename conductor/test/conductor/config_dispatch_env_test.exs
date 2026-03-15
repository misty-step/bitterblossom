defmodule Conductor.ConfigDispatchEnvTest do
  use ExUnit.Case, async: false

  alias Conductor.Config

  describe "dispatch_env/0" do
    test "includes OPENAI_API_KEY when set" do
      prev = System.get_env("OPENAI_API_KEY")
      System.put_env("OPENAI_API_KEY", "sk-test-123")

      try do
        env = Config.dispatch_env()
        assert {"OPENAI_API_KEY", "sk-test-123"} in env
      after
        if prev,
          do: System.put_env("OPENAI_API_KEY", prev),
          else: System.delete_env("OPENAI_API_KEY")
      end
    end

    test "omits OPENAI_API_KEY when unset" do
      prev = System.get_env("OPENAI_API_KEY")
      System.delete_env("OPENAI_API_KEY")

      try do
        env = Config.dispatch_env()
        refute Enum.any?(env, fn {k, _} -> k == "OPENAI_API_KEY" end)
      after
        if prev, do: System.put_env("OPENAI_API_KEY", prev)
      end
    end

    test "does not inject GITHUB_TOKEN even when OPENAI_API_KEY is set" do
      prev_gh = System.get_env("GITHUB_TOKEN")
      prev_oai = System.get_env("OPENAI_API_KEY")
      System.put_env("GITHUB_TOKEN", "ghp_test")
      System.put_env("OPENAI_API_KEY", "sk-test")

      try do
        env = Config.dispatch_env()
        refute {"GITHUB_TOKEN", "ghp_test"} in env
        assert {"OPENAI_API_KEY", "sk-test"} in env
      after
        if prev_gh,
          do: System.put_env("GITHUB_TOKEN", prev_gh),
          else: System.delete_env("GITHUB_TOKEN")

        if prev_oai,
          do: System.put_env("OPENAI_API_KEY", prev_oai),
          else: System.delete_env("OPENAI_API_KEY")
      end
    end

    test "maps OPENAI_API_KEY to CODEX_API_KEY for Codex CLI" do
      prev = System.get_env("OPENAI_API_KEY")
      System.put_env("OPENAI_API_KEY", "sk-test-codex")

      try do
        env = Config.dispatch_env()
        assert {"CODEX_API_KEY", "sk-test-codex"} in env
      after
        if prev,
          do: System.put_env("OPENAI_API_KEY", prev),
          else: System.delete_env("OPENAI_API_KEY")
      end
    end

    test "includes EXA_API_KEY when set" do
      prev = System.get_env("EXA_API_KEY")
      System.put_env("EXA_API_KEY", "exa-test-key")

      try do
        env = Config.dispatch_env()
        assert {"EXA_API_KEY", "exa-test-key"} in env
      after
        if prev,
          do: System.put_env("EXA_API_KEY", prev),
          else: System.delete_env("EXA_API_KEY")
      end
    end
  end
end
