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

  @sprite %{
    name: "bb-weaver",
    role: "builder",
    repo: "misty-step/bitterblossom",
    persona: "You are Weaver.",
    harness: "codex"
  }

  setup do
    original =
      for key <- [:sprite_module, :shell_module, :bb_path, :repo_root], into: %{} do
        {key, Application.get_env(:conductor, key)}
      end

    original_path = System.get_env("PATH")

    Application.put_env(:conductor, :sprite_module, SpriteMock)
    Application.put_env(:conductor, :shell_module, ShellMock)
    Application.put_env(:conductor, :bb_path, "bb-test")

    Process.put(:shell_calls, [])

    on_exit(fn ->
      Enum.each(original, fn
        {key, nil} -> Application.delete_env(:conductor, key)
        {key, value} -> Application.put_env(:conductor, key, value)
      end)

      restore_path(original_path)
    end)

    :ok
  end

  test "provisions from the runtime repo root and rechecks sprite health" do
    repo_root = temp_dir("reconciler-root")
    File.mkdir_p!(repo_root)
    on_exit(fn -> File.rm_rf(repo_root) end)

    Application.put_env(:conductor, :repo_root, repo_root)

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
    assert opts[:cd] == repo_root
    assert Process.get(:sprite_status_responses) == []
  end

  test "treats sprite status errors as unreachable" do
    Process.put(:sprite_status_responses, [{:error, :socket_closed}])

    sprite = %{
      name: "bb-weaver-1",
      role: :builder,
      harness: "codex",
      repo: "misty-step/bitterblossom",
      persona: nil
    }

    assert %{healthy: false, action: :unreachable} = Reconciler.reconcile_sprite(sprite)
    assert Process.get(:shell_calls) == []
  end

  test "prefers the configured bb path before repo and PATH lookups" do
    repo_root = temp_dir("configured-root")
    File.mkdir_p!(repo_root)
    on_exit(fn -> File.rm_rf(repo_root) end)

    Application.put_env(:conductor, :repo_root, repo_root)
    Application.put_env(:conductor, :bb_path, "/custom/bb")

    Process.put(:sprite_status_responses, [{:ok, %{healthy: false}}, {:ok, %{healthy: true}}])

    sprite = %{
      name: "bb-weaver-1",
      role: :builder,
      harness: "codex",
      repo: "misty-step/bitterblossom",
      persona: nil
    }

    assert %{healthy: true, action: :provisioned} = Reconciler.reconcile_sprite(sprite)

    assert [
             {"/custom/bb",
              ["setup", "bb-weaver-1", "--repo", "misty-step/bitterblossom", "--force"], opts}
           ] = Enum.reverse(Process.get(:shell_calls, []))

    assert opts[:cd] == repo_root
  end

  test "uses repo bin/bb when no configured path is set" do
    repo_root = temp_dir("repo-bb-root")
    File.mkdir_p!(Path.join(repo_root, "bin"))
    bb_path = Path.join(repo_root, "bin/bb")
    write_executable(bb_path)
    on_exit(fn -> File.rm_rf(repo_root) end)

    Application.put_env(:conductor, :repo_root, repo_root)
    Application.delete_env(:conductor, :bb_path)

    Process.put(:sprite_status_responses, [{:ok, %{healthy: false}}, {:ok, %{healthy: true}}])

    sprite = %{
      name: "bb-weaver-1",
      role: :builder,
      harness: "codex",
      repo: "misty-step/bitterblossom",
      persona: nil
    }

    assert %{healthy: true, action: :provisioned} = Reconciler.reconcile_sprite(sprite)

    assert [
             {^bb_path, ["setup", "bb-weaver-1", "--repo", "misty-step/bitterblossom", "--force"],
              _opts}
           ] = Enum.reverse(Process.get(:shell_calls, []))
  end

  test "falls back to bb on PATH when repo bin/bb is missing" do
    repo_root = temp_dir("path-bb-root")
    path_dir = temp_dir("path-bb-bin")
    File.mkdir_p!(repo_root)
    File.mkdir_p!(path_dir)
    bb_path = Path.join(path_dir, "bb")
    write_executable(bb_path)

    on_exit(fn ->
      File.rm_rf(repo_root)
      File.rm_rf(path_dir)
    end)

    Application.put_env(:conductor, :repo_root, repo_root)
    Application.delete_env(:conductor, :bb_path)
    System.put_env("PATH", path_dir)

    Process.put(:sprite_status_responses, [{:ok, %{healthy: false}}, {:ok, %{healthy: true}}])

    sprite = %{
      name: "bb-weaver-1",
      role: :builder,
      harness: "codex",
      repo: "misty-step/bitterblossom",
      persona: nil
    }

    assert %{healthy: true, action: :provisioned} = Reconciler.reconcile_sprite(sprite)

    assert [
             {"bb", ["setup", "bb-weaver-1", "--repo", "misty-step/bitterblossom", "--force"],
              _opts}
           ] =
             Enum.reverse(Process.get(:shell_calls, []))
  end

  test "fails cleanly when no bb binary can be found" do
    repo_root = temp_dir("missing-bb-root")
    empty_path = temp_dir("empty-path")
    File.mkdir_p!(repo_root)
    File.mkdir_p!(empty_path)

    on_exit(fn ->
      File.rm_rf(repo_root)
      File.rm_rf(empty_path)
    end)

    Application.put_env(:conductor, :repo_root, repo_root)
    Application.delete_env(:conductor, :bb_path)
    System.put_env("PATH", empty_path)

    Process.put(:sprite_status_responses, [{:ok, %{healthy: false}}])

    sprite = %{
      name: "bb-weaver-1",
      role: :builder,
      harness: "codex",
      repo: "misty-step/bitterblossom",
      persona: nil
    }

    assert %{healthy: false, action: :failed} = Reconciler.reconcile_sprite(sprite)
    assert Process.get(:shell_calls) == []
  end

  test "reconcile_sprite marks unreachable sprites degraded without provisioning" do
    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:error, "timeout"} end,
        provision_fn: fn _name, _opts -> flunk("provision_fn should not be called") end
      )

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :unreachable}
  end

  test "reconcile_sprite marks provisioning failures as degraded" do
    test_pid = self()

    result =
      Reconciler.reconcile_sprite(@sprite,
        status_fn: fn _name, _opts -> {:ok, %{healthy: false}} end,
        provision_fn: fn sprite, opts ->
          send(test_pid, {:provision_called, sprite, opts})
          {:error, "setup failed"}
        end
      )

    assert_received {:provision_called, "bb-weaver",
                     [repo: "misty-step/bitterblossom", persona: "You are Weaver.", force: true]}

    assert result == %{name: "bb-weaver", role: "builder", healthy: false, action: :failed}
  end

  defp temp_dir(prefix) do
    Path.join(System.tmp_dir!(), "#{prefix}-#{System.unique_integer([:positive])}")
  end

  defp write_executable(path) do
    File.write!(path, "#!/bin/sh\nexit 0\n")
    File.chmod!(path, 0o755)
  end

  defp restore_path(nil), do: System.delete_env("PATH")
  defp restore_path(path), do: System.put_env("PATH", path)
end
