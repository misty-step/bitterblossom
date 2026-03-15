defmodule Conductor.CodexTest do
  use ExUnit.Case, async: true

  alias Conductor.Codex

  describe "name/0" do
    test "returns codex" do
      assert Codex.name() == "codex"
    end
  end

  describe "dispatch_command/1" do
    test "returns codex exec with full-auto and json flags" do
      assert Codex.dispatch_command([]) == ["codex", "exec", "--full-auto", "--json"]
    end

    test "ignores model opt (model set via config.toml on sprite)" do
      assert Codex.dispatch_command(model: "o3") == ["codex", "exec", "--full-auto", "--json"]
    end
  end

  describe "continue_command/1" do
    test "returns nil (codex has no session resumption)" do
      assert Codex.continue_command([]) == nil
    end
  end
end
