defmodule Conductor.Harness do
  @moduledoc """
  Behaviour for agent harnesses that run builder code on sprites.

  Implementations: `Conductor.ClaudeCode` (default).

  A harness knows its own name and how to produce the invocation
  command for its agent. The dispatch layer uses this to construct
  the full execution command on the sprite.
  """

  @doc "Human-readable identifier for this harness (e.g. \"claude_code\")."
  @callback name() :: binary()

  @doc """
  Build the base invocation command for this harness.

  Returns a list of command parts (argv). The prompt is typically
  supplied via stdin or a separate flag by the caller.

  Accepted opts:
  - `:model` — override the default model identifier
  """
  @callback dispatch_command(opts :: keyword()) :: [binary()]
end
