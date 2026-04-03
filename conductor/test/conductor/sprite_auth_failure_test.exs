defmodule Conductor.SpriteAuthFailureTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite

  describe "detect_auth_failure/2" do
    test "returns :ok when log has no auth errors" do
      exec_fn = fn _sprite, _command, _opts ->
        {:ok, "normal operation\nall good\nno errors here\n"}
      end

      assert :ok = Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
    end

    test "detects refresh_token_reused error" do
      exec_fn = fn _sprite, _command, _opts ->
        {:ok, "some output\nError: refresh_token_reused - token already consumed\nmore output\n"}
      end

      assert {:auth_failure, reason} = Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
      assert reason =~ "refresh_token_reused"
    end

    test "detects Failed to refresh token error" do
      exec_fn = fn _sprite, _command, _opts ->
        {:ok, "starting up\nFailed to refresh token: invalid grant\nshutting down\n"}
      end

      assert {:auth_failure, reason} = Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
      assert reason =~ "Failed to refresh token"
    end

    test "detects 401 auth error" do
      exec_fn = fn _sprite, _command, _opts ->
        {:ok, "request failed\nHTTP 401 Unauthorized\naborting\n"}
      end

      assert {:auth_failure, reason} = Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
      assert reason =~ "401"
    end

    test "detects auth_error pattern" do
      exec_fn = fn _sprite, _command, _opts ->
        {:ok, "session died\nauth_error: token expired\nbye\n"}
      end

      assert {:auth_failure, reason} = Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
      assert reason =~ "auth_error"
    end

    test "returns :ok when exec fails (no log available)" do
      exec_fn = fn _sprite, _command, _opts ->
        {:error, "no such file", 1}
      end

      assert :ok = Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
    end

    test "greps the last 50 lines of ralph.log" do
      test_pid = self()

      exec_fn = fn _sprite, command, _opts ->
        send(test_pid, {:exec_command, command})
        {:ok, "no errors\n"}
      end

      Sprite.detect_auth_failure("bb-weaver", exec_fn: exec_fn)
      assert_received {:exec_command, command}
      assert command =~ "tail -n 50"
      assert command =~ "ralph.log"
    end
  end
end
