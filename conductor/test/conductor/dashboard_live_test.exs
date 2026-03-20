defmodule Conductor.Web.DashboardLiveTest do
  @moduledoc """
  Tests for the operator dashboard LiveView.
  """
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers
  import Phoenix.ConnTest
  import Phoenix.LiveViewTest

  @endpoint Conductor.Web.Endpoint

  setup do
    db_path = Path.join(System.tmp_dir!(), "dash_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "dash_test_#{:rand.uniform(999_999)}.jsonl")

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)

    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)

    Application.put_env(:conductor, Conductor.Web.Endpoint,
      adapter: Bandit.PhoenixAdapter,
      http: [ip: {127, 0, 0, 1}, port: 0],
      secret_key_base: String.duplicate("a", 64),
      live_view: [signing_salt: "test0000"],
      server: true,
      check_origin: false
    )

    # App-owning tests can leave globally named services running between modules.
    stop_conductor_app()

    start_supervised!({Phoenix.PubSub, name: Conductor.PubSub})
    start_supervised!({Conductor.Store, db_path: db_path, event_log: event_log})
    start_supervised!(Conductor.Web.Endpoint)

    on_exit(fn ->
      Application.put_env(:conductor, :db_path, orig_db)
      Application.put_env(:conductor, :event_log, orig_log)
      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "dashboard mounts and renders the header" do
    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "Bitterblossom Dashboard"
  end

  test "dashboard shows empty run table when no runs exist" do
    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "No runs yet"
  end

  test "dashboard shows run stats at zero on empty store" do
    {:ok, _view, html} = live(build_conn(), "/")
    # All stat values should show 0
    assert html =~ ~r/stat-value[^>]*>0/
  end

  test "dashboard renders a run in the table" do
    {:ok, _run_id} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 42,
        issue_title: "Add dashboard LiveView",
        builder_sprite: "noble-blue-serpent"
      })

    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "#42"
    assert html =~ "Add dashboard LiveView"
    assert html =~ "noble-blue-serpent"
    assert html =~ "test/repo"
  end

  test "dashboard receives PubSub update when a run is created" do
    {:ok, view, _html} = live(build_conn(), "/")

    # Initially empty
    assert render(view) =~ "No runs yet"

    {:ok, _run_id} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 99,
        issue_title: "PubSub update test",
        builder_sprite: "sprite-1"
      })

    # Store broadcasts :runs_updated; the LiveView re-fetches
    assert eventually(fn -> render(view) =~ "#99" end)
  end

  test "dashboard stats count active runs" do
    {:ok, run_id} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 10,
        issue_title: "active run",
        builder_sprite: "s"
      })

    Conductor.Store.update_run(run_id, %{phase: "building"})

    {:ok, _view, html} = live(build_conn(), "/")
    # One active run — the Active stat counter should be `1`
    assert html =~ ~r/stat-label[^>]*>Active<\/div>\s*<div class="stat-value">1</
  end

  test "dashboard counts blocked runs in stats" do
    {:ok, run_id} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 11,
        issue_title: "blocked run",
        builder_sprite: "s"
      })

    Conductor.Store.update_run(run_id, %{status: "blocked"})

    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "Blocked"
  end

  # Poll helper for async assertions
  defp eventually(fun, retries \\ 10) do
    if fun.() do
      true
    else
      if retries > 0 do
        Process.sleep(50)
        eventually(fun, retries - 1)
      else
        false
      end
    end
  end
end
