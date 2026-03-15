defmodule Conductor.Harness do
  @moduledoc """
  Behaviour for agent harnesses that run builder code on sprites.

  Implementations: `Conductor.Codex` (default), `Conductor.ClaudeCode`.

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

  @doc """
  Build the session-resumption command for this harness.

  Used when the agent crashes mid-task: resumes the prior session
  instead of starting fresh. Returns nil if the harness does not
  support continuation.

  Accepted opts:
  - `:model` — override the default model identifier
  """
  @callback continue_command(opts :: keyword()) :: [binary()] | nil
end
