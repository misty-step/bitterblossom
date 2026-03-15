defmodule Conductor.ClaudeCode do
  @moduledoc """
  Agent harness for Claude Code (claude CLI).

  Implements `Conductor.Harness`.

  Claude Code is invoked with `-p` (print mode) and
  `--dangerously-skip-permissions` so the agent can act
  autonomously inside the sprite's isolated environment.
  The prompt is supplied via stdin.
  """

  @behaviour Conductor.Harness

  @default_model "claude-sonnet-4-6"

  @impl Conductor.Harness
  def name, do: "claude_code"

  @impl Conductor.Harness
  def dispatch_command(opts \\ []) do
    model = Keyword.get(opts, :model, @default_model)

    [
      "claude",
      "-p",
      "--dangerously-skip-permissions",
      "--model",
      model,
      "--verbose"
    ]
  end

  @impl Conductor.Harness
  def continue_command(opts \\ []) do
    model = Keyword.get(opts, :model, @default_model)

    [
      "claude",
      "--continue",
      "-p",
      "--dangerously-skip-permissions",
      "--model",
      model,
      "--verbose"
    ]
  end
end
