defmodule Conductor.StoreTest do
  use ExUnit.Case, async: false

  alias Conductor.Store

  setup do
    # Use a temp DB for each test
    db_path = Path.join(System.tmp_dir!(), "conductor_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "conductor_test_#{:rand.uniform(999_999)}.jsonl")

    # Stop any existing store
    if Process.whereis(Store), do: GenServer.stop(Store)

    {:ok, _pid} = Store.start_link(db_path: db_path, event_log: event_log)

    on_exit(fn ->
      case Process.whereis(Store) do
        nil ->
          :ok

        pid when is_pid(pid) ->
          if Process.alive?(pid), do: GenServer.stop(Store), else: :ok
      end

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

    Store.update_run(run_id, %{phase: "building", branch: "factory/1-123"})

    {:ok, run} = Store.get_run(run_id)
    assert run["phase"] == "building"
    assert run["branch"] == "factory/1-123"
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
end
