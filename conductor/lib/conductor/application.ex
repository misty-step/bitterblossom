defmodule Conductor.Application do
  @moduledoc false

  use Application

  @impl true
  def start(_type, _args) do
    children =
      [
        {Phoenix.PubSub, name: Conductor.PubSub},
        Conductor.Store,
        {DynamicSupervisor, name: Conductor.RunSupervisor, strategy: :one_for_one},
        Conductor.Orchestrator
      ] ++ dashboard_children()

    Supervisor.start_link(children, strategy: :one_for_one, name: Conductor.Supervisor)
  end

  # Only start the web endpoint when explicitly enabled (dashboard command sets this).
  defp dashboard_children do
    if Application.get_env(:conductor, :start_dashboard, false) do
      [Conductor.Web.Endpoint]
    else
      []
    end
  end
end
