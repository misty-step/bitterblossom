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

    assert pid = PhaseWorker.whereis(Roles.Fixer)
    assert Process.alive?(pid)
    assert PhaseWorker.status(Roles.Fixer).sprites == ["bb-thorn"]
  end

  test "updates an existing worker through the already_started path" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"])
    pid = PhaseWorker.whereis(Roles.Fixer)

    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn-2", "bb-thorn"])

    assert PhaseWorker.whereis(Roles.Fixer) == pid
    assert PhaseWorker.status(Roles.Fixer).sprites == ["bb-thorn", "bb-thorn-2"]
  end

  test "restores the latest sprite pool after a worker restart" do
    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn"])
    pid = PhaseWorker.whereis(Roles.Fixer)

    assert :ok = Supervisor.ensure_worker(Roles.Fixer, "test/repo", ["bb-thorn", "bb-thorn-2"])

    Process.exit(pid, :kill)

    restarted_pid =
      wait_for(fn ->
        case PhaseWorker.whereis(Roles.Fixer) do
          nil -> nil
          ^pid -> nil
          new_pid -> new_pid
        end
      end)

    assert Process.alive?(restarted_pid)
    assert PhaseWorker.status(Roles.Fixer).sprites == ["bb-thorn", "bb-thorn-2"]
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
