defmodule Conductor.Fleet.ReconcilerTest do
  use ExUnit.Case, async: true

  alias Conductor.Fleet.Reconciler
  @missing_sprite_reason "failed to start sprite command: sprite not found"

  @sprite %{
    name: "bb-weaver",
    role: "builder",
    repo: "misty-step/bitterblossom",
    persona: "You are Weaver.",
    harness: "codex"
  }

  test "reconcile_sprite creates missing sprites before provisioning them" do
    test_pid = self()

    status_fn = fn _name, _opts ->
      case Process.get(:status_call_count, 0) do
        0 ->
          Process.put(:status_call_count, 1)
          {:error, @missing_sprite_reason}

        _ ->
          {:ok, %{healthy: true}}
      end
    end

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: status_fn,
        create_fn: fn sprite, opts ->
          send(test_pid, {:create_called, sprite, opts})
          :ok
        end,
        provision_fn: fn sprite, opts ->
          send(test_pid, {:provision_called, sprite, opts})
          :ok
        end
      )

    assert_received {:create_called, "bb-weaver", []}

    assert_received {:provision_called, "bb-weaver",
                     [repo: "misty-step/bitterblossom", persona: "You are Weaver.", force: true]}

    assert result == %{name: "bb-weaver", role: "builder", healthy: true, action: :created}
  end

  test "reconcile_sprite marks unreachable sprites degraded without creating them" do
    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:error, "timeout"} end,
        create_fn: fn _name, _opts -> flunk("create_fn should not be called") end,
        provision_fn: fn _name, _opts -> flunk("provision_fn should not be called") end
      )

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :unreachable}
  end

  test "reconcile_sprite marks sprite creation failures as degraded" do
    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts ->
          {:error, @missing_sprite_reason}
        end,
        create_fn: fn _name, _opts -> {:error, "create failed"} end,
        provision_fn: fn _name, _opts -> flunk("provision_fn should not be called") end
      )

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :failed}
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
