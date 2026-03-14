defmodule Conductor.Harness do
  @moduledoc """
  Behaviour for agent runtimes that builders execute within.

  Implementations: `Conductor.ClaudeCode` (Claude Code CLI via ralph loop).
  Future: Codex, local LLM harness, stub harness for tests.
  """

  @doc "Human-readable identifier for this harness."
  @callback name() :: binary()

  @doc """
  Return the base command `{executable, args}` used to invoke the agent.

  Opts may include `:workspace`, `:timeout_minutes`, `:model`, etc.
  The caller (Worker implementation) appends input redirection as needed.
  """
  @callback dispatch_command(opts :: keyword()) :: {binary(), [binary()]}
end
