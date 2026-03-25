defmodule Conductor.ShellTest do
  use ExUnit.Case, async: true

  alias Conductor.Shell

  test "cmd closes stdin for sprite invocations" do
    tmp_dir = Path.join(System.tmp_dir!(), "shell_sprite_#{System.unique_integer([:positive])}")
    File.mkdir_p!(tmp_dir)
    on_exit(fn -> File.rm_rf(tmp_dir) end)

    sprite_path = Path.join(tmp_dir, "sprite")

    File.write!(
      sprite_path,
      """
      #!/bin/sh
      if IFS= read -r line; then
        echo "stdin-open:$line"
      else
        echo "stdin-closed"
      fi
      """
    )

    File.chmod!(sprite_path, 0o755)

    path_env = tmp_dir <> ":" <> System.get_env("PATH", "")

    assert {:ok, "stdin-closed"} =
             Shell.cmd("sprite", ["ignored"], env: [{"PATH", path_env}], timeout: 1_000)
  end
end
