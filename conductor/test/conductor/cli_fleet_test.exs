defmodule Conductor.CLIFleetTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO

  alias Conductor.{CLI, Store}

  defmodule MockWorker do
    def status("bb-builder-1", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-builder-1",
           reachable: true,
           harness_ready: true,
           gh_authenticated: true,
           git_credential_helper: true,
           healthy: true
         }}

    def status("bb-builder-2", _opts), do: {:error, "connection refused"}

    def status("bb-builder-3", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-builder-3",
           reachable: true,
           harness_ready: true,
           gh_authenticated: false,
           git_credential_helper: true,
           healthy: false
         }}

    def status("bb-builder-4", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-builder-4",
           reachable: true,
           harness_ready: true,
           gh_authenticated: true,
           git_credential_helper: false,
           healthy: false
         }}
  end

  defmodule ProbeOnlyWorker do
    def probe("bb-builder-1", _opts), do: {:ok, %{sprite: "bb-builder-1", reachable: true}}
    def probe(_worker, _opts), do: {:error, "connection refused"}
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
      name = "bb-builder-1"
      role = "builder"
      capability_tags = ["elixir"]

      [[sprite]]
      name = "bb-builder-2"
      role = "builder"

      [[sprite]]
      name = "bb-builder-3"
      role = "builder"

      [[sprite]]
      name = "bb-builder-4"
      role = "builder"
      """
    )

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)
    orig_worker = Application.get_env(:conductor, :worker_module)

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
        builder_sprite: "bb-builder-1"
      })

    Store.update_run(run_id, %{phase: "building"})

    on_exit(fn ->
      Application.stop(:conductor)
      Application.put_env(:conductor, :db_path, orig_db)
      Application.put_env(:conductor, :event_log, orig_log)

      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)

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

    assert output =~ "bb-builder-1"
    assert output =~ "healthy"
    assert output =~ "issue #622"
    assert output =~ "tags=elixir"

    assert output =~ "bb-builder-2"
    assert output =~ "unreachable"
    assert output =~ "idle"

    assert output =~ "bb-builder-3"
    assert output =~ "needs setup (gh auth missing)"

    assert output =~ "bb-builder-4"
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

      assert output =~ "bb-builder-1"
      assert output =~ "healthy"
    after
      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)
    end
  end

  test "deprecated loop command works without a label and leaves the filter unset" do
    stderr =
      capture_io(:stderr, fn ->
        assert :ok =
                 CLI.loop_command(
                   ["--repo", "test/repo", "--worker", "bb-builder-1"],
                   wait: false
                 )
      end)

    assert stderr =~ "loop is deprecated"
    assert :sys.get_state(Conductor.Orchestrator).label == nil
  end

  test "deprecated loop command still accepts label as an optional narrowing filter" do
    stderr =
      capture_io(:stderr, fn ->
        assert :ok =
                 CLI.loop_command(
                   ["--repo", "test/repo", "--worker", "bb-builder-1", "--label", "hold"],
                   wait: false
                 )
      end)

    assert stderr =~ "loop is deprecated"
    assert stderr =~ "--label is deprecated as a backlog gate"
    assert :sys.get_state(Conductor.Orchestrator).label == "hold"
  end
end
