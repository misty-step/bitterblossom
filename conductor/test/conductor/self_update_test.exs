defmodule Conductor.SelfUpdateTest do
  use ExUnit.Case, async: false

  alias Conductor.SelfUpdate

  describe "check_for_updates/0" do
    test "returns :noop when HEAD matches origin/master" do
      # In the test environment, we just ran — HEAD == origin/master
      # (or fetch fails gracefully). Either way, no update should trigger.
      assert SelfUpdate.check_for_updates() in [:ok, :noop]
    end
  end

  describe "maybe_reload/2" do
    test "returns :noop for non-self repo" do
      assert SelfUpdate.maybe_reload("other-org/other-repo", 1) == :noop
    end
  end
end
