defmodule Conductor.SpriteDispatchTest do
  use ExUnit.Case, async: true

  @moduledoc """
  Verifies the `Conductor.Sprite.dispatch/4` sequence using an injected
  exec_fn — no real sprite required.

  Sequence under test:
  1. Kill stale agent processes
  2. Upload prompt (base64)
  3. Run agent via harness dispatch_command
  4. On crash, retry via harness continue_command
  """

  alias Conductor.Sprite

  # ---------------------------------------------------------------------------
  # Mock Harness
  # ---------------------------------------------------------------------------

  defmodule MockHarness do
    @behaviour Conductor.Harness

    def name, do: "mock"
    def dispatch_command(_opts), do: ["agent", "-p"]
    def continue_command(_opts), do: ["agent", "--continue", "-p"]
  end

  defmodule NoRetryHarness do
    @behaviour Conductor.Harness

    def name, do: "no_retry"
    def dispatch_command(_opts), do: ["agent", "-p"]
    def continue_command(_opts), do: nil
  end

  # ---------------------------------------------------------------------------
  # Helpers
  # ---------------------------------------------------------------------------

  # Returns an exec_fn that records all calls to the test process and returns
  # `responses` matched by substring. Falls back to `{:ok, ""}` if no match.
  defp make_exec_fn(responses \\ []) do
    test_pid = self()

    fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      Enum.find_value(responses, {:ok, ""}, fn {pattern, result} ->
        if String.contains?(command, pattern), do: result
      end)
    end
  end

  # ---------------------------------------------------------------------------
  # Tests
  # ---------------------------------------------------------------------------

  describe "dispatch/4 full sequence" do
    test "kills stale processes before dispatching" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "hello prompt", "org/repo",
        workspace: "/ws",
        harness: MockHarness,
        exec_fn: exec_fn,
        timeout: 1
      )

      assert_received {:exec_called, kill_cmd}
      assert String.contains?(kill_cmd, "pkill")
    end

    test "uploads prompt as base64 to workspace/PROMPT.md" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "my test prompt", "org/repo",
        workspace: "/my/ws",
        harness: MockHarness,
        exec_fn: exec_fn,
        timeout: 1
      )

      # Drain the kill call
      assert_received {:exec_called, _kill_cmd}

      # Base64 upload
      assert_received {:exec_called, upload_cmd}
      assert String.contains?(upload_cmd, "base64 -d")
      assert String.contains?(upload_cmd, "/my/ws/PROMPT.md")

      # Verify the prompt content is encoded in the command
      expected_b64 = Base.encode64("my test prompt")
      assert String.contains?(upload_cmd, expected_b64)
    end

    test "runs agent via harness dispatch_command" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        harness: MockHarness,
        exec_fn: exec_fn,
        timeout: 1
      )

      # Drain kill and upload
      assert_received {:exec_called, _}
      assert_received {:exec_called, _}

      assert_received {:exec_called, agent_cmd}
      assert String.contains?(agent_cmd, "agent -p")
      assert String.contains?(agent_cmd, "/ws")
      assert String.contains?(agent_cmd, "PROMPT.md")
    end

    test "includes LEFTHOOK=0 to suppress git hooks" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        harness: MockHarness,
        exec_fn: exec_fn,
        timeout: 1
      )

      assert_received {:exec_called, _}
      assert_received {:exec_called, _}
      assert_received {:exec_called, agent_cmd}
      assert String.contains?(agent_cmd, "LEFTHOOK=0")
    end

    test "injects GITHUB_TOKEN from env, shell-quoted" do
      prev = System.get_env("GITHUB_TOKEN")
      System.put_env("GITHUB_TOKEN", "ghp_test123")

      try do
        exec_fn = make_exec_fn()

        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

        assert_received {:exec_called, _}
        assert_received {:exec_called, _}
        assert_received {:exec_called, agent_cmd}
        assert String.contains?(agent_cmd, "GITHUB_TOKEN='ghp_test123'")
      after
        if prev, do: System.put_env("GITHUB_TOKEN", prev), else: System.delete_env("GITHUB_TOKEN")
      end
    end

    test "omits env exports when GITHUB_TOKEN is unset" do
      prev = System.get_env("GITHUB_TOKEN")
      System.delete_env("GITHUB_TOKEN")

      try do
        exec_fn = make_exec_fn()

        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

        assert_received {:exec_called, _}
        assert_received {:exec_called, _}
        assert_received {:exec_called, agent_cmd}
        refute String.contains?(agent_cmd, "GITHUB_TOKEN")
        assert String.contains?(agent_cmd, "LEFTHOOK=0")
      after
        if prev, do: System.put_env("GITHUB_TOKEN", prev)
      end
    end

    test "returns :ok tuple on success" do
      exec_fn = make_exec_fn()

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:ok, _} = result
    end
  end

  describe "dispatch/4 retry on agent crash" do
    test "retries with continue_command on non-zero agent exit" do
      exec_fn =
        make_exec_fn([
          # First agent run fails; continue succeeds
          {"agent -p", {:error, "agent crashed", 1}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:ok, _} = result

      # Drain kill and upload
      assert_received {:exec_called, _}
      assert_received {:exec_called, _}

      # First agent call (fails)
      assert_received {:exec_called, first_cmd}
      refute String.contains?(first_cmd, "--continue")

      # Retry call (continue)
      assert_received {:exec_called, retry_cmd}
      assert String.contains?(retry_cmd, "agent --continue -p")
    end

    test "returns error when harness has no continue_command" do
      exec_fn =
        make_exec_fn([
          {"agent -p", {:error, "agent crashed", 1}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: NoRetryHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:error, msg, _code} = result
      assert String.contains?(msg, "continuation")
    end

    test "propagates continue_command failure" do
      exec_fn =
        make_exec_fn([
          {"agent -p", {:error, "crashed", 1}},
          {"agent --continue", {:error, "still crashed", 2}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:error, "still crashed", 2} = result
    end
  end

  describe "dispatch/4 with Codex harness (default)" do
    test "dispatches via codex exec with yolo and web search" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        exec_fn: exec_fn,
        timeout: 1
      )

      # Drain kill and upload
      assert_received {:exec_called, _}
      assert_received {:exec_called, _}

      assert_received {:exec_called, agent_cmd}
      assert String.contains?(agent_cmd, "codex exec --yolo --json")
      assert String.contains?(agent_cmd, "--model gpt-5.4")
      assert String.contains?(agent_cmd, "web_search=live")
      assert String.contains?(agent_cmd, "model_reasoning_effort=medium")
    end

    test "passes harness_opts through to dispatch_command" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        exec_fn: exec_fn,
        harness_opts: [reasoning_effort: "high"],
        timeout: 1
      )

      assert_received {:exec_called, _}
      assert_received {:exec_called, _}
      assert_received {:exec_called, agent_cmd}
      assert String.contains?(agent_cmd, "model_reasoning_effort=high")
      refute String.contains?(agent_cmd, "model_reasoning_effort=medium")
    end

    test "returns error on failure (no retry — codex has no continuation)" do
      exec_fn =
        make_exec_fn([
          {"codex exec", {:error, "codex crashed", 1}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: Conductor.Codex,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:error, msg, _code} = result
      assert String.contains?(msg, "continuation")
    end
  end

  describe "dispatch/4 upload failure" do
    test "returns error when prompt upload fails" do
      exec_fn =
        make_exec_fn([
          {"base64 -d", {:error, "no space left", 1}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:error, msg, _code} = result
      assert String.contains?(msg, "prompt upload failed")
    end
  end
end
