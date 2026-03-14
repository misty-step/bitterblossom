defmodule Conductor.WorkerTest do
  use ExUnit.Case, async: true

  # A mock Worker that records calls and returns canned responses.
  # Demonstrates that a new Worker implementation requires only this behaviour.
  defmodule MockWorker do
    @behaviour Conductor.Worker

    @impl Conductor.Worker
    def exec(_worker, _command, _opts), do: {:ok, "mock output"}

    @impl Conductor.Worker
    def dispatch(_worker, _prompt, _repo, _opts), do: {:ok, "dispatch ok"}

    @impl Conductor.Worker
    def read_artifact(_worker, _path, _opts) do
      {:ok, %{"status" => "ready", "pr_number" => 99, "pr_url" => "http://example.com/pr/99"}}
    end

    @impl Conductor.Worker
    def cleanup(_worker, _repo, _run_id), do: :ok
  end

  describe "Conductor.Worker behaviour" do
    test "mock worker satisfies all callbacks" do
      assert MockWorker.exec("w1", "echo hi", []) == {:ok, "mock output"}
      assert MockWorker.dispatch("w1", "prompt", "org/repo", []) == {:ok, "dispatch ok"}

      assert MockWorker.read_artifact("w1", "/some/path", []) ==
               {:ok,
                %{"status" => "ready", "pr_number" => 99, "pr_url" => "http://example.com/pr/99"}}

      assert MockWorker.cleanup("w1", "org/repo", "run-1") == :ok
    end

    test "Conductor.Sprite implements the Worker behaviour" do
      # Compile-time enforcement: @behaviour Conductor.Worker in Sprite.
      # At runtime we verify the functions exist with the correct arity.
      # ensure_loaded! required because Sprite is a pure module (not OTP-started).
      Code.ensure_loaded!(Conductor.Sprite)
      assert function_exported?(Conductor.Sprite, :exec, 3)
      assert function_exported?(Conductor.Sprite, :dispatch, 4)
      assert function_exported?(Conductor.Sprite, :read_artifact, 3)
      assert function_exported?(Conductor.Sprite, :cleanup, 3)
    end
  end

  describe "Conductor.Tracker behaviour" do
    test "Conductor.GitHub implements the Tracker behaviour" do
      Code.ensure_loaded!(Conductor.GitHub)
      assert function_exported?(Conductor.GitHub, :list_eligible, 2)
      assert function_exported?(Conductor.GitHub, :get_issue, 2)
      assert function_exported?(Conductor.GitHub, :comment, 3)
    end
  end

  describe "Conductor.CodeHost behaviour" do
    test "Conductor.GitHub implements the CodeHost behaviour" do
      Code.ensure_loaded!(Conductor.GitHub)
      assert function_exported?(Conductor.GitHub, :checks_green?, 2)
      assert function_exported?(Conductor.GitHub, :merge, 3)
    end
  end

  describe "Conductor.Harness behaviour" do
    test "Conductor.ClaudeCode implements the Harness behaviour" do
      Code.ensure_loaded!(Conductor.ClaudeCode)
      assert function_exported?(Conductor.ClaudeCode, :name, 0)
      assert function_exported?(Conductor.ClaudeCode, :dispatch_command, 1)
    end

    test "ClaudeCode name is claude-code" do
      assert Conductor.ClaudeCode.name() == "claude-code"
    end

    test "ClaudeCode dispatch_command returns {executable, args}" do
      {exe, args} = Conductor.ClaudeCode.dispatch_command([])
      assert exe == "claude"
      assert "-p" in args
      assert "--dangerously-skip-permissions" in args
    end

    test "ClaudeCode dispatch_command accepts model opt" do
      {_exe, args} = Conductor.ClaudeCode.dispatch_command(model: "anthropic/claude-opus-4")
      assert "--model" in args
      idx = Enum.find_index(args, &(&1 == "--model"))
      assert Enum.at(args, idx + 1) == "anthropic/claude-opus-4"
    end
  end
end
