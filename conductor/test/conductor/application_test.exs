defmodule Conductor.ApplicationTest do
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers

  defmodule MockReconciler do
    def reconcile_all(sprites, _opts \\ []) do
      {:ok, Enum.map(sprites, &%{name: &1.name, healthy: true, action: :none})}
    end
  end

  defmodule MockLauncher do
    def launch(sprite, repo) do
      send(Process.whereis(:application_test), {:launch, sprite.name, repo})
      {:ok, "launched"}
    end
  end

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.jsonl")

    stop_conductor_app()

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)
    orig_start_dashboard = Application.get_env(:conductor, :start_dashboard)
    orig_endpoint = Application.get_env(:conductor, Conductor.Web.Endpoint)
    orig_launcher = Application.get_env(:conductor, :launcher_module)
    orig_reconciler = Application.get_env(:conductor, :fleet_reconciler)

    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.ensure_all_started(:conductor)
    Process.register(self(), :application_test)

    on_exit(fn ->
      stop_conductor_app()
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)
      restore_env(:start_dashboard, orig_start_dashboard)
      restore_env(:launcher_module, orig_launcher)
      restore_env(:fleet_reconciler, orig_reconciler)

      if orig_endpoint do
        Application.put_env(:conductor, Conductor.Web.Endpoint, orig_endpoint)
      else
        Application.delete_env(:conductor, Conductor.Web.Endpoint)
      end

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
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

  test "launch_fleet uses each sprite's resolved repo" do
    fleet_path =
      Path.join(System.tmp_dir!(), "application_test_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "default/repo"

      [[sprite]]
      name = "bb-builder"
      role = "builder"

      [[sprite]]
      name = "bb-fixer"
      role = "fixer"
      repo = "other/repo"
      """
    )

    Application.put_env(:conductor, :fleet_reconciler, MockReconciler)
    Application.put_env(:conductor, :launcher_module, MockLauncher)

    try do
      assert :ok = Conductor.Application.launch_fleet(fleet_path)
      assert_receive {:launch, "bb-builder", "default/repo"}
      assert_receive {:launch, "bb-fixer", "other/repo"}
    after
      File.rm(fleet_path)
    end
  end
end
