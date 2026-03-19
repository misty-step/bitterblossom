defmodule Conductor.Fleet.ReconcilerTest do
  use ExUnit.Case, async: true

  alias Conductor.Fleet.Reconciler

  @sprite %{
    name: "bb-weaver",
    role: "builder",
    repo: "misty-step/bitterblossom",
    persona: "You are Weaver.",
    harness: "codex"
  }

  test "reconcile_sprite marks unreachable sprites degraded without provisioning" do
    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:error, "timeout"} end,
        provision_fn: fn _name, _opts -> flunk("provision_fn should not be called") end
      )

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :unreachable}
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
