defmodule Conductor.SpriteDispatchTest do
  use ExUnit.Case, async: false

  @moduledoc """
  Verifies the `Conductor.Sprite.dispatch/4` sequence using an injected
  exec_fn — no real sprite required.

  Sequence under test:
  1. Kill stale agent processes
  2. Upload prompt and runtime env files
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

    fn _sprite, command, opts ->
      uploaded_files =
        opts
        |> Keyword.get(:files, [])
        |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

      send(test_pid, {:exec_called, command, opts, uploaded_files})

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

      assert_received {:exec_called, kill_cmd, _opts, _files}
      assert String.contains?(kill_cmd, "pkill")
    end

    test "uploads prompt and runtime env files without embedding secret contents in argv" do
      prev_openai = System.get_env("OPENAI_API_KEY")
      prev_exa = System.get_env("EXA_API_KEY")
      System.put_env("OPENAI_API_KEY", "sk-test-123")
      System.put_env("EXA_API_KEY", "exa-test-456")

      try do
        exec_fn = make_exec_fn()

        Sprite.dispatch("s1", "my test prompt", "org/repo",
          workspace: "/my/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

        assert_received {:exec_called, _kill_cmd, _opts, _files}
        assert_received {:exec_called, upload_cmd, upload_opts, uploaded_files}
        assert upload_cmd == "true"
        assert Keyword.has_key?(upload_opts, :files)

        assert {"/my/ws/PROMPT.md", "my test prompt"} in uploaded_files

        assert {"/my/ws/.bb-runtime-env", runtime_env} =
                 Enum.find(uploaded_files, fn {dest, _content} ->
                   dest == "/my/ws/.bb-runtime-env"
                 end)

        assert runtime_env =~ "export OPENAI_API_KEY='sk-test-123'"
        assert runtime_env =~ "export CODEX_API_KEY='sk-test-123'"
        assert runtime_env =~ "export EXA_API_KEY='exa-test-456'"
        refute runtime_env =~ "GITHUB_TOKEN"
        refute upload_cmd =~ "sk-test-123"
        refute upload_cmd =~ "exa-test-456"
      after
        if prev_openai,
          do: System.put_env("OPENAI_API_KEY", prev_openai),
          else: System.delete_env("OPENAI_API_KEY")

        if prev_exa,
          do: System.put_env("EXA_API_KEY", prev_exa),
          else: System.delete_env("EXA_API_KEY")
      end
    end

    test "uploads prompt to workspace/PROMPT.md" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "my test prompt", "org/repo",
        workspace: "/my/ws",
        harness: MockHarness,
        exec_fn: exec_fn,
        timeout: 1
      )

      assert_received {:exec_called, _kill_cmd, _opts, _files}
      assert_received {:exec_called, _upload_cmd, _upload_opts, uploaded_files}
      assert {"/my/ws/PROMPT.md", "my test prompt"} in uploaded_files
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
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}

      assert_received {:exec_called, agent_cmd, _opts, _files}
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

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "LEFTHOOK=0")
    end

    test "tees agent output into ralph.log for later tailing" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        harness: MockHarness,
        exec_fn: exec_fn,
        timeout: 1
      )

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "tee -a '/ws/ralph.log'")
      assert String.contains?(agent_cmd, "set -o pipefail")
    end

    test "does not inject GITHUB_TOKEN from env" do
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

        assert_received {:exec_called, _, _, _}
        assert_received {:exec_called, _upload_cmd, _upload_opts, uploaded_files}
        assert_received {:exec_called, agent_cmd, _opts, _files}
        refute String.contains?(agent_cmd, "GITHUB_TOKEN")

        refute Enum.any?(uploaded_files, fn {_dest, content} ->
                 String.contains?(content, "GITHUB_TOKEN")
               end)
      after
        if prev, do: System.put_env("GITHUB_TOKEN", prev), else: System.delete_env("GITHUB_TOKEN")
      end
    end

    test "still omits GITHUB_TOKEN when unset" do
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

        assert_received {:exec_called, _, _, _}
        assert_received {:exec_called, _, _, _}
        assert_received {:exec_called, agent_cmd, _opts, _files}
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
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}

      # First agent call (fails)
      assert_received {:exec_called, first_cmd, _opts, _files}
      refute String.contains?(first_cmd, "--continue")

      # Retry call (continue)
      assert_received {:exec_called, retry_cmd, _opts, _files}
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
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, "command -v codex >/dev/null 2>&1", _, _}

      assert_received {:exec_called, agent_cmd, _opts, _files}
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

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, "command -v codex >/dev/null 2>&1", _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "model_reasoning_effort=high")
      refute String.contains?(agent_cmd, "model_reasoning_effort=medium")
    end

    test "drops harness opts when falling back to a different harness" do
      exec_fn =
        make_exec_fn([
          {"command -v codex", {:error, "", 1}},
          {"command -v claude", {:ok, ""}}
        ])

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        exec_fn: exec_fn,
        harness: "codex",
        harness_opts: [model: "gpt-5.4", reasoning_effort: "high"],
        timeout: 1
      )

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, "command -v codex >/dev/null 2>&1", _, _}
      assert_received {:exec_called, "command -v claude >/dev/null 2>&1", _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "claude -p --dangerously-skip-permissions")
      refute String.contains?(agent_cmd, "--model gpt-5.4")
      refute String.contains?(agent_cmd, "model_reasoning_effort=high")
    end

    test "returns actionable error for unsupported configured harness names" do
      exec_fn = make_exec_fn()

      assert {:error, msg, 78} =
               Sprite.dispatch("s1", "prompt", "org/repo",
                 workspace: "/ws",
                 exec_fn: exec_fn,
                 harness: "claude_code",
                 timeout: 1
               )

      assert msg =~ "configured harness claude_code is unsupported on sprite s1"
      assert msg =~ "supported harnesses: codex (codex CLI), claude-code (claude CLI)"
    end

    test "keeps execution rooted at the workspace and prepends AGENTS persona for codex" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        exec_fn: exec_fn,
        persona_role: :thorn,
        timeout: 1
      )

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, "command -v codex >/dev/null 2>&1", _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "cd '/ws'")

      assert String.contains?(
               agent_cmd,
               "cat '/ws/.bb/persona/thorn/AGENTS.md' '/ws/PROMPT.md' | LEFTHOOK=0 codex exec"
             )
    end

    test "rejects invalid persona roles early" do
      exec_fn = make_exec_fn()

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          exec_fn: exec_fn,
          persona_role: :unknown,
          timeout: 1
        )

      assert {:error, msg, 1} = result
      assert String.contains?(msg, "invalid persona role")
      refute_received {:exec_called, _, _, _}
    end

    test "runs claude code from the workspace root without persona argv injection" do
      exec_fn = make_exec_fn()

      Sprite.dispatch("s1", "prompt", "org/repo",
        workspace: "/ws",
        harness: Conductor.ClaudeCode,
        exec_fn: exec_fn,
        persona_role: :fern,
        timeout: 1
      )

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, "command -v claude >/dev/null 2>&1", _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "cd '/ws'")
      assert String.contains?(agent_cmd, "< '/ws/PROMPT.md'")
      refute String.contains?(agent_cmd, "--append-system-prompt")
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
      assert String.contains?(msg, "[bb harness] selected harness codex has no continuation")
      assert String.contains?(msg, "codex crashed")
    end

    test "falls back to claude-code when the configured codex harness is unavailable" do
      exec_fn =
        make_exec_fn([
          {"command -v codex", {:error, "", 127}},
          {"command -v claude", {:ok, "/usr/bin/claude\n"}}
        ])

      assert {:ok, _} =
               Sprite.dispatch("s1", "prompt", "org/repo",
                 workspace: "/ws",
                 harness: Conductor.Codex,
                 exec_fn: exec_fn,
                 timeout: 1
               )

      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, _, _, _}
      assert_received {:exec_called, "command -v codex >/dev/null 2>&1", _, _}
      assert_received {:exec_called, "command -v claude >/dev/null 2>&1", _, _}
      assert_received {:exec_called, agent_cmd, _opts, _files}
      assert String.contains?(agent_cmd, "claude -p --dangerously-skip-permissions")
    end

    test "returns actionable error when no supported harness is available on the sprite" do
      exec_fn =
        make_exec_fn([
          {"command -v codex", {:error, "", 127}},
          {"command -v claude", {:error, "", 127}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: Conductor.Codex,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:error, msg, 78} = result
      assert String.contains?(msg, "configured harness codex unavailable on sprite s1")
      assert String.contains?(msg, "command -v codex >/dev/null 2>&1 -> missing")
      assert String.contains?(msg, "command -v claude >/dev/null 2>&1 -> missing")

      assert String.contains?(
               msg,
               "supported harnesses: codex (codex CLI), claude-code (claude CLI)"
             )
    end
  end

  describe "dispatch/4 upload failure" do
    test "returns error when prompt upload fails" do
      exec_fn =
        make_exec_fn([
          {"true", {:error, "no space left", 1}}
        ])

      result =
        Sprite.dispatch("s1", "prompt", "org/repo",
          workspace: "/ws",
          harness: MockHarness,
          exec_fn: exec_fn,
          timeout: 1
        )

      assert {:error, msg, _code} = result
      assert String.contains?(msg, "dispatch file upload failed")
    end
  end
end
