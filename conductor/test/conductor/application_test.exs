defmodule Conductor.ApplicationTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  defmodule MockReconciler do
    def reconcile_all(sprites, _opts \\ []) do
      {:ok, Enum.map(sprites, &%{name: &1.name, healthy: true, action: :none})}
    end
  end

  defmodule FailingPhaseWorkerSupervisor do
    def ensure_worker(_role_module, _repo, []), do: :ok
    def ensure_worker(_role_module, _repo, _sprites), do: {:error, :eacces}
  end

  defmodule CapturingPhaseWorkerSupervisor do
    def ensure_worker(role_module, repo, sprites) do
      send(self(), {:ensure_worker, role_module, repo, sprites})
      :ok
    end
  end

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.jsonl")

    fleet_path =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      label = "ready"

      [[sprite]]
      name = "bb-thorn"
      role = "fixer"
      """
    )

    stop_conductor_app()

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)
    orig_reconciler = Application.get_env(:conductor, :fleet_reconciler)
    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)

    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.put_env(:conductor, :fleet_reconciler, MockReconciler)
    Application.put_env(:conductor, :phase_worker_supervisor, FailingPhaseWorkerSupervisor)
    Application.ensure_all_started(:conductor)

    on_exit(fn ->
      stop_conductor_app()
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)
      restore_env(:fleet_reconciler, orig_reconciler)
      restore_env(:phase_worker_supervisor, orig_phase_worker_supervisor)
      File.rm(db_path)
      File.rm(event_log)
      File.rm(fleet_path)
    end)

    %{fleet_path: fleet_path}
  end

  test "maps renamed phase worker roles to sprite display names" do
    assert Conductor.Application.role_display_name(:fixer) == "thorn"
    assert Conductor.Application.role_display_name(:polisher) == "fern"
  end

  test "falls back to the raw role name for unmapped roles" do
    assert Conductor.Application.role_display_name(:builder) == "builder"
    assert Conductor.Application.role_display_name(:triage) == "triage"
  end

  test "boot_fleet logs a warning when a phase worker cannot start", %{fleet_path: fleet_path} do
    log =
      capture_log(fn ->
        assert :ok = Conductor.Application.boot_fleet(fleet_path)
      end)

    assert log =~ "[boot] thorn failed for test/repo: :eacces"
  end

  test "boot_fleet starts phase workers against the sprite repo instead of defaults repo" do
    fleet_path =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"

      [[sprite]]
      name = "bb-thorn"
      role = "fixer"
      repo = "other/repo"
      """
    )

    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
    Application.put_env(:conductor, :phase_worker_supervisor, CapturingPhaseWorkerSupervisor)

    on_exit(fn ->
      restore_env(:phase_worker_supervisor, orig_phase_worker_supervisor)
      File.rm(fleet_path)
    end)

    assert :ok = Conductor.Application.boot_fleet(fleet_path)

    assert_received {:ensure_worker, Conductor.PhaseWorker.Roles.Fixer, "other/repo",
                     ["bb-thorn"]}
  end

  test "boot_fleet starts separate phase workers for each repo" do
    fleet_path =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"

      [[sprite]]
      name = "bb-thorn-a"
      role = "fixer"
      repo = "test/repo"

      [[sprite]]
      name = "bb-thorn-b"
      role = "fixer"
      repo = "other/repo"
      """
    )

    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
    Application.put_env(:conductor, :phase_worker_supervisor, CapturingPhaseWorkerSupervisor)

    on_exit(fn ->
      restore_env(:phase_worker_supervisor, orig_phase_worker_supervisor)
      File.rm(fleet_path)
    end)

    assert :ok = Conductor.Application.boot_fleet(fleet_path)

    assert_received {:ensure_worker, Conductor.PhaseWorker.Roles.Fixer, "test/repo",
                     ["bb-thorn-a"]}

    assert_received {:ensure_worker, Conductor.PhaseWorker.Roles.Fixer, "other/repo",
                     ["bb-thorn-b"]}
  end
end
