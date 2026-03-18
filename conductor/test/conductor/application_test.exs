defmodule Conductor.ApplicationTest do
  use ExUnit.Case, async: true

  test "maps renamed phase worker roles to sprite display names" do
    assert Conductor.Application.role_display_name(:fixer) == "thorn"
    assert Conductor.Application.role_display_name(:polisher) == "fern"
  end

  test "falls back to the raw role name for unmapped roles" do
    assert Conductor.Application.role_display_name(:builder) == "builder"
    assert Conductor.Application.role_display_name(:triage) == "triage"
  end
end
