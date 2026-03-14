defmodule Conductor.Tracker do
  @moduledoc """
  Behaviour for issue trackers that feed work into the conductor.

  Implementations: `Conductor.GitHub` (default).

  Callers obtain the implementation module via Application config:

      Application.get_env(:conductor, :tracker_module, Conductor.GitHub)
  """

  @doc "Return all issues that are eligible to be dispatched."
  @callback list_eligible(repo :: binary(), opts :: keyword()) :: [Conductor.Issue.t()]

  @doc "Fetch a single issue by number."
  @callback get_issue(repo :: binary(), number :: pos_integer()) ::
              {:ok, Conductor.Issue.t()} | {:error, term()}

  @doc "Post a comment on an issue."
  @callback comment(repo :: binary(), issue_number :: pos_integer(), body :: binary()) ::
              :ok | {:error, term()}
end
