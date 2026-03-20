defmodule Conductor.Web.Router do
  @moduledoc "Routes for the operator dashboard."

  use Phoenix.Router
  import Phoenix.LiveView.Router

  pipeline :browser do
    plug(:fetch_session)
    plug(:put_root_layout, html: {Conductor.Web.Layouts, :root})
    plug(:protect_from_forgery)
    plug(:put_secure_browser_headers)
  end

  scope "/" do
    get("/healthz", Conductor.Web.HealthPlug, [])
  end

  scope "/" do
    pipe_through(:browser)
    live("/", Conductor.Web.DashboardLive, :index)
  end
end
