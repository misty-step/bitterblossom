defmodule Conductor.Worker do
  @moduledoc """
  Behaviour for compute substrates that run builder agents.

  Implementations: `Conductor.Sprite` (real sprites via bb CLI).
  Future: Docker containers, local processes, remote VMs.

  All callbacks receive the worker identifier (e.g. sprite name) as the
  first argument so a single implementation can multiplex across workers.
  """

  @doc "Run a shell command on the worker. Returns stdout or error."
  @callback exec(worker :: binary(), command :: binary(), opts :: keyword()) ::
              {:ok, binary()} | {:error, binary(), integer()}

  @doc "Dispatch an agent prompt on the worker. Blocks until agent exits."
  @callback dispatch(
              worker :: binary(),
              prompt :: binary(),
              repo :: binary(),
              opts :: keyword()
            ) :: {:ok, binary()} | {:error, binary(), integer()}

  @doc "Read and JSON-decode an artifact file from the worker filesystem."
  @callback read_artifact(worker :: binary(), path :: binary(), opts :: keyword()) ::
              {:ok, map()} | {:error, term()}

  @doc "Clean up the run workspace on the worker."
  @callback cleanup(worker :: binary(), repo :: binary(), run_id :: binary()) ::
              :ok | {:error, term()}
end
