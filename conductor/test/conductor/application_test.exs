defmodule Conductor.ApplicationTest do
  use ExUnit.Case, async: false

  import Conductor.TestSupport.ProcessHelpers

  defmodule FailingDashboardEndpoint do
    def start_link, do: {:error, :boom}

    def child_spec(_opts) do
      %{
        id: __MODULE__,
        start: {__MODULE__, :start_link, []}
      }
    end
  end

  test "maps renamed phase worker roles to sprite display names" do
    assert Conductor.Application.role_display_name(:fixer) == "thorn"
    assert Conductor.Application.role_display_name(:polisher) == "fern"
  end

  test "falls back to the raw role name for unmapped roles" do
    assert Conductor.Application.role_display_name(:builder) == "builder"
    assert Conductor.Application.role_display_name(:triage) == "triage"
  end

  test "dashboard_port returns the configured endpoint port" do
    original = Application.get_env(:conductor, Conductor.Web.Endpoint)

    Application.put_env(:conductor, Conductor.Web.Endpoint, http: [port: 4321])

    on_exit(fn ->
      if original do
        Application.put_env(:conductor, Conductor.Web.Endpoint, original)
      else
        Application.delete_env(:conductor, Conductor.Web.Endpoint)
      end
    end)

    assert Conductor.Application.dashboard_port() == 4321
  end

  test "start_dashboard returns an error when the endpoint child fails to start" do
    original_endpoint = Application.get_env(:conductor, :dashboard_endpoint_module)
    original_start_dashboard = Application.get_env(:conductor, :start_dashboard)
    original_endpoint_config = Application.get_env(:conductor, Conductor.Web.Endpoint)

    stop_process(Conductor.Supervisor)
    {:ok, _pid} = Supervisor.start_link([], strategy: :one_for_one, name: Conductor.Supervisor)

    Application.put_env(:conductor, :dashboard_endpoint_module, FailingDashboardEndpoint)
    Application.put_env(:conductor, :start_dashboard, true)
    Application.put_env(:conductor, Conductor.Web.Endpoint, http: [port: 4000], server: false)

    on_exit(fn ->
      stop_process(Conductor.Supervisor)

      if original_endpoint do
        Application.put_env(:conductor, :dashboard_endpoint_module, original_endpoint)
      else
        Application.delete_env(:conductor, :dashboard_endpoint_module)
      end

      if is_boolean(original_start_dashboard) do
        Application.put_env(:conductor, :start_dashboard, original_start_dashboard)
      else
        Application.delete_env(:conductor, :start_dashboard)
      end

      if original_endpoint_config do
        Application.put_env(:conductor, Conductor.Web.Endpoint, original_endpoint_config)
      else
        Application.delete_env(:conductor, Conductor.Web.Endpoint)
      end
    end)

    assert {:error, reason} = Conductor.Application.start_dashboard()
    assert inspect(reason) =~ ":boom"
  end
end
