defmodule Conductor.StoreTest do
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Store

  setup do
    # Use a temp DB for each test
    db_path = Path.join(System.tmp_dir!(), "conductor_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "conductor_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()

    # Stop any existing store before claiming the global name.
    stop_process(Store)

    {:ok, _pid} = Store.start_link(db_path: db_path, event_log: event_log)

    on_exit(fn ->
      stop_process(Store)

      File.rm(db_path)
      File.rm(event_log)
    end)

    %{db_path: db_path, event_log: event_log}
  end

  test "create and retrieve a run" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 42,
        issue_title: "Test issue",
        builder_sprite: "test-sprite"
      })

    assert String.starts_with?(run_id, "run-42-")

    {:ok, run} = Store.get_run(run_id)
    assert run["repo"] == "test/repo"
    assert run["issue_number"] == 42
    assert run["phase"] == "pending"
    assert run["status"] == "pending"
    assert run["builder_sprite"] == "test-sprite"
  end

  test "update run fields" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 1,
        issue_title: "test",
        builder_sprite: "s"
      })

    Store.update_run(run_id, %{
      phase: "building",
      branch: "factory/1-123",
      dispatch_attempt_count: 2,
      builder_failure_class: "transient",
      builder_failure_reason: "network timeout"
    })

    {:ok, run} = Store.get_run(run_id)
    assert run["phase"] == "building"
    assert run["branch"] == "factory/1-123"
    assert run["dispatch_attempt_count"] == 2
    assert run["builder_failure_class"] == "transient"
    assert run["builder_failure_reason"] == "network timeout"
  end

  test "complete run sets terminal state" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 2,
        issue_title: "test",
        builder_sprite: "s"
      })

    Store.complete_run(run_id, "merged", "merged")

    {:ok, run} = Store.get_run(run_id)
    assert run["phase"] == "merged"
    assert run["status"] == "merged"
    assert run["completed_at"] != nil
  end

  test "lease prevents double-leasing" do
    :ok = Store.acquire_lease("test/repo", 10, "run-10-1")
    assert {:error, :already_leased} = Store.acquire_lease("test/repo", 10, "run-10-2")
    assert Store.leased?("test/repo", 10)

    Store.release_lease("test/repo", 10)
    refute Store.leased?("test/repo", 10)

    :ok = Store.acquire_lease("test/repo", 10, "run-10-3")
    assert Store.leased?("test/repo", 10)
  end

  test "record and list events" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 3,
        issue_title: "test",
        builder_sprite: "s"
      })

    Store.record_event(run_id, "lease_acquired", %{issue: 3})
    Store.record_event(run_id, "builder_dispatched", %{sprite: "s"})

    events = Store.list_events(run_id)
    assert length(events) == 2
    assert hd(events)["event_type"] == "lease_acquired"
    assert List.last(events)["event_type"] == "builder_dispatched"
  end

  test "list runs returns most recent first" do
    for i <- 1..3 do
      Store.create_run(%{
        repo: "test/repo",
        issue_number: i,
        issue_title: "issue #{i}",
        builder_sprite: "s"
      })

      Process.sleep(10)
    end

    runs = Store.list_runs(limit: 2)
    assert length(runs) == 2
    assert hd(runs)["issue_number"] == 3
  end

  test "heartbeat updates timestamp" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 4,
        issue_title: "test",
        builder_sprite: "s"
      })

    {:ok, run_before} = Store.get_run(run_id)
    Process.sleep(10)
    Store.heartbeat_run(run_id)
    {:ok, run_after} = Store.get_run(run_id)

    assert run_after["heartbeat_at"] >= run_before["heartbeat_at"]
  end

  test "get_run returns error for missing run" do
    assert {:error, :not_found} = Store.get_run("nonexistent")
  end

  test "mark_semantic_ready sets semantic_ready flag on run" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 5,
        issue_title: "test",
        builder_sprite: "s"
      })

    {:ok, run_before} = Store.get_run(run_id)
    assert is_nil(run_before["semantic_ready"])

    Store.mark_semantic_ready(run_id)
    {:ok, run_after} = Store.get_run(run_id)
    assert run_after["semantic_ready"] == 1
  end

  test "record and list incidents" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 6,
        issue_title: "test",
        builder_sprite: "s"
      })

    Store.record_incident(run_id, %{
      check_name: "cerberus",
      failure_class: "known_false_red",
      signature: "cerberus:FAILURE"
    })

    Store.record_incident(run_id, %{
      check_name: "e2e-timeout",
      failure_class: "transient_infra",
      signature: "e2e-timeout:FAILURE"
    })

    incidents = Store.list_incidents(run_id)
    assert length(incidents) == 2
    assert hd(incidents)["check_name"] == "cerberus"
    assert hd(incidents)["failure_class"] == "known_false_red"
    assert List.last(incidents)["failure_class"] == "transient_infra"
  end

  test "record and list waivers" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 7,
        issue_title: "test",
        builder_sprite: "s"
      })

    Store.record_waiver(run_id, %{
      check_name: "cerberus",
      rationale: "known false-red on trusted surface cerberus; semantic review clean"
    })

    waivers = Store.list_waivers(run_id)
    assert length(waivers) == 1
    assert hd(waivers)["check_name"] == "cerberus"
    assert String.contains?(hd(waivers)["rationale"], "known false-red")
    assert hd(waivers)["waived_at"] != nil
  end

  test "incidents and waivers are isolated per run" do
    {:ok, run_a} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 8,
        issue_title: "a",
        builder_sprite: "s"
      })

    {:ok, run_b} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 9,
        issue_title: "b",
        builder_sprite: "s"
      })

    Store.record_incident(run_a, %{
      check_name: "cerberus",
      failure_class: "known_false_red",
      signature: "x"
    })

    assert Store.list_incidents(run_a) |> length() == 1
    assert Store.list_incidents(run_b) |> length() == 0
    assert Store.list_waivers(run_a) |> length() == 0
  end

  test "dispatch pause flag defaults false and can be toggled" do
    refute Store.dispatch_paused?()

    :ok = Store.set_dispatch_paused(true)
    assert Store.dispatch_paused?()

    :ok = Store.set_dispatch_paused(false)
    refute Store.dispatch_paused?()
  end

  test "terminate_run atomically completes run and releases lease" do
    {:ok, run_id} =
      Store.create_run(%{
        repo: "test/repo",
        issue_number: 77,
        issue_title: "atomic test",
        builder_sprite: "sprite-1"
      })

    :ok = Store.acquire_lease("test/repo", 77, run_id)
    assert Store.leased?("test/repo", 77)

    :ok = Store.terminate_run(run_id, "failed", "failed", "test/repo", 77)

    {:ok, run} = Store.get_run(run_id)
    assert run["phase"] == "failed"
    assert run["completed_at"] != nil
    refute Store.leased?("test/repo", 77)
  end

  test "find_run_by_pr returns an error tuple when the database query fails" do
    :sys.replace_state(Store, fn state -> %{state | conn: :invalid} end)

    assert {:error, {:db_error, _}} = Store.find_run_by_pr("test/repo", 123)
  end

  test "tracks PR polish timestamps independently of runs" do
    assert {:error, :not_found} = Store.get_pr_state("test/repo", 42)

    assert :ok =
             Store.upsert_pr_state("test/repo", 42, %{
               last_substantive_change_at: "2026-03-20T12:00:00Z"
             })

    assert {:ok, pr} = Store.get_pr_state("test/repo", 42)
    assert pr["repo"] == "test/repo"
    assert pr["pr_number"] == 42
    assert pr["last_substantive_change_at"] == "2026-03-20T12:00:00Z"
    assert is_nil(pr["polished_at"])

    assert :ok = Store.mark_pr_polished("test/repo", 42, "2026-03-20T12:30:00Z")

    assert {:ok, pr} = Store.get_pr_state("test/repo", 42)
    assert pr["polished_at"] == "2026-03-20T12:30:00Z"

    assert :ok =
             Store.upsert_pr_state("test/repo", 42, %{
               last_substantive_change_at: "2026-03-20T13:00:00Z"
             })

    assert {:ok, pr} = Store.get_pr_state("test/repo", 42)
    assert pr["polished_at"] == "2026-03-20T12:30:00Z"
    assert pr["last_substantive_change_at"] == "2026-03-20T13:00:00Z"
  end

  test "rejects invalid PR state columns" do
    assert {:error, :invalid_column} =
             Store.upsert_pr_state("test/repo", 42, %{bogus: "value"})
  end

  describe "issue_failure_streak/2" do
    test "returns zero streak when no runs exist" do
      assert {0, nil} = Store.issue_failure_streak("test/repo", 999)
    end

    test "counts consecutive failures" do
      for i <- 1..3 do
        {:ok, rid} =
          Store.create_run(%{
            repo: "test/repo",
            issue_number: 50,
            issue_title: "t",
            builder_sprite: "s",
            run_id: "fail-#{i}"
          })

        Store.complete_run(rid, "failed", "failed")
      end

      {streak, last} = Store.issue_failure_streak("test/repo", 50)
      assert streak == 3
      assert is_binary(last)
    end

    test "resets streak after success" do
      {:ok, r1} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 60,
          issue_title: "t",
          builder_sprite: "s",
          run_id: "f1"
        })

      Store.complete_run(r1, "failed", "failed")

      {:ok, r2} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 60,
          issue_title: "t",
          builder_sprite: "s",
          run_id: "s1"
        })

      Store.complete_run(r2, "pr_opened", "pr_opened")

      {:ok, r3} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 60,
          issue_title: "t",
          builder_sprite: "s",
          run_id: "f2"
        })

      Store.complete_run(r3, "failed", "failed")

      {streak, _} = Store.issue_failure_streak("test/repo", 60)
      assert streak == 1
    end

    test "different repos are independent" do
      {:ok, r1} =
        Store.create_run(%{
          repo: "repo/a",
          issue_number: 70,
          issue_title: "t",
          builder_sprite: "s",
          run_id: "ra1"
        })

      Store.complete_run(r1, "failed", "failed")

      {:ok, r2} =
        Store.create_run(%{
          repo: "repo/b",
          issue_number: 70,
          issue_title: "t",
          builder_sprite: "s",
          run_id: "rb1"
        })

      Store.complete_run(r2, "failed", "failed")

      {streak_a, _} = Store.issue_failure_streak("repo/a", 70)
      {streak_b, _} = Store.issue_failure_streak("repo/b", 70)
      assert streak_a == 1
      assert streak_b == 1
    end
  end

  describe "list_all_events/1" do
    test "returns empty list when no events" do
      assert [] = Store.list_all_events()
    end

    test "lists events across runs, newest first" do
      Store.record_event("run-1", "started", %{a: 1})
      Store.record_event("polisher", "polisher_dispatched", %{pr: 100})
      Store.record_event("run-2", "completed", %{b: 2})

      events = Store.list_all_events(limit: 10)
      assert length(events) == 3
      # Newest first
      assert hd(events)["event_type"] == "completed"
    end

    test "respects limit" do
      for i <- 1..5, do: Store.record_event("r-#{i}", "ev", %{i: i})

      events = Store.list_all_events(limit: 3)
      assert length(events) == 3
    end

    test "decodes payload JSON" do
      Store.record_event("r", "test", %{key: "val"})

      [event] = Store.list_all_events(limit: 1)
      assert event["payload"]["key"] == "val"
    end
  end
end
