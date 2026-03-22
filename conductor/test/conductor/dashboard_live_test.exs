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

  defmodule NoStatusWorker do
  end

  defmodule DurationIssueSource do
    def list_issues(_repo, _opts) do
      {:ok,
       [
         issue(101, "Seconds branch"),
         issue(102, "Minutes branch"),
         issue(103, "Hours branch")
       ]}
    end

    defp issue(number, title) do
      %Conductor.Issue{
        number: number,
        title: title,
        body: "",
        url: "https://github.com/test/repo/issues/#{number}",
        labels: []
      }
    end
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
    orig_phase_worker_specs = Application.get_env(:conductor, :dashboard_phase_worker_specs)
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
      restore_env(:dashboard_phase_worker_specs, orig_phase_worker_specs)
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

    # Store broadcasts a dashboard update; the LiveView re-fetches.
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

  test "dashboard refreshes governor cooldowns when run state changes" do
    {:ok, view, _html} = live(build_conn(), "/")

    assert render(view) =~ "No issues in cooldown"

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

    assert eventually(fn -> render(view) =~ "Dashboard visibility" end)
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

  test "dashboard tolerates crashing phase worker status calls" do
    pid =
      spawn(fn ->
        receive do
          _message -> exit(:boom)
        end
      end)

    Process.register(pid, Conductor.Fixer)

    try do
      {:ok, _view, html} = live(build_conn(), "/")

      assert html =~ "Phase Workers"
      assert html =~ "Thorn"
    after
      if pid = Process.whereis(Conductor.Fixer), do: Process.exit(pid, :kill)
    end
  end

  test "dashboard tolerates phase worker modules without status/0" do
    Application.put_env(:conductor, :dashboard_phase_worker_specs, [
      {NoStatusWorker, "Thorn", :fixer_sprite, "–"},
      {NoStatusWorker, "Fern", :polisher_sprite, "–"}
    ])

    {:ok, _view, html} = live(build_conn(), "/")

    assert html =~ "Phase Workers"
    assert html =~ "Thorn"
    assert html =~ "Fern"
    assert html =~ "stopped"
  end

  test "source filters query matching events before truncation" do
    insert_event!("fleet", "sprite_degraded", %{name: "bb-thorn"}, "2026-03-22T00:00:00Z")

    for index <- 1..55 do
      insert_event!(
        "run-#{index}",
        "run_progress",
        %{step: index},
        "2026-03-22T00:00:#{String.pad_leading(to_string(index), 2, "0")}Z"
      )
    end

    {:ok, view, _html} = live(build_conn(), "/")

    html =
      view
      |> element("button[phx-value-source='fleet']")
      |> render_click()

    assert html =~ "sprite_degraded"
    refute html =~ "run_progress"
  end

  test "governor renders second, minute, and hour cooldown durations" do
    Application.put_env(:conductor, :dashboard_issue_source_module, DurationIssueSource)

    insert_failed_runs!(101, 1, 100)
    insert_failed_runs!(102, 6, 1_205)
    insert_failed_runs!(103, 7, 605)

    {:ok, _view, html} = live(build_conn(), "/")

    assert html =~ "Governor"
    assert html =~ ~r/#101.*?<td>1<\/td>.*?<td>2m<\/td>.*?<td>\d+s<\/td>/s
    assert html =~ ~r/#102.*?<td>6<\/td>.*?<td>64m<\/td>.*?<td>\d+m<\/td>/s
    assert html =~ ~r/#103.*?<td>7<\/td>.*?<td>120m<\/td>.*?<td>1h \d+m<\/td>/s
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

  defp insert_event!(run_id, event_type, payload, created_at) do
    %{conn: conn} = :sys.get_state(Conductor.Store)

    {:ok, stmt} =
      Exqlite.Sqlite3.prepare(
        conn,
        "INSERT INTO events (run_id, event_type, payload, created_at) VALUES (?1, ?2, ?3, ?4)"
      )

    :ok = Exqlite.Sqlite3.bind(stmt, [run_id, event_type, Jason.encode!(payload), created_at])
    :done = Exqlite.Sqlite3.step(conn, stmt)
    :ok = Exqlite.Sqlite3.release(conn, stmt)
  end

  defp insert_failed_runs!(issue_number, count, latest_age_seconds) do
    for index <- 1..count do
      {:ok, run_id} =
        Conductor.Store.create_run(%{
          repo: "test/repo",
          issue_number: issue_number,
          issue_title: "Issue #{issue_number}",
          builder_sprite: "bb-weaver",
          run_id: "run-#{issue_number}-#{index}"
        })

      Conductor.Store.update_run(run_id, %{
        phase: "failed",
        status: "failed",
        completed_at: failed_at_iso(latest_age_seconds + count - index)
      })
    end
  end

  defp failed_at_iso(age_seconds) do
    DateTime.utc_now()
    |> DateTime.add(-age_seconds, :second)
    |> DateTime.to_iso8601()
  end
end
