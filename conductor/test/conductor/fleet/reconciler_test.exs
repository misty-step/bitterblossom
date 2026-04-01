defmodule Conductor.Fleet.ReconcilerTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Fleet.Reconciler
  alias Conductor.Store

  @sprite %{
    name: "bb-weaver",
    role: "builder",
    org: "misty-step",
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

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: false,
             loop_alive: false,
             action: :unreachable
           }
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

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: true,
             loop_alive: false,
             action: :woken
           }
  end

  test "reconcile_sprite provisions a sprite that is reachable after wake but still needs setup" do
    test_pid = self()
    status_calls = :atomics.new(1, [])

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts ->
          case :atomics.add_get(status_calls, 1, 1) do
            1 -> {:error, "websocket: bad handshake (HTTP 502)"}
            2 -> {:ok, %{healthy: false}}
            3 -> {:ok, %{healthy: true}}
          end
        end,
        wake_fn: fn sprite, opts ->
          send(test_pid, {:wake_called, sprite, opts})
          :ok
        end,
        provision_fn: fn sprite, opts ->
          send(test_pid, {:provision_called, sprite, opts})
          :ok
        end,
        sleep_fn: fn _ -> :ok end
      )

    assert_received {:wake_called, "bb-weaver", _wake_opts}

    assert_received {:provision_called, "bb-weaver",
                     [
                       repo: "misty-step/bitterblossom",
                       persona: "You are Weaver.",
                       harness: "codex",
                       org: "misty-step",
                       force: true
                     ]}

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: true,
             loop_alive: false,
             action: :provisioned
           }
  end

  test "reconcile_sprite logs and records a fleet event after recovery retries are exhausted" do
    log =
      capture_log(fn ->
        result =
          Reconciler.reconcile_sprite(@sprite,
            status_fn: fn _name, _opts -> {:error, "websocket: bad handshake (HTTP 502)"} end,
            wake_fn: fn _name, _opts -> {:error, "start failed\r\nmanual check required"} end,
            max_attempts: 2,
            sleep_fn: fn _ -> :ok end
          )

        assert result == %{
                 name: "bb-weaver",
                 role: "builder",
                 healthy: false,
                 loop_alive: false,
                 action: :unreachable
               }
      end)

    assert log =~ "unreachable after 2 wake attempt(s); operator attention required"
    assert log =~ "start failed manual check required; retrying in 1000ms"
    refute log =~ "start failed\r\nmanual check required"

    [event] =
      Store.list_events("fleet")
      |> Enum.filter(&(&1["event_type"] == "sprite_recovery_failed"))

    assert event["payload"]["name"] == "bb-weaver"
    assert event["payload"]["attempts"] == 2
    assert event["payload"]["reason"] == "start failed manual check required"
  end

  test "reconcile_sprite records exhausted recovery failures through an injected event_fn" do
    test_pid = self()

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:error, "websocket: bad handshake (HTTP 502)"} end,
        wake_fn: fn _name, _opts -> {:error, "start failed\r\nmanual check required"} end,
        max_attempts: 1,
        sleep_fn: fn _ -> :ok end,
        event_fn: fn sprite, attempts, reason ->
          send(test_pid, {:event_called, sprite.name, attempts, reason})
          :ok
        end
      )

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: false,
             loop_alive: false,
             action: :unreachable
           }

    assert_received {:event_called, "bb-weaver", 1, "start failed manual check required"}
  end

  test "reconcile_sprite still logs recovery exhaustion when Store is unavailable" do
    stop_process(Store)

    log =
      capture_log(fn ->
        result =
          Reconciler.reconcile_sprite(@sprite,
            status_fn: fn _name, _opts -> {:error, "websocket: bad handshake (HTTP 502)"} end,
            wake_fn: fn _name, _opts -> {:error, "start failed"} end,
            max_attempts: 1,
            sleep_fn: fn _ -> :ok end
          )

        assert result == %{
                 name: "bb-weaver",
                 role: "builder",
                 healthy: false,
                 loop_alive: false,
                 action: :unreachable
               }
      end)

    assert log =~ "unreachable after 1 wake attempt(s); operator attention required"
  end

  test "reconcile_sprite uses exponential backoff between wake retries" do
    base_key = :fleet_recovery_backoff_base_ms
    cap_key = :fleet_recovery_backoff_cap_ms
    orig_base = Application.get_env(:conductor, base_key)
    orig_cap = Application.get_env(:conductor, cap_key)
    Application.put_env(:conductor, base_key, 100)
    Application.put_env(:conductor, cap_key, 250)

    on_exit(fn ->
      restore_env(base_key, orig_base)
      restore_env(cap_key, orig_cap)
    end)

    test_pid = self()

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:error, "websocket: bad handshake (HTTP 502)"} end,
        wake_fn: fn _name, _opts -> {:error, "still down"} end,
        max_attempts: 4,
        sleep_fn: fn ms ->
          send(test_pid, {:sleep_called, ms})
          :ok
        end,
        event_fn: fn _sprite, _attempts, _reason -> :ok end
      )

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: false,
             loop_alive: false,
             action: :unreachable
           }

    assert_received {:sleep_called, 100}
    assert_received {:sleep_called, 200}
    assert_received {:sleep_called, 250}
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
                     [
                       repo: "misty-step/bitterblossom",
                       persona: "You are Weaver.",
                       harness: "codex",
                       org: "misty-step",
                       force: true
                     ]}

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: false,
             loop_alive: false,
             action: :failed
           }
  end

  test "reconcile_all degrades sprites whose reconcile task crashes" do
    {:ok, [result]} =
      Reconciler.reconcile_all([@sprite],
        status_fn: fn _name, _opts -> {:ok, %{healthy: false}} end,
        provision_fn: fn _name, _opts -> raise "boom" end
      )

    assert result == %{
             name: "bb-weaver",
             role: "builder",
             healthy: false,
             loop_alive: false,
             action: :failed
           }
  end
end
