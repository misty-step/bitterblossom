# Stop the application so tests can manage their own Store instances
Application.stop(:conductor)

defmodule Conductor.TestSupport.ProcessHelpers do
  @moduledoc false

  import ExUnit.Assertions

  def stop_process(name, timeout \\ 1_000) do
    case Process.whereis(name) do
      nil ->
        :ok

      pid ->
        ref = Process.monitor(pid)

        try do
          GenServer.stop(pid, :normal, timeout)
        catch
          :exit, _reason -> :ok
        end

        assert_receive {:DOWN, ^ref, :process, ^pid, _reason}, timeout
    end
  end

  def stop_conductor_app(timeout \\ 1_000) do
    case Process.whereis(Conductor.Supervisor) do
      nil ->
        case Application.stop(:conductor) do
          :ok -> :ok
          {:error, {:not_started, :conductor}} -> :ok
        end

      pid ->
        ref = Process.monitor(pid)

        case Application.stop(:conductor) do
          :ok -> :ok
          {:error, {:not_started, :conductor}} -> :ok
        end

        assert_receive {:DOWN, ^ref, :process, ^pid, _reason}, timeout
    end
  end

  def restore_env(key, nil), do: Application.delete_env(:conductor, key)
  def restore_env(key, value), do: Application.put_env(:conductor, key, value)
end

ExUnit.start()
