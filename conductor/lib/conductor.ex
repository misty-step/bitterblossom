defmodule Conductor do
  @moduledoc """
  Bitterblossom — agent-first software factory.

  Infrastructure layer: provisions sprites, bootstraps harnesses,
  dispatches autonomous agent loops, monitors health.

  Start with `mix conductor start --fleet ../fleet.toml`.
  """

  defdelegate launch_fleet(fleet_path), to: Conductor.Application
end
