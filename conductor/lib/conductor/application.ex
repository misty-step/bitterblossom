defmodule Conductor.Application do
  @moduledoc false

  use Application

  @impl true
  def start(_type, _args) do
    children = [
      Conductor.Store,
      Conductor.Retro,
      {DynamicSupervisor, name: Conductor.RunSupervisor, strategy: :one_for_one},
      Conductor.Orchestrator
    ]

    Supervisor.start_link(children, strategy: :one_for_one, name: Conductor.Supervisor)
  end
end
