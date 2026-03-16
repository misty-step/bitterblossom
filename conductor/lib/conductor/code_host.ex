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

  @doc "List open PRs with a given label."
  @callback labeled_prs(repo :: binary(), label :: binary()) ::
              {:ok, [map()]} | {:error, term()}

  @doc "Return true when at least one completed check has a non-green conclusion."
  @callback checks_failed?(repo :: binary(), pr_number :: pos_integer()) :: boolean()

  @doc "List all open PRs with CI and label metadata."
  @callback open_prs(repo :: binary()) :: {:ok, [map()]} | {:error, term()}

  @doc "Fetch review comments on a PR."
  @callback pr_review_comments(repo :: binary(), pr_number :: pos_integer()) ::
              {:ok, [map()]} | {:error, term()}

  @doc "Fetch CI failure logs for a PR."
  @callback pr_ci_failure_logs(repo :: binary(), pr_number :: pos_integer()) ::
              {:ok, binary()} | {:error, term()}

  @doc "Add a label to a PR."
  @callback add_label(repo :: binary(), pr_number :: pos_integer(), label :: binary()) ::
              :ok | {:error, term()}

  @doc "Find an open PR associated with the given issue number."
  @callback find_open_pr(repo :: binary(), issue_number :: pos_integer()) ::
              {:ok, map()} | {:error, :not_found}

  @doc "Return the state of a PR: OPEN, MERGED, or CLOSED."
  @callback pr_state(repo :: binary(), pr_number :: pos_integer()) ::
              {:ok, binary()} | {:error, term()}
end
