defmodule Conductor.Shell do
  @moduledoc """
  Subprocess execution with timeout support.

  Deep module: hides process spawning, timeout handling, and output capture.
  All external commands flow through here.
  """

  require Logger

  @type result :: {:ok, binary()} | {:error, binary(), non_neg_integer()}

  @spec cmd(binary(), [binary()], keyword()) :: result()
  def cmd(program, args, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 120_000)
    env = Keyword.get(opts, :env, [])
    cd = Keyword.get(opts, :cd)

    sys_opts =
      [stderr_to_stdout: true, env: env]
      |> maybe_cd(cd)

    task = Task.async(fn -> System.cmd(program, args, sys_opts) end)

    case Task.yield(task, timeout) || Task.shutdown(task, :brutal_kill) do
      {:ok, {output, 0}} ->
        {:ok, String.trim(output)}

      {:ok, {output, code}} ->
        {:error, String.trim(output), code}

      nil ->
        {:error, "command timed out after #{timeout}ms: #{program} #{Enum.join(args, " ")}", 124}
    end
  end

  @spec cmd!(binary(), [binary()], keyword()) :: binary()
  def cmd!(program, args, opts \\ []) do
    case cmd(program, args, opts) do
      {:ok, output} ->
        output

      {:error, output, code} ->
        raise "command failed (#{code}): #{program} #{Enum.join(args, " ")}\n#{output}"
    end
  end

  @spec validate_bash(binary(), keyword()) :: :ok | {:error, binary()}
  def validate_bash(command, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 10_000)

    case cmd("bash", ["-n", "-c", command], timeout: timeout) do
      {:ok, _output} -> :ok
      {:error, output, _code} -> {:error, String.trim(output)}
    end
  end

  defp maybe_cd(opts, nil), do: opts
  defp maybe_cd(opts, cd), do: [{:cd, cd} | opts]
end
