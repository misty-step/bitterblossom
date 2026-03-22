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
      MockState.put(:reconciled_sprites, [sprite.name | MockState.get(:reconciled_sprites, [])])

      case MockState.get({:sprite_health, sprite.name}, :healthy) do
        :healthy ->
          %{name: sprite.name, role: sprite.role, healthy: true, action: :none}

        :unhealthy ->
          %{name: sprite.name, role: sprite.role, healthy: false, action: :unreachable}
      end
    end
  end

  defmodule MockSprite do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def status(name, opts) do
      calls = [{name, opts} | MockState.get(:sprite_status_calls, [])]
      MockState.put(:sprite_status_calls, calls)
      MockState.get({:sprite_status, name}, {:ok, %{healthy: true}})
    end
  end

  defmodule MockSpriteStatus1 do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def status(name) do
      calls = [name | MockState.get(:sprite_status_1_calls, [])]
      MockState.put(:sprite_status_1_calls, calls)
      MockState.get({:sprite_status, name}, {:ok, %{healthy: true}})
    end
  end

  defmodule UnsupportedSprite do
  end

  defmodule MissingSupervisor do
  end

  defmodule NoopWorker do
    def dispatch(_, _, _, _), do: {:ok, ""}
    def exec(_, _, _), do: {:ok, ""}
    def cleanup(_, _, _), do: :ok
    def busy?(_, _), do: false
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
    orig_sprite = Application.get_env(:conductor, :sprite_module)
    Application.put_env(:conductor, :reconciler_module, MockReconciler)
    Application.put_env(:conductor, :sprite_module, MockSprite)

    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_supervisor_name = Application.get_env(:conductor, :supervisor_name)

    Application.put_env(:conductor, :worker_module, NoopWorker)
    Application.put_env(:conductor, :workspace_module, NoopWorkspace)
    Application.put_env(:conductor, :code_host_module, NoopCodeHost)

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
            {:sprite_module, orig_sprite},
            {:worker_module, orig_worker},
            {:workspace_module, orig_workspace},
            {:code_host_module, orig_code_host},
            {:supervisor_name, orig_supervisor_name}
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
    assert status.interval_ms == 60_000
    assert status.last_check_at == nil
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
    polisher = status.sprites["bb-polisher"]
    fixer = status.sprites["bb-fixer"]
    assert polisher.status == :healthy
    assert polisher.role == :polisher
    assert polisher.last_probe_at == nil
    assert fixer.status == :degraded
    assert fixer.role == :fixer
    assert fixer.consecutive_failures == 0
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
    assert HealthMonitor.status().sprites["bb-polisher"].status == :degraded

    # Trigger check — sprite is now healthy
    log =
      capture_log(fn ->
        HealthMonitor.check_now()
        eventually(fn -> HealthMonitor.status().sprites["bb-polisher"].status == :healthy end)
      end)

    assert log =~ "bb-polisher recovered"
    status = HealthMonitor.status()
    polisher = status.sprites["bb-polisher"]
    assert polisher.status == :healthy
    assert polisher.last_probe_at
    assert polisher.consecutive_failures == 0
    assert status.last_check_at
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

    assert HealthMonitor.status().sprites["bb-fixer"].status == :healthy

    log =
      capture_log(fn ->
        HealthMonitor.check_now()
        eventually(fn -> HealthMonitor.status().sprites["bb-fixer"].status == :degraded end)
      end)

    assert log =~ "bb-fixer degraded"
    status = HealthMonitor.status()
    fixer = status.sprites["bb-fixer"]
    assert fixer.status == :degraded
    assert fixer.consecutive_failures == 1
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

    assert HealthMonitor.status().sprites["bb-polisher"].status == :degraded

    HealthMonitor.check_now()
    eventually(fn -> HealthMonitor.status().sprites["bb-polisher"].status == :healthy end)

    status = HealthMonitor.status()
    polisher = status.sprites["bb-polisher"]
    assert polisher.status == :healthy
    assert polisher.last_probe_at
  end

  test "marks repeated probe failures as unavailable" do
    start_monitor(interval_ms: 60_000)
    threshold = Conductor.Config.fleet_probe_failure_threshold()

    sprites = [
      %{name: "bb-fixer", role: :fixer, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_health, "bb-fixer"}, :unhealthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-fixer"])
    )

    if threshold > 1 do
      for _ <- 1..(threshold - 1), do: HealthMonitor.check_now()

      eventually(fn ->
        sprite = HealthMonitor.status().sprites["bb-fixer"]
        sprite.status == :degraded and sprite.consecutive_failures == threshold - 1
      end)
    end

    HealthMonitor.check_now()

    eventually(fn ->
      sprite = HealthMonitor.status().sprites["bb-fixer"]
      sprite.status == :unavailable and sprite.consecutive_failures == threshold
    end)

    status = HealthMonitor.status()
    fixer = status.sprites["bb-fixer"]
    assert fixer.status == :unavailable
    assert fixer.consecutive_failures == threshold
  end

  test "checks builders via status/2 without running reconciler" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-weaver", role: :builder, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_status, "bb-weaver"}, {:ok, %{healthy: false}})

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-weaver"])
    )

    HealthMonitor.check_now()

    eventually(fn ->
      sprite = HealthMonitor.status().sprites["bb-weaver"]
      sprite.status == :degraded and sprite.consecutive_failures == 1
    end)

    status = HealthMonitor.status()
    builder = status.sprites["bb-weaver"]
    assert builder.status == :degraded
    assert builder.last_probe_at
    assert builder.consecutive_failures == 1
    assert MockState.get(:sprite_status_calls, []) == [{"bb-weaver", [harness: "codex"]}]
    assert MockState.get(:reconciled_sprites, []) == []
  end

  test "falls back to builder status/1 when status/2 is unavailable" do
    Application.put_env(:conductor, :sprite_module, MockSpriteStatus1)
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-weaver", role: :builder, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_status, "bb-weaver"}, {:ok, %{healthy: false}})

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-weaver"])
    )

    HealthMonitor.check_now()

    eventually(fn ->
      sprite = HealthMonitor.status().sprites["bb-weaver"]
      sprite.status == :degraded and sprite.consecutive_failures == 1
    end)

    assert MockState.get(:sprite_status_1_calls, []) == ["bb-weaver"]
    assert MockState.get(:reconciled_sprites, []) == []
  end

  test "unsupported builder status probes degrade before the configured threshold" do
    Application.put_env(:conductor, :sprite_module, UnsupportedSprite)
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-weaver", role: :builder, harness: "codex", repo: "test/repo"}
    ]

    threshold = Conductor.Config.fleet_probe_failure_threshold()

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-weaver"])
    )

    if threshold > 1 do
      for _ <- 1..(threshold - 1), do: HealthMonitor.check_now()

      eventually(fn ->
        sprite = HealthMonitor.status().sprites["bb-weaver"]
        sprite.status == :degraded and sprite.consecutive_failures == threshold - 1
      end)
    end

    HealthMonitor.check_now()

    eventually(fn ->
      sprite = HealthMonitor.status().sprites["bb-weaver"]
      sprite.status == :unavailable and sprite.consecutive_failures == threshold
    end)

    builder = HealthMonitor.status().sprites["bb-weaver"]
    assert builder.status == :unavailable
    assert builder.consecutive_failures == threshold
    assert MockState.get(:reconciled_sprites, []) == []
  end

  test "persists recovered health even when phase worker restart fails" do
    Application.put_env(:conductor, :supervisor_name, MissingSupervisor)
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

    HealthMonitor.check_now()

    eventually(fn -> HealthMonitor.status().sprites["bb-polisher"].status == :healthy end)

    polisher = HealthMonitor.status().sprites["bb-polisher"]
    assert polisher.status == :healthy
    assert polisher.consecutive_failures == 0
    assert Store.list_events("fleet") == []
  end

  test "does not emit a recovery event for a first healthy probe from unknown state" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-weaver", role: :builder, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_status, "bb-weaver"}, {:ok, %{healthy: true}})

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new()
    )

    :sys.replace_state(HealthMonitor, fn state ->
      %{state | sprite_statuses: Map.delete(state.sprite_statuses, "bb-weaver")}
    end)

    log =
      capture_log(fn ->
        HealthMonitor.check_now()
        eventually(fn -> HealthMonitor.status().sprites["bb-weaver"].status == :healthy end)
      end)

    refute log =~ "bb-weaver recovered"
    assert Store.list_events("fleet") == []
    assert HealthMonitor.status().sprites["bb-weaver"].status == :healthy
  end

  defp eventually(fun, timeout_ms \\ 1_000, step_ms \\ 10) do
    deadline = System.monotonic_time(:millisecond) + timeout_ms
    do_eventually(fun, deadline, step_ms)
  end

  defp do_eventually(fun, deadline, step_ms) do
    if fun.() do
      :ok
    else
      if System.monotonic_time(:millisecond) < deadline do
        Process.sleep(step_ms)
        do_eventually(fun, deadline, step_ms)
      else
        flunk("condition not met before timeout")
      end
    end
  end
end
