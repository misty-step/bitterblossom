defmodule Conductor.Web.DashboardLiveTest do
  @moduledoc """
  Tests for the operator dashboard LiveView.
  """
  use ExUnit.Case, async: false

  alias Conductor.Fleet.HealthMonitor

  import Conductor.TestSupport.ProcessHelpers
  import Phoenix.ConnTest
  import Phoenix.LiveViewTest

  @endpoint Conductor.Web.Endpoint

  defmodule MockIssueSource do
    def list_issues(_repo, _opts) do
      {:ok,
       [
         %Conductor.Issue{
           number: 779,
           title: "Dashboard visibility",
           body: "",
           url: "https://github.com/test/repo/issues/779",
           labels: []
         },
         %Conductor.Issue{
           number: 780,
           title: "Muse",
           body: "",
           url: "https://github.com/test/repo/issues/780",
           labels: []
         }
       ]}
    end
  end

  defmodule ErrorIssueSource do
    def list_issues(_repo, _opts), do: {:error, :timeout}
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
    db_path = Path.join(System.tmp_dir!(), "dash_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "dash_test_#{:rand.uniform(999_999)}.jsonl")

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)
    orig_issue_source = Application.get_env(:conductor, :dashboard_issue_source_module)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_start_dashboard = Application.get_env(:conductor, :start_dashboard)

    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.put_env(:conductor, :dashboard_issue_source_module, MockIssueSource)
    Application.put_env(:conductor, :worker_module, NoopWorker)
    Application.put_env(:conductor, :workspace_module, NoopWorkspace)
    Application.put_env(:conductor, :code_host_module, NoopCodeHost)
    Application.put_env(:conductor, :start_dashboard, false)

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
      stop_process(Conductor.Polisher)
      stop_process(Conductor.Fixer)
      stop_process(HealthMonitor)
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)
      restore_env(:dashboard_issue_source_module, orig_issue_source)
      restore_env(:worker_module, orig_worker)
      restore_env(:workspace_module, orig_workspace)
      restore_env(:code_host_module, orig_code_host)
      restore_env(:start_dashboard, orig_start_dashboard)
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

  test "dashboard renders operator panels with live runtime state" do
    start_supervised!({HealthMonitor, interval_ms: 60_000})

    HealthMonitor.configure(
      repo: "test/repo",
      sprites: [
        %{name: "bb-weaver", role: :builder, harness: "codex"},
        %{name: "bb-thorn", role: :fixer, harness: "codex"},
        %{name: "bb-fern", role: :polisher, harness: "codex"}
      ],
      healthy: MapSet.new(["bb-weaver", "bb-thorn", "bb-fern"])
    )

    start_supervised!(
      {Conductor.Fixer, repo: "test/repo", fixer_sprite: "bb-thorn", poll_ms: 60_000}
    )

    start_supervised!(
      {Conductor.Polisher, repo: "test/repo", polisher_sprite: "bb-fern", poll_ms: 60_000}
    )

    {:ok, failed_run} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 779,
        issue_title: "Dashboard visibility",
        builder_sprite: "bb-weaver"
      })

    Conductor.Store.update_run(failed_run, %{
      phase: "failed",
      status: "failed",
      completed_at: DateTime.utc_now() |> DateTime.to_iso8601()
    })

    {:ok, merged_run} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 780,
        issue_title: "Muse synthesis",
        builder_sprite: "bb-weaver"
      })

    Conductor.Store.update_run(merged_run, %{
      phase: "merged",
      status: "merged",
      completed_at: DateTime.utc_now() |> DateTime.to_iso8601()
    })

    Conductor.Store.record_event("fleet", "sprite_degraded", %{name: "bb-thorn"})
    Conductor.Store.record_event("fixer", "fixer_failed", %{pr_number: 42})

    {:ok, view, html} = live(build_conn(), "/")

    assert html =~ "Fleet Health"
    assert html =~ "Phase Workers"
    assert html =~ "Governor"
    assert html =~ "Recent Events"
    assert html =~ "Run Timeline"
    assert html =~ "bb-thorn"
    assert html =~ "bb-fern"
    assert html =~ "#779"
    assert html =~ "sprite_degraded"
    assert html =~ "fixer_failed"

    html =
      view
      |> element("button[phx-value-source='fleet']")
      |> render_click()

    assert html =~ "sprite_degraded"
    refute html =~ "fixer_failed"
  end

  test "dashboard refreshes event stream when a new event is recorded" do
    {:ok, view, _html} = live(build_conn(), "/")

    refute render(view) =~ "sprite_recovered"

    Conductor.Store.record_event("fleet", "sprite_recovered", %{name: "bb-thorn"})

    assert eventually(fn -> render(view) =~ "sprite_recovered" end)
  end

  test "governor cooldowns fall back to empty when issue loading fails" do
    Application.put_env(:conductor, :dashboard_issue_source_module, ErrorIssueSource)

    {:ok, failed_run} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 779,
        issue_title: "Dashboard visibility",
        builder_sprite: "bb-weaver"
      })

    Conductor.Store.update_run(failed_run, %{
      phase: "failed",
      status: "failed",
      completed_at: DateTime.utc_now() |> DateTime.to_iso8601()
    })

    {:ok, _view, html} = live(build_conn(), "/")

    assert html =~ "Governor"
    assert html =~ "No issues in cooldown"
  end

  test "governor ignores cooldown entries with invalid timestamps" do
    {:ok, failed_run} =
      Conductor.Store.create_run(%{
        repo: "test/repo",
        issue_number: 779,
        issue_title: "Dashboard visibility",
        builder_sprite: "bb-weaver"
      })

    Conductor.Store.update_run(failed_run, %{
      phase: "failed",
      status: "failed",
      completed_at: "not-a-timestamp"
    })

    {:ok, _view, html} = live(build_conn(), "/")

    assert html =~ "Governor"
    assert html =~ "No issues in cooldown"
  end

  test "source filters query matching events before truncation" do
    Conductor.Store.record_event("fleet", "sprite_degraded", %{name: "bb-thorn"})
    Process.sleep(1_100)

    for index <- 1..55 do
      {:ok, run_id} =
        Conductor.Store.create_run(%{
          repo: "test/repo",
          issue_number: 800 + index,
          issue_title: "Run event #{index}",
          builder_sprite: "bb-weaver"
        })

      Conductor.Store.record_event(run_id, "run_progress", %{step: index})
    end

    {:ok, view, _html} = live(build_conn(), "/")

    html =
      view
      |> element("button[phx-value-source='fleet']")
      |> render_click()

    assert html =~ "sprite_degraded"
    refute html =~ "run_progress"
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
