defmodule Conductor.SelfUpdateTest do
  use ExUnit.Case, async: false

  alias Conductor.SelfUpdate

  describe "check_for_updates/0" do
    test "returns a valid result" do
      result = SelfUpdate.check_for_updates()
      assert result in [:noop, :ok, {:error, :recompile_failed}]
    end
  end

  describe "maybe_reload/2" do
    test "returns :noop for non-self repo" do
      assert SelfUpdate.maybe_reload("other-org/other-repo", 1) == :noop
    end
  end
end
