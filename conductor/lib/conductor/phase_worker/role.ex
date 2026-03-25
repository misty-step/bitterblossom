defmodule Conductor.PhaseWorker.Role do
  @moduledoc """
  Role-specific callbacks for `Conductor.PhaseWorker`.
  """

  @callback role() :: atom()
  @callback persona_role() :: atom()
  @callback event_prefix() :: binary()
  @callback find_work(repo :: binary(), code_host :: module()) ::
              {:ok, [map()]} | {:error, term()}
  @callback eligible?(work_item :: map(), state :: map()) :: boolean()
  @callback enrich_context(work_item :: map(), repo :: binary(), code_host :: module()) :: map()
  @callback build_prompt(work_item :: map(), context :: map(), opts :: keyword()) :: binary()
  @callback dispatch_opts(work_item :: map()) :: keyword()
  @callback work_ref(work_item :: map()) :: pos_integer()
  @callback dispatch_log_message(work_item :: map()) :: binary()
end
