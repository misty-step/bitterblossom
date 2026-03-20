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
    on_progress = Keyword.get(opts, :on_progress)

    case on_progress do
      fun when is_function(fun, 1) ->
        progress_prefix = Keyword.get(opts, :progress_prefix, "PROGRESS:")
        stream_cmd(program, args, timeout, env, cd, fun, progress_prefix)

      _ ->
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
            {:error, "command timed out after #{timeout}ms: #{program} #{Enum.join(args, " ")}",
             124}
        end
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

  defp maybe_cd(opts, nil), do: opts
  defp maybe_cd(opts, cd), do: [{:cd, cd} | opts]

  defp stream_cmd(program, args, timeout, env, cd, on_progress, progress_prefix) do
    executable = System.find_executable(program) || program

    port =
      Port.open(
        {:spawn_executable, executable},
        [:binary, :exit_status, :stderr_to_stdout, :use_stdio, :hide, args: args]
        |> maybe_port_env(env)
        |> maybe_port_cd(cd)
      )

    deadline = System.monotonic_time(:millisecond) + timeout

    collect_port_output(
      port,
      deadline,
      timeout,
      program,
      args,
      "",
      [],
      on_progress,
      progress_prefix
    )
  end

  defp collect_port_output(
         port,
         deadline,
         timeout,
         program,
         args,
         partial,
         chunks,
         on_progress,
         progress_prefix
       ) do
    remaining = max(deadline - System.monotonic_time(:millisecond), 0)

    receive do
      {^port, {:data, data}} ->
        {next_partial, progress_messages} =
          extract_progress_messages(partial <> data, progress_prefix)

        Enum.each(progress_messages, &emit_progress(on_progress, &1))

        collect_port_output(
          port,
          deadline,
          timeout,
          program,
          args,
          next_partial,
          [data | chunks],
          on_progress,
          progress_prefix
        )

      {^port, {:exit_status, 0}} ->
        {:ok, chunks |> Enum.reverse() |> IO.iodata_to_binary() |> String.trim()}

      {^port, {:exit_status, code}} ->
        {:error, chunks |> Enum.reverse() |> IO.iodata_to_binary() |> String.trim(), code}
    after
      remaining ->
        Port.close(port)
        {:error, "command timed out after #{timeout}ms: #{program} #{Enum.join(args, " ")}", 124}
    end
  end

  defp extract_progress_messages(buffer, progress_prefix) do
    case String.split(buffer, "\n") do
      [] ->
        {"", []}

      parts ->
        {complete_lines, [partial]} = Enum.split(parts, -1)

        messages =
          complete_lines
          |> Enum.map(&String.trim_trailing(&1, "\r"))
          |> Enum.flat_map(fn line ->
            if String.starts_with?(line, progress_prefix) do
              <<_prefix::binary-size(byte_size(progress_prefix)), message::binary>> = line
              message = String.trim(message)
              if message == "", do: [], else: [message]
            else
              []
            end
          end)

        {partial, messages}
    end
  end

  defp emit_progress(on_progress, message) do
    on_progress.(%{message: message})
  rescue
    error ->
      Logger.warning("[shell] progress callback failed: #{Exception.message(error)}")
  end

  defp maybe_port_env(opts, []), do: opts
  defp maybe_port_env(opts, env), do: [{:env, env} | opts]
  defp maybe_port_cd(opts, nil), do: opts
  defp maybe_port_cd(opts, cd), do: [{:cd, cd} | opts]
end
