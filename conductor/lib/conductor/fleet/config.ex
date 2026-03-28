defmodule Conductor.Fleet.Config do
  @moduledoc """
  Shared fleet runtime configuration lookups.

  Keeps sprite-level repo resolution and launcher indirection in one place so
  boot and recovery paths cannot drift apart.
  """

  @spec sprite_repo(map(), binary() | nil) :: binary() | nil
  def sprite_repo(sprite, fallback_repo), do: Map.get(sprite, :repo, fallback_repo)

  @spec launcher_module() :: module()
  def launcher_module do
    Application.get_env(:conductor, :launcher_module, Conductor.Launcher)
  end
end
