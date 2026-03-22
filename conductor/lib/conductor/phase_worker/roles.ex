defmodule Conductor.PhaseWorker.Roles do
  @moduledoc """
  Canonical role modules for phase workers.
  """

  alias Conductor.PhaseWorker.Roles.{Fixer, Polisher}

  @roles %{
    fixer: Fixer,
    polisher: Polisher
  }

  @spec all() :: [module()]
  def all, do: Map.values(@roles)

  @spec by_role(atom()) :: module() | nil
  def by_role(role), do: Map.get(@roles, role)

  @spec fetch!(atom() | module()) :: module()
  def fetch!(role) when is_atom(role) do
    if Enum.member?(all(), role), do: role, else: Map.fetch!(@roles, role)
  end
end
