defmodule Conductor.Web.Endpoint do
  @moduledoc "Phoenix endpoint for the operator dashboard."

  @session_options [
    store: :cookie,
    key: "_conductor_session",
    signing_salt: "bb_session"
  ]

  use Phoenix.Endpoint, otp_app: :conductor

  socket("/live", Phoenix.LiveView.Socket, websocket: [connect_info: [session: @session_options]])

  plug(Plug.Static,
    at: "/assets/phoenix",
    from: {:phoenix, "priv/static"},
    gzip: false
  )

  plug(Plug.Static,
    at: "/assets/live_view",
    from: {:phoenix_live_view, "priv/static"},
    gzip: false
  )

  plug(Plug.RequestId)
  plug(Plug.Telemetry, event_prefix: [:conductor, :endpoint])

  plug(Plug.Session, @session_options)
  plug(Conductor.Web.Router)
end
