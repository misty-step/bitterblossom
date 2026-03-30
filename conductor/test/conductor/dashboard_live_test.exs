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

    stop_conductor_app()

    start_supervised!({Phoenix.PubSub, name: Conductor.PubSub})
    start_supervised!({Conductor.Store, db_path: db_path, event_log: event_log})
    start_supervised!(Conductor.Web.Endpoint)

    on_exit(fn ->
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)
      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "dashboard mounts and renders the header" do
    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "Bitterblossom Dashboard"
  end

  test "dashboard shows empty state when no events exist" do
    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "No events yet"
  end

  test "dashboard renders events" do
    Conductor.Store.record_event("fleet", "sprite_started", %{sprite: "bb-weaver"})

    {:ok, _view, html} = live(build_conn(), "/")
    assert html =~ "sprite_started"
    assert html =~ "fleet"
  end

  test "dashboard receives PubSub update when an event is recorded" do
    {:ok, view, _html} = live(build_conn(), "/")

    assert render(view) =~ "No events yet"

    Conductor.Store.record_event("fleet", "sprite_recovered", %{sprite: "bb-thorn"})

    assert eventually(fn -> render(view) =~ "sprite_recovered" end)
  end

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
