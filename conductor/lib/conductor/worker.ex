defmodule Conductor.Worker do
  @moduledoc """
  Behaviour for sprite/compute workers that execute builder tasks.

  Implementations: `Conductor.Sprite` (default).

  Callers obtain the implementation module via Application config:

      Application.get_env(:conductor, :worker_module, Conductor.Sprite)

  This allows test doubles and alternative backends (Docker, etc.)
  without modifying RunServer or Orchestrator.
  """

  @doc "Run an arbitrary command on the worker. Returns stdout or error."
  @callback exec(worker :: binary(), command :: binary(), opts :: keyword()) ::
              {:ok, binary()} | {:error, binary(), integer()}

  @doc "Dispatch a builder agent to work on a prompt inside a repo worktree."
  @callback dispatch(worker :: binary(), prompt :: binary(), repo :: binary(), opts :: keyword()) ::
              {:ok, binary()} | {:error, binary(), integer()}

  @doc "Remove the run worktree from the worker filesystem."
  @callback cleanup(worker :: binary(), repo :: binary(), run_id :: binary()) ::
              :ok | {:error, term()}
end
