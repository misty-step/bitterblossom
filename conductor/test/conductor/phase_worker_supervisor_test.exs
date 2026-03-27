defmodule Conductor.PhaseWorkerSupervisorTest do
  use ExUnit.Case, async: false
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.PhaseWorker
  alias Conductor.PhaseWorker.Roles
  alias Conductor.PhaseWorker.Supervisor

  defmodule NoopCodeHost do
    def open_prs(_repo), do: {:ok, []}
    def pr_review_comments(_repo, _pr_number), do: {:ok, []}
  end

  defmodule NoopWorker do
    def dispatch(_, _, _, _), do: {:ok, ""}
    def exec(_, _, _), do: {:ok, ""}
    def cleanup(_, _, _), do: :ok
    def busy?(_, _), do: false
  end

  defmodule NoopWorkspace do
    def sync_persona(_, _, _, _ \\ []), do: :ok
  end

  defmodule SlowStoredPool do
    def stored_sprites(_role_module, default), do: default

    def stored_sprite_generation(_role_module, default) do
      Process.sleep(150)
      default
    end
  end

  setup do
    stop_conductor_app()
    stop_process(Supervisor)
    stop_process(Conductor.PhaseWorkerRegistry)
    stop_process(Conductor.TaskSupervisor)

    {:ok, _} = Registry.start_link(keys: :unique, name: Conductor.PhaseWorkerRegistry)
    {:ok, _} = Supervisor.start_link()
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_code_host = Application.get_env(:conductor, :code_host_module)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_workspace = Application.get_env(:conductor, :workspace_module)
    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
    orig_phase_worker_sprites = Application.get_env(:conductor, :phase_worker_sprites)

    Application.put_env(:conductor, :code_host_module, NoopCodeHost)
    Application.put_env(:conductor, :worker_module, NoopWorker)
    Application.put_env(:conductor, :workspace_module, NoopWorkspace)
    Application.put_env(:conductor, :phase_worker_supervisor, Supervisor)

    on_exit(fn ->
      stop_process(Conductor.TaskSupervisor)
      stop_process(Supervisor)
      stop_process(Conductor.PhaseWorkerRegistry)

      restore_env(:code_host_module, orig_code_host)
      restore_env(:worker_module, orig_worker)
      restore_env(:workspace_module, orig_workspace)
      restore_env(:phase_worker_supervisor, orig_phase_worker_supervisor)
      restore_env(:phase_worker_sprites, orig_phase_worker_sprites)
    end)

    :ok
  end

  test "returns :ok without starting a worker when sprites are empty" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", [])
    assert PhaseWorker.whereis(Roles.Fixer) == nil
  end

  test "starts a role worker when sprites are present" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"])

    assert pid = PhaseWorker.whereis(Roles.Fixer, "test/repo")
    assert Process.alive?(pid)
    assert PhaseWorker.status(Roles.Fixer, "test/repo").sprites == ["bb-thorn"]
  end

  test "updates an existing worker through the already_started path" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"])
    pid = PhaseWorker.whereis(Roles.Fixer, "test/repo")

    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn-2", "bb-thorn"])

    assert PhaseWorker.whereis(Roles.Fixer, "test/repo") == pid
    assert PhaseWorker.status(Roles.Fixer, "test/repo").sprites == ["bb-thorn", "bb-thorn-2"]
  end

  test "concurrent ensure_worker calls converge on the latest sprite pool during startup" do
    orig_phase_worker_supervisor = Application.get_env(:conductor, :phase_worker_supervisor)
    Application.put_env(:conductor, :phase_worker_supervisor, SlowStoredPool)

    on_exit(fn ->
      restore_env(:phase_worker_supervisor, orig_phase_worker_supervisor)
    end)

    first =
      Task.async(fn ->
        Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"])
      end)

    Process.sleep(25)

    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn-2"])
    assert :ok = Task.await(first, 2_000)

    assert PhaseWorker.status(Roles.Fixer, "test/repo").sprites == ["bb-thorn-2"]
  end

  test "restores the latest sprite pool after a worker restart" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"])
    pid = PhaseWorker.whereis(Roles.Fixer, "test/repo")

    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn", "bb-thorn-2"])

    Process.exit(pid, :kill)

    restarted_pid =
      wait_for(fn ->
        case PhaseWorker.whereis(Roles.Fixer, "test/repo") do
          nil -> nil
          ^pid -> nil
          new_pid -> new_pid
        end
      end)

    assert Process.alive?(restarted_pid)
    assert PhaseWorker.status(Roles.Fixer, "test/repo").sprites == ["bb-thorn", "bb-thorn-2"]
  end

  test "starts separate workers for the same role across repos" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn-a"])
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "other/repo", ["bb-thorn-b"])

    assert PhaseWorker.whereis(Roles.Fixer, "test/repo")
    assert PhaseWorker.whereis(Roles.Fixer, "other/repo")
    assert PhaseWorker.status(Roles.Fixer, "test/repo").sprites == ["bb-thorn-a"]
    assert PhaseWorker.status(Roles.Fixer, "other/repo").sprites == ["bb-thorn-b"]
  end

  test "returns an error when the worker cannot start" do
    stop_process(Conductor.PhaseWorkerRegistry)

    assert match?({:error, _}, Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"]))
  end

  defp wait_for(fun, attempts \\ 20)

  defp wait_for(_fun, 0), do: flunk("timed out waiting for phase worker state")

  defp wait_for(fun, attempts) do
    case fun.() do
      nil ->
        Process.sleep(25)
        wait_for(fun, attempts - 1)

      value ->
        value
    end
  end
end
