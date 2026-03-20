defmodule Conductor.RunReconcilerTest do
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{RunReconciler, Store}

  setup do
    db_path = Path.join(System.tmp_dir!(), "reconcile_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "reconcile_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(Store)

    {:ok, _pid} = Store.start_link(db_path: db_path, event_log: event_log)

    orig_stale = Application.get_env(:conductor, :stale_run_threshold_minutes)
    Application.put_env(:conductor, :stale_run_threshold_minutes, 60)

    on_exit(fn ->
      stop_process(Store)

      if orig_stale,
        do: Application.put_env(:conductor, :stale_run_threshold_minutes, orig_stale),
        else: Application.delete_env(:conductor, :stale_run_threshold_minutes)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "expires stale runs and releases their leases" do
    old_heartbeat = DateTime.utc_now() |> DateTime.add(-7_200, :second) |> DateTime.to_iso8601()

    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 42,
        issue_title: "stale startup run",
        builder_sprite: "sprite-1"
      })

    Store.update_run(run_id, %{heartbeat_at: old_heartbeat})
    :ok = Store.acquire_lease("test/repo", 42, run_id)

    assert :ok == RunReconciler.reconcile_stale_runs("test/repo")

    {:ok, run} = Store.get_run(run_id)
    assert run["phase"] == "failed"
    assert run["status"] == "failed"
    assert run["completed_at"] != nil
    refute Store.leased?("test/repo", 42)

    event_types = Store.list_events(run_id) |> Enum.map(& &1["event_type"])
    assert "stale_run_detected" in event_types
  end

  test "keeps live in-memory runs excluded from stale expiry" do
    old_heartbeat = DateTime.utc_now() |> DateTime.add(-7_200, :second) |> DateTime.to_iso8601()

    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 43,
        issue_title: "active run",
        builder_sprite: "sprite-1"
      })

    Store.update_run(run_id, %{heartbeat_at: old_heartbeat})
    :ok = Store.acquire_lease("test/repo", 43, run_id)

    assert :ok ==
             RunReconciler.reconcile_stale_runs("test/repo", active_issue_numbers: [43])

    {:ok, run} = Store.get_run(run_id)
    assert run["completed_at"] == nil
    assert Store.leased?("test/repo", 43)
  end
end
