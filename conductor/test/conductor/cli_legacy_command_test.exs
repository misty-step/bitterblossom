defmodule Conductor.CLILegacyCommandTest do
  use ExUnit.Case, async: false

  @conductor_dir Path.expand("../..", __DIR__)

  test "mix conductor run-once prints the removal message and exits 1" do
    {output, status} =
      System.cmd("mix", ["conductor", "run-once"], cd: @conductor_dir, stderr_to_stdout: true)

    assert status == 1
    assert output =~ "run-once removed"
    assert output =~ "mix conductor start"
  end

  test "mix conductor loop prints the removal message and exits 1" do
    {output, status} =
      System.cmd("mix", ["conductor", "loop"], cd: @conductor_dir, stderr_to_stdout: true)

    assert status == 1
    assert output =~ "loop removed"
    assert output =~ "mix conductor start"
  end
end
