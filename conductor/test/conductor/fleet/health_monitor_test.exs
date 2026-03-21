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

  setup do
    db_path = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(HealthMonitor)
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    orig_reconciler = Application.get_env(:conductor, :reconciler_module)
    Application.put_env(:conductor, :reconciler_module, MockReconciler)

    on_exit(fn ->
      stop_process(HealthMonitor)
      stop_process(Store)
      MockState.cleanup()

      if orig_reconciler,
        do: Application.put_env(:conductor, :reconciler_module, orig_reconciler),
        else: Application.delete_env(:conductor, :reconciler_module)

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
end
