defmodule Conductor.CLILogsTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO

  alias Conductor.CLI

  @conductor_dir Path.expand("../..", __DIR__)

  test "logs help prints usage without exiting" do
    output =
      capture_io(fn ->
        CLI.main(["logs", "--help"])
      end)

    assert output =~ "usage: mix conductor logs <sprite>"
    assert output =~ "--follow"
    assert output =~ "--lines"
  end

  test "mix conductor logs rejects negative line counts" do
    {output, status} =
      System.cmd("mix", ["conductor", "logs", "bb-weaver", "--lines", "-1"],
        cd: @conductor_dir,
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "--lines must be >= 0"
  end
end
