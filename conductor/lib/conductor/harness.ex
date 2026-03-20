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

  @harnesses [
    %{name: "codex", module: Conductor.Codex, command: "codex", requirement: "codex CLI"},
    %{
      name: "claude-code",
      module: Conductor.ClaudeCode,
      command: "claude",
      requirement: "claude CLI"
    }
  ]

  @diagnostic_prefix "[bb harness] "

  @spec classify_dispatch_failure(binary() | nil, integer() | nil) ::
          {:transient | :permanent, atom()}
  def classify_dispatch_failure(output, code) do
    message = output |> to_string() |> String.downcase()

    cond do
      String.contains?(message, "[bb harness] configured harness") and
          String.contains?(message, "[bb harness] supported harnesses:") ->
        {:permanent, :harness_unsupported}

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

      true ->
        {:permanent, :unknown}
    end
  end

  @spec supported_harnesses() :: [map()]
  def supported_harnesses, do: @harnesses

  @spec supported_harness_names() :: [binary()]
  def supported_harness_names, do: Enum.map(@harnesses, & &1.name)

  @spec supported_harness_summary() :: binary()
  def supported_harness_summary do
    @harnesses
    |> Enum.map_join(", ", fn harness -> "#{harness.name} (#{harness.requirement})" end)
  end

  @spec module_for_name(binary() | module()) :: {:ok, module()} | {:error, atom()}
  def module_for_name(module) when is_atom(module) do
    if Enum.any?(@harnesses, &(&1.module == module)),
      do: {:ok, module},
      else: {:error, :unknown_harness}
  end

  def module_for_name(name) when is_binary(name) do
    case Enum.find(@harnesses, &(&1.name == name)) do
      nil -> {:error, :unknown_harness}
      harness -> {:ok, harness.module}
    end
  end

  def module_for_name(_), do: {:error, :unknown_harness}

  @spec name(binary() | module()) :: binary()
  def name(harness) when is_binary(harness), do: harness

  def name(harness) when is_atom(harness) do
    case Enum.find(@harnesses, &(&1.module == harness)) do
      nil -> inspect(harness)
      known -> known.name
    end
  end

  @spec detect_dispatch_harness(binary(), binary() | module(), (binary(), binary(), keyword() ->
                                                                  {:ok, binary()}
                                                                  | {:error, binary(), integer()})) ::
          {:ok, %{diagnostics: [binary()], harness: module()}} | {:error, binary(), integer()}
  def detect_dispatch_harness(sprite, configured_harness, exec_fn) do
    case known_harness(configured_harness) do
      nil ->
        {:ok, %{harness: configured_harness, diagnostics: []}}

      configured ->
        configured_line =
          diagnostic_line("configured harness #{configured.name} on sprite #{sprite}")

        case command_available?(sprite, configured.command, exec_fn) do
          {:ok, configured_result} ->
            {:ok,
             %{harness: configured.module, diagnostics: [configured_line, configured_result]}}

          {:error, configured_result} ->
            handle_unavailable_harness(
              sprite,
              configured,
              configured_line,
              configured_result,
              exec_fn
            )
        end
    end
  end

  @spec safe_diagnostic_summary(binary() | nil) :: binary() | nil
  def safe_diagnostic_summary(output) do
    output
    |> to_string()
    |> String.split("\n")
    |> Enum.filter(&String.starts_with?(&1, @diagnostic_prefix))
    |> Enum.map(&String.replace_prefix(&1, @diagnostic_prefix, ""))
    |> Enum.reject(&(&1 == ""))
    |> case do
      [] -> nil
      lines -> Enum.join(lines, " | ")
    end
  end

  @spec attach_safe_diagnostics(binary(), binary() | nil) :: binary()
  def attach_safe_diagnostics(reason, output) do
    case safe_diagnostic_summary(output) do
      nil -> reason
      summary -> "#{reason}; harness: #{summary}"
    end
  end

  @spec annotate_initial_failure(binary() | nil, binary() | module()) :: binary()
  def annotate_initial_failure(output, harness) do
    diagnostics = [
      diagnostic_line(
        "selected harness #{name(harness)} has no continuation command; returning initial failure"
      )
    ]

    Enum.join(diagnostics ++ [to_string(output)], "\n")
  end

  @spec retry_backoff_ms(pos_integer()) :: non_neg_integer()
  def retry_backoff_ms(attempt) when attempt > 0 do
    base = Config.builder_retry_backoff_base_ms()
    exponent = min(attempt - 1, 2)
    base * trunc(:math.pow(2, exponent))
  end

  defp known_harness(harness) do
    harness_name = name(harness)
    Enum.find(@harnesses, &(&1.name == harness_name or &1.module == harness))
  end

  defp command_available?(sprite, command, exec_fn) do
    check = "command -v #{command} >/dev/null 2>&1"

    case exec_fn.(sprite, check, timeout: 15_000) do
      {:ok, _} -> {:ok, diagnostic_line("#{check} -> ok")}
      {:error, _output, _code} -> {:error, diagnostic_line("#{check} -> missing")}
    end
  end

  defp handle_unavailable_harness(sprite, configured, configured_line, configured_result, exec_fn) do
    alternates = Enum.reject(@harnesses, &(&1.name == configured.name))
    {available, alternate_diagnostics} = detect_alternate_harness(sprite, alternates, exec_fn)

    case available do
      nil ->
        diagnostics =
          [
            configured_line,
            configured_result
          ] ++
            alternate_diagnostics ++
            [
              diagnostic_line(
                "configured harness #{configured.name} unavailable on sprite #{sprite}"
              ),
              diagnostic_line("supported harnesses: #{supported_harness_summary()}")
            ]

        {:error, Enum.join(diagnostics, "\n"), 78}

      alternate ->
        {:ok,
         %{
           harness: alternate.module,
           diagnostics:
             [
               configured_line,
               configured_result
             ] ++
               alternate_diagnostics ++
               [
                 diagnostic_line(
                   "falling back to #{alternate.name} because #{configured.name} is unavailable"
                 )
               ]
         }}
    end
  end

  defp detect_alternate_harness(sprite, alternates, exec_fn) do
    Enum.reduce(alternates, {nil, []}, fn alternate, {selected, diagnostics} ->
      case command_available?(sprite, alternate.command, exec_fn) do
        {:ok, result} ->
          selected = selected || alternate
          {selected, diagnostics ++ [result]}

        {:error, result} ->
          {selected, diagnostics ++ [result]}
      end
    end)
  end

  defp diagnostic_line(message), do: @diagnostic_prefix <> message
end
