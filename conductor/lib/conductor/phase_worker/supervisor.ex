defmodule Conductor.PhaseWorker.Supervisor do
  @moduledoc """
  Dynamic supervisor for role-based phase workers.
  """

  use DynamicSupervisor

  alias Conductor.PhaseWorker
  alias Conductor.PhaseWorker.Roles

  def start_link(opts \\ []) do
    DynamicSupervisor.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @impl true
  def init(_opts) do
    DynamicSupervisor.init(strategy: :one_for_one)
  end

  @spec ensure_worker(atom() | module(), binary(), [binary()], keyword()) ::
          :ok | {:error, term()}
  def ensure_worker(role_module, repo, sprites, opts \\ []) do
    role_module = Roles.fetch!(role_module)
    sprites = sprites |> Enum.uniq() |> Enum.sort()

    case PhaseWorker.whereis(role_module) do
      nil ->
        if sprites == [] do
          :ok
        else
          child_opts = [repo: repo, role_module: role_module, sprites: sprites] ++ opts

          case DynamicSupervisor.start_child(__MODULE__, {PhaseWorker, child_opts}) do
            {:ok, _pid} -> :ok
            {:error, {:already_started, _pid}} -> PhaseWorker.update_sprites(role_module, sprites)
            {:error, reason} -> {:error, reason}
          end
        end

      _pid ->
        PhaseWorker.update_sprites(role_module, sprites)
    end
  end
end
