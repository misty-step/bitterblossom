defmodule Conductor.CodeHost do
  @moduledoc """
  Behaviour for code hosting platforms that gate and complete PRs.

  Implementations: `Conductor.GitHub` (default).

  Callers obtain the implementation module via Application config:

      Application.get_env(:conductor, :code_host_module, Conductor.GitHub)
  """

  @doc "Return true when all required checks on a PR have passed."
  @callback checks_green?(repo :: binary(), pr_number :: pos_integer()) :: boolean()

  @doc "Merge a pull request using the configured merge strategy."
  @callback merge(repo :: binary(), pr_number :: pos_integer(), opts :: keyword()) ::
              :ok | {:error, term()}
end
