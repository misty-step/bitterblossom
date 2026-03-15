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

  @doc "Return true when the issue currently has the given label."
  @callback issue_has_label?(repo :: binary(), issue_number :: pos_integer(), label :: binary()) ::
              {:ok, boolean()} | {:error, term()}

  @doc "Return issue comments normalized to maps with a binary `body` field."
  @callback issue_comments(repo :: binary(), issue_number :: pos_integer()) ::
              {:ok, [map()]} | {:error, term()}
end
