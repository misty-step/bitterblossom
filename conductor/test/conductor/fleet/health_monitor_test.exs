defmodule Conductor.Fleet.HealthMonitorTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Fleet.HealthMonitor
  alias Conductor.PhaseWorker
  alias Conductor.PhaseWorker.Roles
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

  defmodule NoopCodeHost do
    def open_prs(_), do: {:ok, []}
    def labeled_prs(_, _), do: {:ok, []}
    def checks_green?(_, _), do: false
    def checks_failed?(_, _), do: false
  end

  defmodule NoopWorkspace do
    def sync_persona(_, _, _, _ \\ []), do: :ok
  end

  defmodule FailingPhaseWorkerSupervisor do
    def ensure_worker(_role_module, _repo, _sprites), do: {:error, :sync_failed}
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(HealthMonitor)
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    stop_process(Conductor.PhaseWorker.Supervisor)
    stop_process(Conductor.PhaseWorkerRegistry)
    stop_process(Conductor.TaskSupervisor)

    {:ok, _} = Registry.start_link(keys: :unique, name: Conductor.PhaseWorkerRegistry)
    {:ok, _} = Conductor.PhaseWorker.Supervisor.start_link()
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_reconciler = Application.get_env(:conductor, :reconciler_module)
    Application.put_env(:conductor, :reconciler_module, MockReconciler)

    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
    orig_phase_worker_sprites = Application.get_env(:conductor, :phase_worker_sprites)

    Application.put_env(:conductor, :worker_module, NoopWorker)
    Application.put_env(:conductor, :workspace_module, NoopWorkspace)
    Application.put_env(:conductor, :code_host_module, NoopCodeHost)

    on_exit(fn ->
      stop_process(HealthMonitor)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Conductor.PhaseWorker.Supervisor)
      stop_process(Conductor.PhaseWorkerRegistry)
      stop_process(Store)
      MockState.cleanup()

      for {key, orig} <- [
            {:reconciler_module, orig_reconciler},
            {:worker_module, orig_worker},
            {:workspace_module, orig_workspace},
            {:code_host_module, orig_code_host},
            {:phase_worker_supervisor, orig_phase_worker_supervisor},
            {:phase_worker_sprites, orig_phase_worker_sprites}
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
    assert PhaseWorker.whereis(Roles.Polisher)
    assert PhaseWorker.status(Roles.Polisher).sprites == ["bb-polisher"]
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

  test "recovered sprite joins the existing role worker instead of starting a second singleton" do
    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-polisher-1", role: :polisher, harness: "codex", repo: "test/repo"},
      %{name: "bb-polisher-2", role: :polisher, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_health, "bb-polisher-1"}, :healthy)
    MockState.put({:sprite_health, "bb-polisher-2"}, :healthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-polisher-1"])
    )

    :ok =
      Conductor.PhaseWorker.Supervisor.ensure_worker(
        Roles.Polisher,
        "test/repo",
        ["bb-polisher-1"]
      )

    assert PhaseWorker.status(Roles.Polisher).sprites == ["bb-polisher-1"]

    HealthMonitor.check_now()
    Process.sleep(100)

    assert PhaseWorker.status(Roles.Polisher).sprites == ["bb-polisher-1", "bb-polisher-2"]
  end

  test "keeps degraded health state when phase worker sync fails" do
    Application.put_env(:conductor, :phase_worker_supervisor, FailingPhaseWorkerSupervisor)

    start_monitor(interval_ms: 60_000)

    sprites = [
      %{name: "bb-fixer", role: :fixer, harness: "codex", repo: "test/repo"}
    ]

    MockState.put({:sprite_health, "bb-fixer"}, :unhealthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-fixer"])
    )

    assert HealthMonitor.status().sprites["bb-fixer"] == :healthy

    log =
      capture_log(fn ->
        HealthMonitor.check_now()
        Process.sleep(100)
      end)

    assert log =~ "failed to sync"
    assert HealthMonitor.status().sprites["bb-fixer"] == :unhealthy
  end
end
