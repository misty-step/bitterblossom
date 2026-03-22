defmodule Conductor.Fleet.HealthMonitorTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Fleet.HealthMonitor
  alias Conductor.Store

  defmodule MockState do
    def put(key, value), do: :persistent_term.put({__MODULE__, key}, value)

    def get(key, default \\ nil) do
      try do
        :persistent_term.get({__MODULE__, key})
      rescue
        ArgumentError -> default
      end
    end

    def cleanup do
      for {{__MODULE__, _} = k, _} <- :persistent_term.get(), do: :persistent_term.erase(k)
    end
  end

  defmodule MockReconciler do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def reconcile_sprite(sprite) do
      calls = MockState.get(:reconcile_calls, [])
      MockState.put(:reconcile_calls, calls ++ [sprite.name])

      health =
        case MockState.get({:reconcile_script, sprite.name}) do
          [next | rest] ->
            MockState.put({:reconcile_script, sprite.name}, rest)
            next

          _ ->
            MockState.get({:sprite_health, sprite.name}, :healthy)
        end

      reconcile_result(sprite, health)
    end

    defp reconcile_result(sprite, :healthy) do
      %{name: sprite.name, role: sprite.role, healthy: true, action: :none}
    end

    defp reconcile_result(sprite, :unhealthy) do
      %{name: sprite.name, role: sprite.role, healthy: false, action: :unreachable}
    end

    defp reconcile_result(sprite, {:unhealthy, action}) do
      %{name: sprite.name, role: sprite.role, healthy: false, action: action}
    end
  end

  defmodule NoopWorker do
    def dispatch(_, _, _, _), do: {:ok, ""}
    def exec(_, _, _), do: {:ok, ""}
    def cleanup(_, _, _), do: :ok
    def busy?(_, _), do: false
  end

  defmodule MockSprite do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def gc_checkpoints(sprite, _opts \\ []) do
      calls = MockState.get(:gc_calls, [])
      MockState.put(:gc_calls, calls ++ [sprite])
      :ok
    end

    def check_stuck(sprite, _opts \\ []) do
      calls = MockState.get(:stuck_calls, [])
      MockState.put(:stuck_calls, calls ++ [sprite])
      MockState.get({:check_stuck_result, sprite}, {:ok, :not_stuck})
    end
  end

  defmodule NoopCodeHost do
    def open_prs(_), do: {:ok, []}
    def labeled_prs(_, _), do: {:ok, []}
    def checks_green?(_, _), do: false
    def checks_failed?(_, _), do: false
  end

  defmodule NoopWorkspace do
    def sync_persona(_, _, _, _ \\ []), do: :ok
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(HealthMonitor)
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    # Start a test supervisor so ensure_phase_worker can start children
    stop_process(Conductor.Supervisor)
    {:ok, _} = Supervisor.start_link([], strategy: :one_for_one, name: Conductor.Supervisor)
    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_reconciler = Application.get_env(:conductor, :reconciler_module)
    Application.put_env(:conductor, :reconciler_module, MockReconciler)

    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_sprite = Application.get_env(:conductor, :sprite_module)

    Application.put_env(:conductor, :worker_module, NoopWorker)
    Application.put_env(:conductor, :workspace_module, NoopWorkspace)
    Application.put_env(:conductor, :code_host_module, NoopCodeHost)
    Application.put_env(:conductor, :sprite_module, MockSprite)

    on_exit(fn ->
      stop_process(Conductor.Polisher)
      stop_process(Conductor.Fixer)
      stop_process(HealthMonitor)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Conductor.Supervisor)
      stop_process(Store)
      MockState.cleanup()

      for {key, orig} <- [
            {:reconciler_module, orig_reconciler},
            {:worker_module, orig_worker},
            {:workspace_module, orig_workspace},
            {:code_host_module, orig_code_host},
            {:sprite_module, orig_sprite}
          ] do
        if orig,
          do: Application.put_env(:conductor, key, orig),
          else: Application.delete_env(:conductor, key)
      end

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  defp start_monitor(opts \\ []) do
    now_ms_fn = Keyword.get(opts, :now_ms_fn, fn -> MockState.get(:now_ms, 0) end)

    {:ok, pid} =
      HealthMonitor.start_link(
        interval_ms: Keyword.get(opts, :interval_ms, 60_000),
        now_ms_fn: now_ms_fn
      )

    pid
  end

  test "starts with empty state and reports status" do
    start_monitor()

    status = HealthMonitor.status()
    assert status.sprites == %{}
    assert status.repo == nil
  end

  test "configure sets sprites and initial health" do
    start_monitor()

    sprites = [
      %{name: "bb-polisher", role: :polisher, harness: "codex", repo: "test/repo"},
      %{name: "bb-fixer", role: :fixer, harness: "codex", repo: "test/repo"}
    ]

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-polisher"])
    )

    status = HealthMonitor.status()
    assert status.sprites["bb-polisher"] == :healthy
    assert status.sprites["bb-fixer"] == :unhealthy
  end

  test "detects unhealthy→healthy transition and logs recovery" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-polisher", role: :polisher, harness: "codex", repo: "test/repo"}
    ]

    # Start as unhealthy
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new()
    )

    # Verify starts unhealthy
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    # Trigger check — sprite is now healthy
    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher recovered"
    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
  end

  test "detects healthy→unhealthy transition and logs degradation" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-fixer", role: :fixer, harness: "codex", repo: "test/repo"}
    ]

    # Start as healthy, but mock returns unhealthy
    MockState.put({:sprite_health, "bb-fixer"}, :unhealthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-fixer"])
    )

    assert HealthMonitor.status().sprites["bb-fixer"] == :healthy

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-fixer degraded"
    assert HealthMonitor.status().sprites["bb-fixer"] == :unhealthy
  end

  test "no-ops when sprites list is empty" do
    start_monitor(interval_ms: 60_000)

    # Trigger check with no sprites configured
    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert HealthMonitor.status().sprites == %{}
  end

  test "check_now triggers immediate probe" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-polisher", role: :polisher, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_health, "bb-polisher"}, :healthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new()
    )

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    HealthMonitor.check_now()
    Process.sleep(100)

    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
  end

  test "runs checkpoint gc every 30 minutes for healthy sprites" do
    MockState.put(:now_ms, 0)
    start_monitor(interval_ms: 20 * 60_000)

    sprites = [
      %{name: "bb-builder", role: :builder, harness: "codex", repo: "test/repo"},
      %{name: "bb-polisher", role: :polisher, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_health, "bb-polisher"}, :healthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-builder", "bb-polisher"])
    )

    MockState.put(:now_ms, 20 * 60_000)
    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert MockState.get(:gc_calls, []) == []

    MockState.put(:now_ms, 30 * 60_000)
    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert MockState.get(:gc_calls, []) == ["bb-builder", "bb-polisher"]
  end

  test "checkpoint gc skips unhealthy sprites" do
    MockState.put(:now_ms, 0)
    start_monitor(interval_ms: 120_000)

    sprites = [
      %{name: "bb-builder", role: :builder, harness: "codex", repo: "test/repo"},
      %{name: "bb-polisher", role: :polisher, harness: "codex", repo: "test/repo"},
      %{name: "bb-fixer", role: :fixer, harness: "codex", repo: "test/repo"}
    ]

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-builder", "bb-polisher"])
    )

    MockState.put({:sprite_health, "bb-fixer"}, :unhealthy)

    MockState.put(:now_ms, 30 * 60_000)
    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert MockState.get(:gc_calls, []) == ["bb-builder", "bb-polisher"]
  end

  test "recreates stuck sprites before marking them degraded" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{
        name: "bb-polisher",
        role: :polisher,
        org: "misty-step",
        harness: "codex",
        repo: "test/repo"
      }
    ]

    MockState.put({:reconcile_script, "bb-polisher"}, [:unhealthy, :healthy])
    MockState.put({:check_stuck_result, "bb-polisher"}, {:ok, :recreated})

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new()
    )

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher was stuck; recreating sprite"
    assert log =~ "bb-polisher recovered"
    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
    assert MockState.get(:reconcile_calls, []) == ["bb-polisher", "bb-polisher"]
    assert MockState.get(:stuck_calls, []) == ["bb-polisher"]
  end

  test "does not run stuck recovery for non-unreachable reconciler failures" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{
        name: "bb-polisher",
        role: :polisher,
        org: "misty-step",
        harness: "codex",
        repo: "test/repo"
      }
    ]

    MockState.put({:reconcile_script, "bb-polisher"}, [{:unhealthy, :setup_incomplete}])

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-polisher"])
    )

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher degraded"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
    assert MockState.get(:stuck_calls, []) == []
    assert MockState.get(:reconcile_calls, []) == ["bb-polisher"]
  end

  test "logs stuck-check failures and marks the sprite unhealthy" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{
        name: "bb-polisher",
        role: :polisher,
        org: "misty-step",
        harness: "codex",
        repo: "test/repo"
      }
    ]

    MockState.put({:sprite_health, "bb-polisher"}, :unhealthy)
    MockState.put({:check_stuck_result, "bb-polisher"}, {:error, "api timeout"})

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-polisher"])
    )

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "stuck check failed for bb-polisher"
    assert log =~ "bb-polisher degraded"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
    assert MockState.get(:stuck_calls, []) == ["bb-polisher"]
  end

  test "keeps sprites unhealthy when phase-worker startup fails" do
    orig_supervisor_name = Application.get_env(:conductor, :supervisor_name)
    Application.put_env(:conductor, :supervisor_name, MissingSupervisor)

    on_exit(fn ->
      if orig_supervisor_name,
        do: Application.put_env(:conductor, :supervisor_name, orig_supervisor_name),
        else: Application.delete_env(:conductor, :supervisor_name)
    end)

    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-polisher", role: :polisher, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_health, "bb-polisher"}, :healthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new()
    )

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher recovered"
    assert log =~ "supervisor unavailable"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end

  test "builder health is refreshed before periodic checkpoint gc without phase-worker recovery" do
    MockState.put(:now_ms, 0)
    start_monitor(interval_ms: 120_000)

    sprites = [
      %{
        name: "bb-builder",
        role: :builder,
        org: "misty-step",
        harness: "codex",
        repo: "test/repo"
      },
      %{
        name: "bb-polisher",
        role: :polisher,
        org: "misty-step",
        harness: "codex",
        repo: "test/repo"
      }
    ]

    MockState.put({:sprite_health, "bb-builder"}, :unhealthy)
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-builder", "bb-polisher"])
    )

    MockState.put(:now_ms, 30 * 60_000)

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    refute log =~ "bb-builder recovered"
    refute log =~ "bb-builder degraded"
    assert MockState.get(:gc_calls, []) == ["bb-polisher"]
  end
end
