defmodule Conductor.Codex do
  @moduledoc """
  Agent harness for OpenAI Codex CLI.

  Implements `Conductor.Harness`.

  Codex is invoked via `codex exec` in full-auto mode with JSON output.
  The prompt is supplied via stdin. Model selection lives in the sprite's
  `~/.codex/config.toml`, not in CLI args.

  Codex has no session resumption — `continue_command/1` returns nil.
  """

  @behaviour Conductor.Harness

  @impl Conductor.Harness
  def name, do: "codex"

  @impl Conductor.Harness
  def dispatch_command(_opts \\ []) do
    ["codex", "exec", "--full-auto", "--json"]
  end

  @impl Conductor.Harness
  def continue_command(_opts), do: nil
end
