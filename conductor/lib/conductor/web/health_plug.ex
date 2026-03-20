defmodule Conductor.Web.HealthPlug do
  @moduledoc "Minimal liveness endpoint for external watchdogs."

  import Plug.Conn

  def init(opts), do: opts

  def call(conn, _opts) do
    body =
      Jason.encode!(%{
        status: "ok",
        service: "bitterblossom",
        checked_at: DateTime.utc_now() |> DateTime.truncate(:second) |> DateTime.to_iso8601()
      })

    conn
    |> put_resp_content_type("application/json")
    |> send_resp(200, body)
    |> halt()
  end
end
