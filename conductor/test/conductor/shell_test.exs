defmodule Conductor.ShellTest do
  use ExUnit.Case, async: true

  alias Conductor.Shell

  test "emits progress callbacks for PROGRESS lines while preserving command output" do
    test_pid = self()

    assert {:ok, output} =
             Shell.cmd(
               "bash",
               [
                 "-lc",
                 "printf 'PROGRESS: installing deps\\n'; sleep 0.05; printf 'PROGRESS: generating code\\n'; sleep 0.05; printf 'done\\n'"
               ],
               timeout: 1_000,
               on_progress: fn progress -> send(test_pid, {:progress, progress}) end
             )

    assert_received {:progress, %{message: "installing deps"}}
    assert_received {:progress, %{message: "generating code"}}
    assert output =~ "PROGRESS: installing deps"
    assert output =~ "PROGRESS: generating code"
    assert output =~ "done"
  end
end
