defmodule Conductor.SelfUpdateTest do
  use ExUnit.Case, async: false

  alias Conductor.SelfUpdate

  describe "check_for_updates/0" do
    test "returns :noop when not behind origin/master" do
      # On CI branches, HEAD may differ from origin/master, but
      # rev-list --count HEAD..origin/master returns 0 when at or ahead.
      # We only assert :noop to avoid mutating the checkout mid-test.
      assert SelfUpdate.check_for_updates() == :noop
    end
  end

  describe "maybe_reload/2" do
    test "returns :noop for non-self repo" do
      assert SelfUpdate.maybe_reload("other-org/other-repo", 1) == :noop
    end
  end
end
