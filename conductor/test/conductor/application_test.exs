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
    orig_start_dashboard = Application.get_env(:conductor, :start_dashboard)
    orig_endpoint = Application.get_env(:conductor, Conductor.Web.Endpoint)

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
      restore_env(:start_dashboard, orig_start_dashboard)

      if orig_endpoint do
        Application.put_env(:conductor, Conductor.Web.Endpoint, orig_endpoint)
      else
        Application.delete_env(:conductor, Conductor.Web.Endpoint)
      end

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

    assert log =~ "[boot] thorn failed: :eacces"
  end

  test "application boot does not auto-start the dashboard endpoint" do
    Application.put_env(:conductor, :start_dashboard, true)

    refute Enum.any?(Supervisor.which_children(Conductor.Supervisor), fn {id, _, _, _} ->
             id == Conductor.Web.Endpoint
           end)
  end

  test "start_dashboard/0 starts the dashboard endpoint when enabled" do
    Application.put_env(:conductor, :start_dashboard, true)

    Application.put_env(:conductor, Conductor.Web.Endpoint,
      adapter: Bandit.PhoenixAdapter,
      http: [ip: {127, 0, 0, 1}, port: 0],
      secret_key_base:
        System.get_env("DASHBOARD_SECRET_KEY_BASE") ||
          "bitterblossom-dashboard-dev-key-must-be-at-least-64-chars-long-x",
      live_view: [signing_salt: "bb_lv_salt"],
      server: true
    )

    assert :ok = Conductor.Application.start_dashboard()

    assert Enum.any?(Supervisor.which_children(Conductor.Supervisor), fn {id, _, _, _} ->
             id == Conductor.Web.Endpoint
           end)
  end
end
