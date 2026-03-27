defmodule Conductor.PhaseWorker.Supervisor do
  @moduledoc """
  Dynamic supervisor for role-based phase workers.
  """

  use DynamicSupervisor

  alias Conductor.PhaseWorker
  alias Conductor.PhaseWorker.Roles
  @sprite_pool_env :phase_worker_sprites

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
    pool = store_sprites(role_module, repo, sprites)

    if sprites == [] and PhaseWorker.whereis(role_module, repo) == nil do
      :ok
    else
      child_opts =
        [
          repo: repo,
          role_module: role_module,
          sprites: pool.sprites,
          sprite_generation: pool.generation
        ] ++ opts

      case DynamicSupervisor.start_child(__MODULE__, {PhaseWorker, child_opts}) do
        {:ok, _pid} ->
          :ok

        {:error, {:already_started, _pid}} ->
          PhaseWorker.update_sprites(role_module, repo, pool.sprites, pool.generation)

        {:error, {:already_present, _pid}} ->
          PhaseWorker.update_sprites(role_module, repo, pool.sprites, pool.generation)

        {:error, reason} ->
          {:error, reason}
      end
    end
  end

  @spec stored_sprites(atom() | module(), [binary()]) :: [binary()]
  def stored_sprites(role_module, default \\ []) do
    stored_sprites(role_module, nil, default)
  end

  @spec stored_sprites(atom() | module(), binary() | nil, [binary()]) :: [binary()]
  def stored_sprites(role_module, repo, default) do
    Application.get_env(:conductor, @sprite_pool_env, %{})
    |> Map.get(pool_key(role_module, repo))
    |> case do
      %{sprites: sprites} -> sprites
      nil -> default
      sprites when is_list(sprites) -> sprites
    end
  end

  @spec stored_sprite_generation(atom() | module(), integer()) :: integer()
  def stored_sprite_generation(role_module, default \\ 0) do
    stored_sprite_generation(role_module, nil, default)
  end

  @spec stored_sprite_generation(atom() | module(), binary() | nil, integer()) :: integer()
  def stored_sprite_generation(role_module, repo, default) do
    Application.get_env(:conductor, @sprite_pool_env, %{})
    |> Map.get(pool_key(role_module, repo))
    |> case do
      %{generation: generation} when is_integer(generation) -> generation
      _ -> default
    end
  end

  defp store_sprites(role_module, repo, sprites) do
    stored = Application.get_env(:conductor, @sprite_pool_env, %{})
    pool = %{sprites: sprites, generation: System.unique_integer([:positive, :monotonic])}

    Application.put_env(
      :conductor,
      @sprite_pool_env,
      Map.put(stored, pool_key(role_module, repo), pool)
    )

    pool
  end

  defp pool_key(role_module, nil), do: Roles.fetch!(role_module).role()
  defp pool_key(role_module, repo), do: {Roles.fetch!(role_module).role(), repo}
end
