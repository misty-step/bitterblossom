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
          loop_alive = MockState.get({:loop_alive, sprite.name}, false)

          %{
            name: sprite.name,
            role: sprite.role,
            healthy: true,
            loop_alive: loop_alive,
            action: :none
          }

        :unhealthy ->
          %{
            name: sprite.name,
            role: sprite.role,
            healthy: false,
            loop_alive: false,
            action: :unreachable
          }
      end
    end
  end

  defmodule MockLauncher do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def launch(sprite, repo) do
      MockState.put({:launch, sprite.name}, repo)
      send(MockState.get(:test_pid), {:launched, sprite.name, repo})
      {:ok, "launched"}
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(HealthMonitor)
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_reconciler = Application.get_env(:conductor, :reconciler_module)
    orig_launcher = Application.get_env(:conductor, :launcher_module)
    Application.put_env(:conductor, :reconciler_module, MockReconciler)
    Application.put_env(:conductor, :launcher_module, MockLauncher)
    MockState.put(:test_pid, self())

    on_exit(fn ->
      stop_process(HealthMonitor)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Store)
      MockState.cleanup()

      if orig_reconciler,
        do: Application.put_env(:conductor, :reconciler_module, orig_reconciler),
        else: Application.delete_env(:conductor, :reconciler_module)

      if orig_launcher,
        do: Application.put_env(:conductor, :launcher_module, orig_launcher),
        else: Application.delete_env(:conductor, :launcher_module)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  defp start_monitor(opts \\ []) do
    {:ok, pid} = HealthMonitor.start_link(interval_ms: Keyword.get(opts, :interval_ms, 60_000))
    pid
  end

  defp wait_for(fun, attempts \\ 20)
  defp wait_for(_fun, 0), do: flunk("timed out waiting for health monitor state")

  defp wait_for(fun, attempts) do
    case fun.() do
      nil ->
        Process.sleep(25)
        wait_for(fun, attempts - 1)

      value ->
        value
    end
  end

  test "starts with empty state and reports status" do
    start_monitor()

    status = HealthMonitor.status()
    assert status.sprites == %{}
    assert status.repo == nil
  end

  test "configure sets sprites and initial health with launching state" do
    start_monitor()

    sprites = [
      %{name: "bb-polisher", role: :polisher, harness: "codex"},
      %{name: "bb-fixer", role: :fixer, harness: "codex"}
    ]

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    status = HealthMonitor.status()
    assert status.sprites["bb-polisher"] == :launching
    assert status.sprites["bb-fixer"] == :unhealthy
  end

  test "launching sprite with loop alive transitions to healthy" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    assert HealthMonitor.status().sprites["bb-polisher"] == :launching

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher loop confirmed"
    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
  end

  test "launching sprite without loop stays launching" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(100)

    assert HealthMonitor.status().sprites["bb-polisher"] == :launching
  end

  test "launching sprite times out after max ticks" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    log =
      capture_log(fn ->
        # Tick 3 times to exceed @max_launch_ticks (3)
        for _ <- 1..3 do
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end
      end)

    assert log =~ "launch timed out"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end

  test "healthy sprite with loop exiting transitions to unhealthy" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    # Start as healthy (with confirmed loop)
    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-polisher"])
    )

    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy

    # Simulate loop death
    MockState.put({:loop_alive, "bb-polisher"}, false)

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher loop exited"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end

  test "detects unhealthy→recovered and relaunches when no loop" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher recovered, relaunching loop"
    assert HealthMonitor.status().sprites["bb-polisher"] == :launching
    assert_receive {:launched, "bb-polisher", "test/repo"}
  end

  test "detects unhealthy→recovered with loop already running" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher recovered (loop already running)"
    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
  end

  test "detects healthy→unhealthy transition and logs degradation" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-fixer", role: :fixer, harness: "codex"}]
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

    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert HealthMonitor.status().sprites == %{}
  end

  test "check_now triggers immediate probe" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    HealthMonitor.check_now()

    wait_for(fn ->
      if HealthMonitor.status().sprites["bb-polisher"] == :healthy, do: :ok
    end)
  end

  test "records recovery event in store" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    HealthMonitor.check_now()

    wait_for(fn ->
      if HealthMonitor.status().sprites["bb-polisher"] == :healthy, do: :ok
    end)

    assert Enum.any?(Store.list_events("fleet"), &(&1["event_type"] == "sprite_recovered"))
  end

  test "relaunches a recovered sprite with its configured repo override" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex", repo: "other/repo"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(sprites: sprites, repo: "default/repo")
    HealthMonitor.check_now()

    assert_receive {:launched, "bb-polisher", "other/repo"}
  end

  test "rapid exit detection and backoff suppresses relaunch after repeated fast exits" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    # Cycle 3 rapid exits: launching → healthy (confirm) → unhealthy (rapid exit)
    for i <- 1..3 do
      # Confirm loop alive → :healthy (sets launch_time)
      MockState.put({:loop_alive, "bb-polisher"}, true)

      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      assert HealthMonitor.status().sprites["bb-polisher"] == :healthy

      # Kill loop → :unhealthy (rapid exit, since launch_time was just set)
      MockState.put({:loop_alive, "bb-polisher"}, false)

      log =
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end)

      assert log =~ "rapid exit (#{i}x)"
      assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

      # Recovery → relaunch (count < 3 allows it)
      if i < 3 do
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end)

        assert HealthMonitor.status().sprites["bb-polisher"] == :launching
      end
    end

    # After 3 rapid exits, relaunch should be suppressed (backoff)
    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "backing off relaunch"
    # Should stay :unhealthy, not transition to :launching
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end
end
