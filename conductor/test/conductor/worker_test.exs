defmodule Conductor.WorkerTest do
  use ExUnit.Case, async: true

  @moduledoc """
  Verifies that:
  1. The Conductor.Worker behaviour can be satisfied by a plain module.
  2. The mock can be injected via Application config so RunServer
     and Orchestrator work without real sprites.
  """

  # ---------------------------------------------------------------------------
  # Mock Worker — no real sprite needed
  # ---------------------------------------------------------------------------

  defmodule MockWorker do
    @behaviour Conductor.Worker

    @impl Conductor.Worker
    def exec(_worker, command, _opts), do: {:ok, "exec:#{command}"}

    @impl Conductor.Worker
    def dispatch(_worker, prompt, _repo, _opts), do: {:ok, "dispatched:#{prompt}"}

    @impl Conductor.Worker
    def read_artifact(_worker, path, _opts) do
      {:ok, %{"status" => "ready", "path" => path, "pr_number" => 42, "pr_url" => "http://pr"}}
    end

    @impl Conductor.Worker
    def cleanup(_worker, _repo, _run_id), do: :ok
  end

  # ---------------------------------------------------------------------------
  # Tests
  # ---------------------------------------------------------------------------

  describe "MockWorker satisfies Worker behaviour" do
    test "exec/3 returns ok tuple" do
      assert {:ok, "exec:ls"} = MockWorker.exec("sprite-1", "ls", [])
    end

    test "dispatch/4 returns ok tuple" do
      assert {:ok, "dispatched:hello"} = MockWorker.dispatch("sprite-1", "hello", "org/repo", [])
    end

    test "read_artifact/3 returns decoded map" do
      assert {:ok, %{"status" => "ready", "pr_number" => 42}} =
               MockWorker.read_artifact("sprite-1", "/tmp/result.json", [])
    end

    test "cleanup/3 returns :ok" do
      assert :ok = MockWorker.cleanup("sprite-1", "org/repo", "run-42-000")
    end
  end

  describe "mock worker can be injected via Application config" do
    setup do
      original = Application.get_env(:conductor, :worker_module)

      Application.put_env(:conductor, :worker_module, MockWorker)

      on_exit(fn ->
        if original do
          Application.put_env(:conductor, :worker_module, original)
        else
          Application.delete_env(:conductor, :worker_module)
        end
      end)

      :ok
    end

    test "configured module resolves to MockWorker" do
      mod = Application.get_env(:conductor, :worker_module, Conductor.Sprite)
      assert mod == MockWorker
    end

    test "calling behaviour callbacks through resolved module works" do
      mod = Application.get_env(:conductor, :worker_module, Conductor.Sprite)
      assert {:ok, _} = mod.exec("w", "echo hi", [])
      assert {:ok, _} = mod.dispatch("w", "prompt", "org/repo", [])
      assert {:ok, _} = mod.read_artifact("w", "/path", [])
      assert :ok = mod.cleanup("w", "org/repo", "run-1")
    end
  end
end
