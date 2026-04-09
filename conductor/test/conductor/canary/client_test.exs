defmodule Conductor.Canary.ClientTest do
  use ExUnit.Case, async: false

  alias Conductor.Canary.Client
  import Plug.Conn

  defmodule TestPlug do
    use Plug.Router

    plug(:match)

    plug(Plug.Parsers,
      parsers: [:json],
      pass: ["application/json"],
      json_decoder: Jason
    )

    plug(:dispatch)

    get "/api/v1/incidents" do
      notify({:request, :incidents, conn.query_params, auth_header(conn)})

      json(conn, %{
        incidents: [
          %{
            id: "INC-123",
            service: "volume",
            state: "investigating",
            severity: "high",
            title: "Volume is failing health checks"
          }
        ]
      })
    end

    get "/api/v1/report" do
      notify({:request, :report, conn.query_params, auth_header(conn)})

      case conn.query_params["q"] do
        "bad" ->
          conn
          |> put_resp_content_type("application/json")
          |> send_resp(422, Jason.encode!(%{detail: "Invalid q parameter."}))

        _ ->
          json(conn, %{status: "degraded", summary: "1 service degraded."})
      end
    end

    get "/api/v1/timeline" do
      notify({:request, :timeline, conn.query_params, auth_header(conn)})

      json(conn, %{
        summary: "2 recent events.",
        events: [
          %{
            id: "EVT-1",
            service: "volume",
            event: "incident.opened",
            severity: "high",
            summary: "Incident opened for volume.",
            created_at: "2026-04-08T12:00:00Z"
          }
        ]
      })
    end

    get "/api/v1/incidents/INC-123/annotations" do
      notify({:request, :incident_annotations, auth_header(conn)})

      json(conn, %{annotations: [%{agent: "tansy", action: "bitterblossom.claimed"}]})
    end

    post "/api/v1/incidents/INC-123/annotations" do
      notify({:request, :annotate_incident, conn.body_params, auth_header(conn)})

      conn
      |> put_resp_content_type("application/json")
      |> send_resp(
        201,
        Jason.encode!(%{
          created_at: "2026-04-08T12:02:00Z",
          agent: conn.body_params["agent"],
          action: conn.body_params["action"]
        })
      )
    end

    match _ do
      send_resp(conn, 404, "not found")
    end

    defp json(conn, body) do
      conn
      |> put_resp_content_type("application/json")
      |> send_resp(200, Jason.encode!(body))
    end

    defp auth_header(conn) do
      conn |> get_req_header("authorization") |> List.first()
    end

    defp notify(message) do
      if pid = Application.get_env(:conductor, :canary_test_pid) do
        send(pid, message)
      end
    end
  end

  setup do
    original_endpoint = System.get_env("CANARY_ENDPOINT")
    original_api_key = System.get_env("CANARY_API_KEY")
    original_test_pid = Application.get_env(:conductor, :canary_test_pid)

    port = free_port()

    start_supervised!({Bandit, plug: TestPlug, scheme: :http, port: port})
    Application.put_env(:conductor, :canary_test_pid, self())
    System.put_env("CANARY_ENDPOINT", "http://127.0.0.1:#{port}")
    System.put_env("CANARY_API_KEY", "canary-test-key")

    on_exit(fn ->
      restore_env("CANARY_ENDPOINT", original_endpoint)
      restore_env("CANARY_API_KEY", original_api_key)

      if original_test_pid,
        do: Application.put_env(:conductor, :canary_test_pid, original_test_pid),
        else: Application.delete_env(:conductor, :canary_test_pid)
    end)

    :ok
  end

  test "lists incidents with query filters and bearer auth" do
    assert {:ok, %{"incidents" => [%{"id" => "INC-123"}]}} =
             Client.incidents(without_annotation: "bitterblossom.claimed")

    assert_received {:request, :incidents, %{"without_annotation" => "bitterblossom.claimed"},
                     "Bearer canary-test-key"}
  end

  test "fetches timeline with bounded query params" do
    assert {:ok, %{"events" => [%{"event" => "incident.opened"}]}} =
             Client.timeline(service: "volume", window: "24h", limit: 50)

    assert_received {:request, :timeline,
                     %{"limit" => "50", "service" => "volume", "window" => "24h"},
                     "Bearer canary-test-key"}
  end

  test "lists incident annotations" do
    assert {:ok, %{"annotations" => [%{"action" => "bitterblossom.claimed"}]}} =
             Client.incident_annotations("INC-123")

    assert_received {:request, :incident_annotations, "Bearer canary-test-key"}
  end

  test "creates incident annotations with json metadata" do
    assert {:ok, %{"agent" => "tansy", "action" => "bitterblossom.claimed"}} =
             Client.annotate_incident("INC-123", %{
               agent: "tansy",
               action: "bitterblossom.claimed",
               metadata: %{service: "volume"}
             })

    assert_received {:request, :annotate_incident, body, "Bearer canary-test-key"}
    assert body["agent"] == "tansy"
    assert body["action"] == "bitterblossom.claimed"
    assert body["metadata"] == %{"service" => "volume"}
  end

  test "maps Canary validation errors into readable messages" do
    assert {:error, "Canary API 422: Invalid q parameter."} = Client.report(q: "bad")
  end

  test "fails clearly when Canary credentials are missing" do
    System.delete_env("CANARY_ENDPOINT")
    System.delete_env("CANARY_API_KEY")

    assert {:error, "CANARY_ENDPOINT and CANARY_API_KEY must be set"} = Client.report()
  end

  defp free_port do
    {:ok, socket} = :gen_tcp.listen(0, [:binary, active: false, ip: {127, 0, 0, 1}])
    {:ok, port} = :inet.port(socket)
    :gen_tcp.close(socket)
    port
  end

  defp restore_env(key, nil), do: System.delete_env(key)
  defp restore_env(key, value), do: System.put_env(key, value)
end
