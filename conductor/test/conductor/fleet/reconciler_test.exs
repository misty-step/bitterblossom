defmodule Conductor.Fleet.ReconcilerTest do
  use ExUnit.Case, async: true

  alias Conductor.Fleet.Reconciler

  describe "find_bb path resolution" do
    # find_bb/0 is private, so we test it through reconcile_sprite/1
    # which calls run_setup → find_bb when a sprite needs setup.
    # The key invariant: returned paths must be absolute (Path.expand),
    # because System.cmd/3 does not resolve ".." in executable paths.

    test "reconcile_sprite returns degraded (not crash) when sprite is unreachable" do
      # Ensure SPRITES_ORG is set so Config.sprites_org! doesn't raise
      prev = System.get_env("SPRITES_ORG")
      System.put_env("SPRITES_ORG", "test-org")

      try do
        sprite = %{
          name: "test-sprite",
          role: "builder",
          harness: "codex",
          repo: "test/repo",
          persona: nil
        }

        result = Reconciler.reconcile_sprite(sprite)

        # Should not crash — returns a map with healthy: false
        assert is_map(result)
        assert result.name == "test-sprite"
        # Unreachable (can't reach sprite) — graceful degradation, not crash
        assert result.healthy == false
      after
        if prev, do: System.put_env("SPRITES_ORG", prev), else: System.delete_env("SPRITES_ORG")
      end
    end
  end
end
