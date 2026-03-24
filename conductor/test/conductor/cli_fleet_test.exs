defmodule Conductor.CLIFleetTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{CLI, Store}

  @conductor_dir Path.expand("../..", __DIR__)

  defmodule MockWorker do
    def status("bb-weaver-1", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-weaver-1",
           reachable: true,
           harness_ready: true,
           gh_authenticated: true,
           git_credential_helper: true,
           healthy: true
         }}

    def status("bb-weaver-2", _opts), do: {:error, "connection refused"}

    def status("bb-weaver-3", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-weaver-3",
           reachable: true,
           harness_ready: true,
           gh_authenticated: false,
           git_credential_helper: true,
           healthy: false
         }}

    def status("bb-weaver-4", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-weaver-4",
           reachable: true,
           harness_ready: true,
           gh_authenticated: true,
           git_credential_helper: false,
           healthy: false
         }}
  end

  defmodule ProbeOnlyWorker do
    def probe("bb-weaver-1", _opts), do: {:ok, %{sprite: "bb-weaver-1", reachable: true}}
    def probe(_worker, _opts), do: {:error, "connection refused"}
  end

  defmodule MockReconciler do
    def reconcile_all(sprites, _opts \\ []) do
      send(self(), {:reconciled, Enum.map(sprites, & &1.name)})
      {:ok, Enum.map(sprites, &%{name: &1.name, healthy: true, action: :provisioned})}
    end
  end

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.jsonl")

    fleet_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"

      [[sprite]]
      name = "bb-weaver-1"
      role = "builder"
      capability_tags = ["elixir"]

      [[sprite]]
      name = "bb-weaver-2"
      role = "builder"

      [[sprite]]
      name = "bb-weaver-3"
      role = "builder"

      [[sprite]]
      name = "bb-weaver-4"
      role = "builder"
      """
    )

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_reconciler = Application.get_env(:conductor, :fleet_reconciler)
    Application.stop(:conductor)
    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.ensure_all_started(:conductor)

    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 622,
        issue_title: "fleet status",
        builder_sprite: "bb-weaver-1"
      })

    Store.update_run(run_id, %{phase: "building"})

    on_exit(fn ->
      Application.stop(:conductor)
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)

      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)

      if orig_reconciler,
        do: Application.put_env(:conductor, :fleet_reconciler, orig_reconciler),
        else: Application.delete_env(:conductor, :fleet_reconciler)

      Application.ensure_all_started(:conductor)

      File.rm(db_path)
      File.rm(event_log)
      File.rm(fleet_path)
    end)

    %{fleet_path: fleet_path}
  end

  test "fleet prints declared workers with health and assignment status", %{
    fleet_path: fleet_path
  } do
    output =
      capture_io(fn ->
        CLI.main(["fleet", "--fleet", fleet_path])
      end)

    assert output =~ "bb-weaver-1"
    assert output =~ "healthy"
    assert output =~ "issue #622"
    assert output =~ "tags=elixir"

    assert output =~ "bb-weaver-2"
    assert output =~ "unreachable"
    assert output =~ "idle"

    assert output =~ "bb-weaver-3"
    assert output =~ "needs setup (gh auth missing)"

    assert output =~ "bb-weaver-4"
    assert output =~ "needs setup (git helper missing)"
  end

  test "fleet keeps probe-only workers healthy", %{fleet_path: fleet_path} do
    orig_worker = Application.get_env(:conductor, :worker_module)
    Application.put_env(:conductor, :worker_module, ProbeOnlyWorker)

    try do
      output =
        capture_io(fn ->
          CLI.main(["fleet", "--fleet", fleet_path])
        end)

      assert output =~ "bb-weaver-1"
      assert output =~ "healthy"
    after
      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)
    end
  end

  test "configured reconciler receives the declared fleet sprites", %{fleet_path: fleet_path} do
    {:ok, config} = Conductor.Fleet.Loader.load(fleet_path)
    {:ok, _results} = MockReconciler.reconcile_all(config.sprites)

    assert_received {:reconciled, ["bb-weaver-1", "bb-weaver-2", "bb-weaver-3", "bb-weaver-4"]}
  end

  test "mix conductor fleet --reconcile fails with environment preflight output", %{
    fleet_path: fleet_path
  } do
    codex_home =
      Path.join(System.tmp_dir!(), "codex_home_#{System.unique_integer([:positive])}")

    File.mkdir_p!(codex_home)

    {output, status} =
      System.cmd("mix", ["conductor", "fleet", "--fleet", fleet_path, "--reconcile"],
        cd: @conductor_dir,
        env: [
          {"MIX_ENV", "test"},
          {"GITHUB_TOKEN", ""},
          {"OPENAI_API_KEY", ""},
          {"CODEX_HOME", codex_home},
          {"SPRITE_TOKEN", "sprite-test"}
        ],
        stderr_to_stdout: true
      )

    File.rm_rf(codex_home)

    assert status == 1
    assert output =~ "environment check failed: missing: GITHUB_TOKEN"
    assert output =~ "Codex ChatGPT auth cache or OPENAI_API_KEY"
  end
end
