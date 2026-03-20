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
          GenServer.stop(pid)
        catch
          :exit, _reason -> :ok
        end

        assert_receive {:DOWN, ^ref, :process, ^pid, _reason}, timeout
    end
  end
end

ExUnit.start()
