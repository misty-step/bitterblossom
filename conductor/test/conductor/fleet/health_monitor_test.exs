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
      case MockState.get({:sprite_health, sprite.name}, :healthy) do
        :healthy ->
          %{name: sprite.name, role: sprite.role, healthy: true, action: :none}

        :unhealthy ->
          %{name: sprite.name, role: sprite.role, healthy: false, action: :unreachable}
      end
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
    {:ok, pid} = HealthMonitor.start_link(interval_ms: Keyword.get(opts, :interval_ms, 60_000))
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
    start_monitor(interval_ms: 120_000)

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

    Enum.each(1..14, fn _ ->
      send(Process.whereis(HealthMonitor), :check)
      Process.sleep(20)
    end)

    assert MockState.get(:gc_calls, []) == []

    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert MockState.get(:gc_calls, []) == ["bb-builder", "bb-polisher"]
  end

  test "checkpoint gc skips unhealthy sprites" do
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

    Enum.each(1..15, fn _ ->
      send(Process.whereis(HealthMonitor), :check)
      Process.sleep(20)
    end)

    assert MockState.get(:gc_calls, []) == ["bb-builder", "bb-polisher"]
  end
end
