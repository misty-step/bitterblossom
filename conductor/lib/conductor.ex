defmodule Conductor do
  @moduledoc """
  Bitterblossom conductor — always-on issue-to-merged-PR service.

  Start with `mix conductor start`. The conductor reads fleet.toml,
  provisions sprites, and runs continuously: picking issues, building,
  polishing, and merging.
  """

  defdelegate boot_fleet(fleet_path), to: Conductor.Application
end
