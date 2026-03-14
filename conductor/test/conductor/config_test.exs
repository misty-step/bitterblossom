defmodule Conductor.ConfigTest do
  use ExUnit.Case, async: false

  alias Conductor.Config

  describe "bb_path/0" do
    test "returns BB_PATH env var when set" do
      System.put_env("BB_PATH", "/custom/path/bb")
      assert Config.bb_path() == "/custom/path/bb"
    after
      System.delete_env("BB_PATH")
    end

    test "falls back to resolution when BB_PATH unset" do
      System.delete_env("BB_PATH")
      path = Config.bb_path()

      # Must return a string; either a resolved candidate or the fallback "bb"
      assert is_binary(path)
    end
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

  describe "builder_timeout/0" do
    test "returns default of 25" do
      assert Config.builder_timeout() == 25
    end

    test "returns configured value" do
      Application.put_env(:conductor, :builder_timeout_minutes, 40)
      assert Config.builder_timeout() == 40
    after
      Application.delete_env(:conductor, :builder_timeout_minutes)
    end
  end

  describe "ci_timeout/0" do
    test "returns default of 15" do
      assert Config.ci_timeout() == 15
    end

    test "returns configured value" do
      Application.put_env(:conductor, :ci_timeout_minutes, 30)
      assert Config.ci_timeout() == 30
    after
      Application.delete_env(:conductor, :ci_timeout_minutes)
    end
  end

  describe "pr_minimum_age/0" do
    test "returns default of 300" do
      assert Config.pr_minimum_age() == 300
    end

    test "returns configured value" do
      Application.put_env(:conductor, :pr_minimum_age_seconds, 600)
      assert Config.pr_minimum_age() == 600
    after
      Application.delete_env(:conductor, :pr_minimum_age_seconds)
    end
  end

  describe "poll_seconds/0" do
    test "returns default of 60" do
      assert Config.poll_seconds() == 60
    end

    test "returns configured value" do
      Application.put_env(:conductor, :poll_seconds, 120)
      assert Config.poll_seconds() == 120
    after
      Application.delete_env(:conductor, :poll_seconds)
    end
  end

  describe "max_concurrent_runs/0" do
    test "returns default of 2" do
      assert Config.max_concurrent_runs() == 2
    end

    test "returns configured value" do
      Application.put_env(:conductor, :max_concurrent_runs, 5)
      assert Config.max_concurrent_runs() == 5
    after
      Application.delete_env(:conductor, :max_concurrent_runs)
    end
  end

  describe "stale_run_threshold_seconds/0" do
    test "returns default of 300" do
      assert Config.stale_run_threshold_seconds() == 300
    end

    test "returns configured value" do
      Application.put_env(:conductor, :stale_run_threshold_seconds, 600)
      assert Config.stale_run_threshold_seconds() == 600
    after
      Application.delete_env(:conductor, :stale_run_threshold_seconds)
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

    test "raises when both missing" do
      System.delete_env("SPRITES_ORG")
      System.delete_env("FLY_ORG")

      assert_raise System.EnvError, fn ->
        Config.sprites_org!()
      end
    end
  end
end
