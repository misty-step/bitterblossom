defmodule Conductor.Fleet.ReconcilerTest do
  use ExUnit.Case, async: false

  alias Conductor.Fleet.Reconciler

  defmodule SpriteMock do
    def status(_sprite, _opts) do
      case Process.get(:sprite_status_responses, []) do
        [response | rest] ->
          Process.put(:sprite_status_responses, rest)
          response

        [] ->
          raise "no sprite status response configured"
      end
    end
  end

  defmodule ShellMock do
    def cmd(program, args, opts \\ []) do
      calls = Process.get(:shell_calls, [])
      Process.put(:shell_calls, [{program, args, opts} | calls])
      Process.get(:shell_result, {:ok, "setup ok"})
    end
  end

  setup do
    original =
      for key <- [:sprite_module, :shell_module, :bb_path], into: %{} do
        {key, Application.get_env(:conductor, key)}
      end

    Application.put_env(:conductor, :sprite_module, SpriteMock)
    Application.put_env(:conductor, :shell_module, ShellMock)
    Application.put_env(:conductor, :bb_path, "bb-test")

    Process.put(:shell_calls, [])

    on_exit(fn ->
      Enum.each(original, fn
        {key, nil} -> Application.delete_env(:conductor, key)
        {key, value} -> Application.put_env(:conductor, key, value)
      end)
    end)

    :ok
  end

  test "provisions from the repo root and rechecks sprite health" do
    Process.put(:sprite_status_responses, [
      {:ok, %{healthy: false}},
      {:ok, %{healthy: true}}
    ])

    sprite = %{
      name: "bb-weaver-1",
      role: :builder,
      harness: "codex",
      repo: "misty-step/bitterblossom",
      persona: nil
    }

    assert %{healthy: true, action: :provisioned} = Reconciler.reconcile_sprite(sprite)

    assert [
             {"bb-test",
              ["setup", "bb-weaver-1", "--repo", "misty-step/bitterblossom", "--force"], opts}
           ] = Enum.reverse(Process.get(:shell_calls, []))

    assert opts[:timeout] == 300_000
    assert opts[:cd] == Path.expand("../../../../", __DIR__)
    assert Process.get(:sprite_status_responses) == []
  end
end
