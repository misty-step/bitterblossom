defmodule Conductor.Canary.Client do
  @moduledoc """
  Thin Canary query and annotation client for responder sprites.

  This module deliberately mirrors Canary's HTTP API closely. It centralizes
  auth, error mapping, and bounded request shapes without embedding incident
  workflow logic into the conductor.
  """

  @type request_opts :: keyword()
  @type body :: map()

  @spec incidents(request_opts()) :: {:ok, body()} | {:error, binary()}
  def incidents(opts \\ []) do
    get("/api/v1/incidents", params: query(opts, [:with_annotation, :without_annotation]))
  end

  @spec report(request_opts()) :: {:ok, body()} | {:error, binary()}
  def report(opts \\ []) do
    get("/api/v1/report", params: query(opts, [:window, :q, :limit, :cursor]))
  end

  @spec timeline(request_opts()) :: {:ok, body()} | {:error, binary()}
  def timeline(opts \\ []) do
    get("/api/v1/timeline",
      params: query(opts, [:service, :window, :limit, :after, :cursor, :event_type])
    )
  end

  @spec incident_annotations(binary()) :: {:ok, body()} | {:error, binary()}
  def incident_annotations(incident_id) when is_binary(incident_id) do
    get("/api/v1/incidents/#{incident_id}/annotations")
  end

  @spec annotate_incident(binary(), map()) :: {:ok, body()} | {:error, binary()}
  def annotate_incident(incident_id, attrs) when is_binary(incident_id) and is_map(attrs) do
    post("/api/v1/incidents/#{incident_id}/annotations", attrs)
  end

  defp get(path, opts \\ []) do
    request(:get, path, opts)
  end

  defp post(path, body) do
    request(:post, path, json: body)
  end

  defp request(method, path, opts) do
    with {:ok, req} <- build_request() do
      case Req.request(req, Keyword.merge([method: method, url: path], opts)) do
        {:ok, %Req.Response{status: status, body: body}} when status in 200..299 ->
          {:ok, body}

        {:ok, %Req.Response{status: status, body: body}} ->
          {:error, format_http_error(status, body)}

        {:error, error} ->
          {:error, Exception.message(error)}
      end
    end
  end

  defp build_request do
    with endpoint when is_binary(endpoint) <- Conductor.Config.canary_endpoint(),
         api_key when is_binary(api_key) <- Conductor.Config.canary_api_key() do
      {:ok,
       Req.new(
         base_url: String.trim_trailing(endpoint, "/"),
         headers: [
           {"authorization", "Bearer #{api_key}"},
           {"accept", "application/json"}
         ]
       )}
    else
      _ -> {:error, "CANARY_ENDPOINT and CANARY_API_KEY must be set"}
    end
  end

  defp query(opts, allowed_keys) do
    opts
    |> Enum.filter(fn {key, value} -> key in allowed_keys and not is_nil(value) end)
    |> Enum.into(%{}, fn {key, value} -> {to_string(key), value} end)
  end

  defp format_http_error(status, %{"detail" => detail}) when is_binary(detail) do
    "Canary API #{status}: #{detail}"
  end

  defp format_http_error(status, body) when is_map(body) do
    "Canary API #{status}: #{Jason.encode!(body)}"
  end

  defp format_http_error(status, body) do
    "Canary API #{status}: #{inspect(body)}"
  end
end
