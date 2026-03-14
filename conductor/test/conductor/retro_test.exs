defmodule Conductor.RetroTest do
  use ExUnit.Case, async: true

  alias Conductor.Retro

  describe "analyze/1" do
    test "returns :ok without crashing when retro is disabled" do
      # Retro is disabled when no API key is set (test environment)
      assert Retro.analyze("run-999-0000000000") == :ok
    end
  end
end
