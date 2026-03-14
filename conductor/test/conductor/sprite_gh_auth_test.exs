defmodule Conductor.SpriteGHAuthTest do
  use ExUnit.Case, async: true

  @moduledoc """
  Tests for `Conductor.Sprite.setup_gh_auth/3` and `Conductor.Sprite.gh_auth_ok?/2`.

  Uses injected exec_fn — no real sprite required.
  """

  alias Conductor.Sprite

  # ---------------------------------------------------------------------------
  # Helpers
  # ---------------------------------------------------------------------------

  defp make_exec_fn(responses \\ []) do
    test_pid = self()

    fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      Enum.find_value(responses, {:ok, ""}, fn {pattern, result} ->
        if String.contains?(command, pattern), do: result
      end)
    end
  end

  # Override exec via process dictionary trick — we need to test setup_gh_auth
  # which calls exec/3 directly. We use a wrapper that patches the internal exec.
  # Instead, we test via the public exec_fn injection pattern used in dispatch tests:
  # setup_gh_auth/3 accepts an optional :exec_fn in opts for testability.

  # ---------------------------------------------------------------------------
  # setup_gh_auth/3
  # ---------------------------------------------------------------------------

  describe "setup_gh_auth/3" do
    test "calls gh auth login with base64-decoded token" do
      exec_fn = make_exec_fn()

      :ok = Sprite.setup_gh_auth("my-sprite", "test-token-123", exec_fn: exec_fn)

      assert_received {:exec_called, auth_cmd}
      # Token is base64-encoded in the command
      encoded = Base.encode64("test-token-123")
      assert String.contains?(auth_cmd, encoded)
      assert String.contains?(auth_cmd, "gh auth login --with-token")
    end

    test "configures git credential helper after auth login" do
      exec_fn = make_exec_fn()

      :ok = Sprite.setup_gh_auth("my-sprite", "test-token-123", exec_fn: exec_fn)

      # Drain auth call
      assert_received {:exec_called, _auth_cmd}

      assert_received {:exec_called, git_cmd}
      assert String.contains?(git_cmd, "gh auth git-credential")
      assert String.contains?(git_cmd, "credential.helper")
    end

    test "returns :ok on success" do
      exec_fn = make_exec_fn()
      result = Sprite.setup_gh_auth("sprite", "token", exec_fn: exec_fn)
      assert result == :ok
    end

    test "returns {:error, reason} when gh auth login fails" do
      exec_fn = make_exec_fn([{"gh auth login", {:error, "gh not found", 127}}])

      result = Sprite.setup_gh_auth("sprite", "token", exec_fn: exec_fn)
      assert {:error, _reason} = result
    end

    test "returns {:error, reason} when git config fails" do
      exec_fn =
        make_exec_fn([
          {"gh auth login", {:ok, ""}},
          {"credential.helper", {:error, "git not found", 127}}
        ])

      result = Sprite.setup_gh_auth("sprite", "token", exec_fn: exec_fn)
      assert {:error, _reason} = result
    end
  end

  # ---------------------------------------------------------------------------
  # gh_auth_ok?/2
  # ---------------------------------------------------------------------------

  describe "gh_auth_ok?/2" do
    test "returns true when gh auth status shows Logged in" do
      exec_fn =
        make_exec_fn([
          {"gh auth status", {:ok, "github.com\n  Logged in to github.com as bot (token)\n"}}
        ])

      assert Sprite.gh_auth_ok?("sprite", exec_fn: exec_fn)
    end

    test "returns false when gh auth status shows not logged in" do
      exec_fn =
        make_exec_fn([
          {"gh auth status", {:error, "You are not logged into any GitHub hosts.", 1}}
        ])

      refute Sprite.gh_auth_ok?("sprite", exec_fn: exec_fn)
    end

    test "returns false when gh auth status output lacks 'Logged in'" do
      exec_fn = make_exec_fn([{"gh auth status", {:ok, "something unexpected\n"}}])

      refute Sprite.gh_auth_ok?("sprite", exec_fn: exec_fn)
    end

    test "returns false when exec fails entirely" do
      exec_fn = make_exec_fn([{"gh auth status", {:error, "command not found", 127}}])

      refute Sprite.gh_auth_ok?("sprite", exec_fn: exec_fn)
    end
  end
end
