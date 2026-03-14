defmodule Conductor.ClaudeCode do
  @moduledoc """
  Harness implementation for the Claude Code CLI (via ralph loop).

  Satisfies the `Conductor.Harness` behaviour. Produces the base command
  that a Worker uses when dispatching an agent session on a sprite.
  """

  @behaviour Conductor.Harness

  @impl Conductor.Harness
  def name, do: "claude-code"

  @impl Conductor.Harness
  @doc """
  Return the claude CLI invocation for a non-interactive builder session.

  Recognised opts:
    - `:model` — override the model string (default from environment)
  """
  def dispatch_command(opts \\ []) do
    args = ["-p", "--dangerously-skip-permissions", "--verbose"]

    args =
      case Keyword.get(opts, :model) do
        nil -> args
        model -> args ++ ["--model", model]
      end

    {"claude", args}
  end
end
