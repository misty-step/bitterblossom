defmodule Conductor.Tracker do
  @moduledoc """
  Behaviour for issue tracking systems.

  Implementations: `Conductor.GitHub` (GitHub Issues via gh CLI).
  Future: Linear, Jira, GitLab.
  """

  alias Conductor.Issue

  @doc "Return issues that are eligible for dispatch (open, labelled, ready)."
  @callback list_eligible(repo :: binary(), opts :: keyword()) :: [Issue.t()]

  @doc "Fetch a single issue by number."
  @callback get_issue(repo :: binary(), number :: pos_integer()) ::
              {:ok, Issue.t()} | {:error, term()}

  @doc "Post a comment on an issue."
  @callback comment(repo :: binary(), issue_number :: pos_integer(), body :: binary()) ::
              :ok | {:error, term()}
end
