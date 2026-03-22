defmodule Conductor.Fleet.ReconcilerTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Fleet.Reconciler
  alias Conductor.Store

  @sprite %{
    name: "bb-weaver",
    role: "builder",
    repo: "misty-step/bitterblossom",
    persona: "You are Weaver.",
    harness: "codex"
  }

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "reconciler_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "reconciler_test_#{System.unique_integer([:positive])}.jsonl")

    stop_conductor_app()
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    on_exit(fn ->
      stop_process(Store)
      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  test "reconcile_sprite marks unreachable sprites degraded without provisioning" do
    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:error, "timeout"} end,
        wake_fn: fn _name, _opts -> {:error, "still down"} end,
        max_attempts: 1,
        sleep_fn: fn _ -> :ok end,
        provision_fn: fn _name, _opts -> flunk("provision_fn should not be called") end
      )

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :unreachable}
  end

  test "reconcile_sprite wakes an unreachable sprite before marking it degraded" do
    test_pid = self()
    status_calls = :atomics.new(1, [])

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts ->
          case :atomics.add_get(status_calls, 1, 1) do
            1 -> {:error, "websocket: bad handshake (HTTP 502)"}
            2 -> {:ok, %{healthy: true}}
          end
        end,
        wake_fn: fn sprite, opts ->
          send(test_pid, {:wake_called, sprite, opts})
          :ok
        end,
        sleep_fn: fn _ -> :ok end
      )

    assert_received {:wake_called, "bb-weaver", wake_opts}
    assert wake_opts[:harness] == "codex"
    assert result == %{name: "bb-weaver", role: "builder", healthy: true, action: :woken}
  end

  test "reconcile_sprite logs and records a fleet event after recovery retries are exhausted" do
    log =
      capture_log(fn ->
        result =
          Reconciler.reconcile_sprite(@sprite,
            status_fn: fn _name, _opts -> {:error, "websocket: bad handshake (HTTP 502)"} end,
            wake_fn: fn _name, _opts -> {:error, "start failed"} end,
            max_attempts: 2,
            sleep_fn: fn _ -> :ok end
          )

        assert result == %{
                 name: "bb-weaver",
                 role: "builder",
                 healthy: false,
                 action: :unreachable
               }
      end)

    assert log =~ "unreachable after 2 wake attempt(s); operator attention required"

    [event] =
      Store.list_events("fleet")
      |> Enum.filter(&(&1["event_type"] == "sprite_recovery_failed"))

    assert event["payload"]["name"] == "bb-weaver"
    assert event["payload"]["attempts"] == 2
    assert event["payload"]["reason"] == "start failed"
  end

  test "reconcile_sprite marks provisioning failures as degraded" do
    test_pid = self()

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:ok, %{healthy: false}} end,
        provision_fn: fn sprite, opts ->
          send(test_pid, {:provision_called, sprite, opts})
          {:error, "setup failed"}
        end
      )

    assert_received {:provision_called, "bb-weaver",
                     [repo: "misty-step/bitterblossom", persona: "You are Weaver.", force: true]}

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :failed}
  end

  test "reconcile_all degrades sprites whose reconcile task crashes" do
    {:ok, [result]} =
      Reconciler.reconcile_all([@sprite],
        status_fn: fn _name, _opts -> {:ok, %{healthy: false}} end,
        provision_fn: fn _name, _opts -> raise "boom" end
      )

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :failed}
  end
end
