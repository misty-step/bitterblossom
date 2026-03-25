defmodule Conductor.ShellTest do
  use ExUnit.Case, async: true

  alias Conductor.Shell

  describe "validate_bash/2" do
    test "accepts valid bash" do
      assert :ok = Shell.validate_bash("set -e\nprintf 'ok\\n'\n")
    end

    test "returns the parser error for invalid bash" do
      assert {:error, message} =
               Shell.validate_bash(
                 "printf 'ok\\n'\n| while read -r line; do\n  echo \"$line\"\ndone\n"
               )

      assert message =~ "syntax error"
      assert message =~ "unexpected token `|'"
    end
  end
end
