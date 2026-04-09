defmodule Conductor.CLILogsTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO

  alias Conductor.CLI

  test "logs help prints usage without exiting" do
    output =
      capture_io(fn ->
        CLI.main(["logs", "--help"])
      end)

    assert output =~ "usage: bitterblossom logs <sprite>"
    assert output =~ "--follow"
    assert output =~ "--lines"
  end
end
