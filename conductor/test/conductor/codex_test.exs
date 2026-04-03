defmodule Conductor.CodexTest do
  use ExUnit.Case, async: true

  alias Conductor.Codex

  describe "name/0" do
    test "returns codex" do
      assert Codex.name() == "codex"
    end
  end

  describe "dispatch_command/1" do
    test "returns codex exec with yolo, json, model, web search, and medium reasoning" do
      cmd = Codex.dispatch_command([])

      assert "codex" in cmd
      assert "exec" in cmd
      assert "--yolo" in cmd
      assert "--json" in cmd
      assert "--model" in cmd
      assert "gpt-5.4-mini" in cmd
      assert "web_search=live" in cmd
      assert "model_reasoning_effort=medium" in cmd
    end

    test "accepts reasoning_effort override" do
      cmd = Codex.dispatch_command(reasoning_effort: "high")
      assert "model_reasoning_effort=high" in cmd
      refute "model_reasoning_effort=medium" in cmd
    end
  end
end
