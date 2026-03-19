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

  alias Conductor.Config

  @spec classify_dispatch_failure(binary() | nil, integer() | nil) ::
          {:transient | :permanent, atom()}
  def classify_dispatch_failure(output, code) do
    message = output |> to_string() |> String.downcase()

    cond do
      String.contains?(message, "timeout") or code == 124 ->
        {:transient, :network_timeout}

      String.contains?(message, "resource contention") or
        String.contains?(message, "temporarily unavailable") or code in [70, 75] ->
        {:transient, :resource_contention}

      String.contains?(message, "busy") or String.contains?(message, "unavailable") ->
        {:transient, :worker_unavailable}

      String.contains?(message, "auth") or String.contains?(message, "permission denied") or
          code == 4 ->
        {:permanent, :auth}

      String.contains?(message, "harness does not support continuation") ->
        {:permanent, :harness_unsupported}

      true ->
        {:permanent, :unknown}
    end
  end

  @spec retry_backoff_ms(pos_integer()) :: non_neg_integer()
  def retry_backoff_ms(attempt) when attempt > 0 do
    base = Config.builder_retry_backoff_base_ms()
    exponent = min(attempt - 1, 2)
    base * trunc(:math.pow(2, exponent))
  end
end
