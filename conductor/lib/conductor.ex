defmodule Conductor do
  @moduledoc """
  Bitterblossom conductor — issue-to-merged-PR orchestrator.

  Thin state machine that trusts the builder agent.
  The conductor owns authority (lease, merge) and persistence (runs, events).
  The agent owns judgment (code, reviews, revisions).
  """

  defdelegate run_once(opts), to: Conductor.Orchestrator
  defdelegate start_loop(opts), to: Conductor.Orchestrator
end
