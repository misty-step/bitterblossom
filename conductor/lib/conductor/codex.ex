defmodule Conductor.Codex do
  @moduledoc """
  Agent harness for OpenAI Codex CLI.

  Implements `Conductor.Harness`.

  Codex is invoked via `codex exec` with `--yolo` (full sandbox bypass)
  and live web search. The prompt is supplied via stdin.

  Accepts `:reasoning_effort` opt to override the default (`"medium"`).
  The polisher uses `"high"`.

  """

  @behaviour Conductor.Harness

  @default_model "gpt-5.4"
  @default_reasoning "medium"

  @impl Conductor.Harness
  def name, do: "codex"

  @impl Conductor.Harness
  def dispatch_command(opts) do
    reasoning = Keyword.get(opts, :reasoning_effort, @default_reasoning)

    [
      "codex",
      "exec",
      "--yolo",
      "--json",
      "--model",
      @default_model,
      "-c",
      "web_search=live",
      "-c",
      "model_reasoning_effort=#{reasoning}"
    ]
  end
end
