defmodule Conductor.Recovery do
  @moduledoc """
  Recovery and incident classification for conductor runs.

  Separates semantic readiness from mechanical check state. Classifies failure
  kinds and drives policy decisions: waiver-eligible merges, bounded replay, or
  true semantic blocks.

  ## Failure classes

  - `:transient_infra`     — timeout, network hiccup, ephemeral infra noise
  - `:auth_config`         — missing credential or misconfigured service
  - `:semantic_code`       — actual test or compile failure in the PR
  - `:flaky_check`         — known intermittent check (matches flaky signature)
  - `:known_false_red`     — trusted external surface failed for a known bogus reason
  - `:human_policy_block`  — check explicitly requires human approval
  - `:unknown`             — unclassified; requires investigation

  ## Policy decisions (returned by evaluate_with_policy/2)

  - `:pass`                 — all checks green; proceed to merge
  - `{:waiver_eligible, [check]}`  — all non-green are on trusted surfaces; may merge with waiver
  - `{:retryable, [check]}` — non-green checks are transient/flaky; replay applies
  - `{:pending}`            — checks still in-progress
  - `{:blocked, [check]}`   — non-green checks are true code failures or unclassified
  """

  @type failure_class ::
          :transient_infra
          | :auth_config
          | :semantic_code
          | :flaky_check
          | :known_false_red
          | :human_policy_block
          | :unknown

  @type policy_decision ::
          :pass
          | {:waiver_eligible, [map()]}
          | {:retryable, [map()]}
          | :pending
          | {:blocked, [map()]}

  @green ~w(SUCCESS success NEUTRAL neutral SKIPPED skipped)
  @active_statuses ~w(IN_PROGRESS QUEUED PENDING WAITING REQUESTED in_progress queued pending waiting requested)

  # Patterns that indicate transient infra noise when in error messages or check names.
  @transient_patterns ~w(timeout network connection refused flaky intermittent unavailable)

  # Patterns that indicate auth/config issues.
  @auth_patterns ~w(auth credential token forbidden unauthorized config missing)

  @doc """
  Classify a single failed check against a list of trusted external surfaces.

  A check is `known_false_red` when its name (or app name) appears in `trusted_surfaces`.
  Otherwise the classification falls back to pattern matching on the check name.
  """
  @spec classify_check(map(), [binary()]) :: failure_class()
  def classify_check(check, trusted_surfaces \\ []) do
    name = check["name"] || ""
    app = check["app"] || ""

    cond do
      surface_match?(name, trusted_surfaces) or surface_match?(app, trusted_surfaces) ->
        :known_false_red

      matches_any?(name, @transient_patterns) ->
        :transient_infra

      matches_any?(name, @auth_patterns) ->
        :auth_config

      true ->
        :unknown
    end
  end

  @doc """
  Classify a failure reason string (e.g. from a timeout event or builder error message).
  Used when classifying run-level failures that don't map 1:1 to a single check.
  """
  @spec classify_reason(binary()) :: failure_class()
  def classify_reason(reason) when is_binary(reason) do
    r = String.downcase(reason)

    cond do
      matches_any?(r, ~w(timeout network connection)) -> :transient_infra
      matches_any?(r, ~w(auth token credential unauthorized forbidden)) -> :auth_config
      matches_any?(r, ~w(human policy)) -> :human_policy_block
      matches_any?(r, ~w(test compile build)) -> :semantic_code
      true -> :unknown
    end
  end

  def classify_reason(_), do: :unknown

  @doc """
  Evaluate a check list with recovery-aware policy.

  Returns a policy decision that distinguishes waiver-eligible False-reds,
  retryable transients, still-pending checks, and true semantic blockers.

  Trusted surfaces are check names or app names that may be waived when they
  fail for documented reasons.
  """
  @spec evaluate_with_policy([map()], [binary()]) :: policy_decision()
  def evaluate_with_policy(checks, trusted_surfaces \\ []) do
    real =
      Enum.filter(checks, fn c ->
        not is_nil(c["conclusion"]) or c["status"] in @active_statuses
      end)

    cond do
      real == [] ->
        :pending

      Enum.any?(real, fn c -> is_nil(c["conclusion"]) end) ->
        :pending

      Enum.all?(real, fn c -> c["conclusion"] in @green end) ->
        :pass

      true ->
        failing = Enum.reject(real, fn c -> c["conclusion"] in @green end)
        classify_failing(failing, trusted_surfaces)
    end
  end

  @doc """
  Convert a failure class atom to a canonical string for storage.
  """
  @spec failure_class_to_string(failure_class()) :: binary()
  def failure_class_to_string(atom) when is_atom(atom), do: Atom.to_string(atom)

  @doc """
  Whether a failure class is retryable under replay policy.
  """
  @spec retryable?(failure_class()) :: boolean()
  def retryable?(:transient_infra), do: true
  def retryable?(:flaky_check), do: true
  def retryable?(_), do: false

  @doc """
  Whether a failure class is waivable under false-red policy.
  """
  @spec waivable?(failure_class()) :: boolean()
  def waivable?(:known_false_red), do: true
  def waivable?(_), do: false

  # --- Private ---

  defp classify_failing(failing, trusted_surfaces) do
    classified = Enum.map(failing, &{&1, classify_check(&1, trusted_surfaces)})

    false_reds = for {c, :known_false_red} <- classified, do: c
    retryables = for {c, cls} <- classified, retryable?(cls), do: c
    blockers = for {c, cls} <- classified, not retryable?(cls), cls != :known_false_red, do: c

    cond do
      blockers != [] -> {:blocked, blockers}
      false_reds != [] and retryables == [] -> {:waiver_eligible, false_reds}
      true -> {:retryable, retryables ++ false_reds}
    end
  end

  defp surface_match?("", _), do: false

  defp surface_match?(name, surfaces) do
    lower = String.downcase(name)
    Enum.any?(surfaces, fn s -> String.downcase(s) == lower end)
  end

  defp matches_any?(name, patterns) do
    lower = String.downcase(name)
    Enum.any?(patterns, fn p -> String.contains?(lower, p) end)
  end
end
