defmodule Conductor.Web.DashboardLive do
  @moduledoc """
  Real-time operator dashboard.

  Shows fleet status and recent events. Subscribes to PubSub for live updates.
  """

  use Phoenix.LiveView

  @topic "dashboard"
  @event_limit 50

  @impl true
  def mount(_params, _session, socket) do
    if connected?(socket) do
      Phoenix.PubSub.subscribe(Conductor.PubSub, @topic)
      :timer.send_interval(30_000, self(), :refresh)
    end

    events = Conductor.Store.list_all_events(limit: @event_limit)
    {:ok, assign(socket, events: events)}
  end

  @impl true
  def handle_info(:runs_updated, socket) do
    events = Conductor.Store.list_all_events(limit: @event_limit)
    {:noreply, assign(socket, events: events)}
  end

  def handle_info(:refresh, socket) do
    events = Conductor.Store.list_all_events(limit: @event_limit)
    {:noreply, assign(socket, events: events)}
  end

  @impl true
  def render(assigns) do
    ~H"""
    <h1>Bitterblossom Dashboard</h1>

    <table>
      <thead>
        <tr>
          <th>Time</th>
          <th>Source</th>
          <th>Event</th>
        </tr>
      </thead>
      <tbody>
        <tr :for={event <- @events}>
          <td style="color: #8b949e;"><%= format_time(event["created_at"]) %></td>
          <td><code><%= event["run_id"] %></code></td>
          <td><%= event["event_type"] %></td>
        </tr>
        <tr :if={@events == []}>
          <td colspan="3" style="text-align: center; color: #8b949e; padding: 24px;">No events yet</td>
        </tr>
      </tbody>
    </table>

    <p class="refresh-note">Auto-refreshes on updates · 30s poll fallback</p>
    """
  end

  defp format_time(nil), do: "–"

  defp format_time(iso_str) do
    case DateTime.from_iso8601(iso_str) do
      {:ok, dt, _} -> Calendar.strftime(dt, "%m-%d %H:%M")
      _ -> iso_str
    end
  end
end
