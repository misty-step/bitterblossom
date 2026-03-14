defmodule Conductor.CodeHost do
  @moduledoc """
  Behaviour for code hosting platforms that own PRs and CI.

  Implementations: `Conductor.GitHub` (GitHub PRs and Actions via gh CLI).
  Future: GitLab, Gitea, Bitbucket.
  """

  @doc "Fetch the raw check list for a pull request."
  @callback get_pr_checks(repo :: binary(), pr_number :: pos_integer()) ::
              {:ok, [map()]} | {:error, term()}

  @doc "Return true when all real CI checks have passed (or are green/neutral/skipped)."
  @callback checks_green?(repo :: binary(), pr_number :: pos_integer()) :: boolean()

  @doc "Merge the pull request using the configured merge method."
  @callback merge(repo :: binary(), pr_number :: pos_integer(), opts :: keyword()) ::
              :ok | {:error, term()}
end
