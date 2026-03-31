defmodule Conductor.StoreTest do
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Store

  setup do
    db_path = Path.join(System.tmp_dir!(), "conductor_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "conductor_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()

    stop_process(Store)

    {:ok, _pid} = Store.start_link(db_path: db_path, event_log: event_log)

    on_exit(fn ->
      stop_process(Store)

      File.rm(db_path)
      File.rm(event_log)
    end)

    %{db_path: db_path, event_log: event_log}
  end

  test "record and list events" do
    Store.record_event("fleet", "sprite_started", %{sprite: "bb-weaver"})
    Store.record_event("fleet", "sprite_healthy", %{sprite: "bb-weaver"})

    events = Store.list_events("fleet")
    assert length(events) == 2
    assert hd(events)["event_type"] == "sprite_started"
    assert List.last(events)["event_type"] == "sprite_healthy"
  end

  describe "list_all_events/1" do
    test "returns empty list when no events" do
      assert [] = Store.list_all_events()
    end

    test "lists events across sources, newest first" do
      Store.record_event("bb-weaver", "started", %{a: 1})
      Store.record_event("bb-thorn", "dispatched", %{pr: 100})
      Store.record_event("bb-fern", "completed", %{b: 2})

      events = Store.list_all_events(limit: 10)
      assert length(events) == 3
      assert hd(events)["event_type"] == "completed"
    end

    test "respects limit" do
      for i <- 1..5, do: Store.record_event("s-#{i}", "ev", %{i: i})

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
