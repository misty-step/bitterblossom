defmodule Conductor.SpriteHealthTest do
  use ExUnit.Case, async: true

  alias Conductor.Sprite

  describe "probe/2" do
    test "uses the probe marker command to verify the sprite" do
      test_pid = self()

      assert {:ok, %{sprite: "test-sprite", reachable: true}} =
               Sprite.probe("test-sprite",
                 exec_fn: fn sprite, cmd, _opts ->
                   send(test_pid, {:probe_called, sprite, cmd})
                   {:ok, "ok\n"}
                 end
               )

      assert_received {:probe_called, "test-sprite", "printf '__bb_probe__'"}
    end

    test "returns an error when the sprite cannot be reached" do
      assert {:error, "connection refused"} =
               Sprite.probe("test-sprite",
                 exec_fn: fn _sprite, _cmd, _opts ->
                   {:error, "connection refused", 255}
                 end
               )
    end
  end

  describe "busy?/1" do
    test "returns true when agent processes are detected" do
      # Stub exec to simulate pgrep finding claude processes
      busy =
        Sprite.busy?("test-sprite",
          exec_fn: fn _sprite, _cmd, _opts ->
            {:ok, "12345 claude -p\n12346 claude -p\n"}
          end
        )

      assert busy == true
    end

    test "returns false when no agent processes found" do
      busy =
        Sprite.busy?("test-sprite",
          exec_fn: fn _sprite, _cmd, _opts ->
            # pgrep returns exit 1 when no matches
            {:error, "", 1}
          end
        )

      assert busy == false
    end

    test "returns true when codex processes are detected" do
      busy =
        Sprite.busy?("test-sprite",
          exec_fn: fn _sprite, cmd, _opts ->
            if String.contains?(cmd, "codex") do
              {:ok, "54321\n"}
            else
              {:error, "", 1}
            end
          end
        )

      assert busy == true
    end

    test "returns false when sprite is unreachable" do
      busy =
        Sprite.busy?("test-sprite",
          exec_fn: fn _sprite, _cmd, _opts ->
            {:error, "connection refused", 255}
          end
        )

      assert busy == false
    end
  end
end
