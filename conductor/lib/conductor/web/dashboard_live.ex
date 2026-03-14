defmodule Conductor.Web.DashboardLive do
  @moduledoc """
  Real-time operator dashboard.

  Shows fleet health, active run progress, and turn-count proxy for cost.
  Subscribes to the "dashboard" PubSub topic and refreshes on any run update.
  """

  use Phoenix.LiveView

  @topic "dashboard"
  @run_limit 50

  @impl true
  def mount(_params, _session, socket) do
    if connected?(socket) do
      Phoenix.PubSub.subscribe(Conductor.PubSub, @topic)
      :timer.send_interval(30_000, self(), :refresh)
    end

    runs = Conductor.Store.list_runs(limit: @run_limit)
    {:ok, assign(socket, runs: runs, stats: compute_stats(runs))}
  end

  @impl true
  def handle_info(:runs_updated, socket) do
    runs = Conductor.Store.list_runs(limit: @run_limit)
    {:noreply, assign(socket, runs: runs, stats: compute_stats(runs))}
  end

  def handle_info(:refresh, socket) do
    runs = Conductor.Store.list_runs(limit: @run_limit)
    {:noreply, assign(socket, runs: runs, stats: compute_stats(runs))}
  end

  @impl true
  def render(assigns) do
    ~H"""
    <h1>Bitterblossom Dashboard</h1>

    <div class="stats">
      <div class="stat">
        <div class="stat-label">Active</div>
        <div class="stat-value"><%= @stats.active %></div>
      </div>
      <div class="stat">
        <div class="stat-label">Merged (24h)</div>
        <div class="stat-value"><%= @stats.merged_24h %></div>
      </div>
      <div class="stat">
        <div class="stat-label">Blocked</div>
        <div class="stat-value"><%= @stats.blocked %></div>
      </div>
      <div class="stat">
        <div class="stat-label">Total Turns</div>
        <div class="stat-value"><%= @stats.total_turns %></div>
      </div>
    </div>

    <table>
      <thead>
        <tr>
          <th>Run ID</th>
          <th>Repo</th>
          <th>Issue</th>
          <th>Phase</th>
          <th>Status</th>
          <th>Sprite</th>
          <th>Turns</th>
          <th>PR</th>
          <th>Updated</th>
        </tr>
      </thead>
      <tbody>
        <tr :for={run <- @runs}>
          <td><code><%= short_id(run["run_id"]) %></code></td>
          <td><%= run["repo"] %></td>
          <td>
            <a href={"https://github.com/#{run["repo"]}/issues/#{run["issue_number"]}"} target="_blank" style="color: #58a6ff;">
              #<%= run["issue_number"] %>
            </a>
            <%= if run["issue_title"], do: " – #{run["issue_title"]}" %>
          </td>
          <td class={"phase-#{run["phase"]}"}><%= run["phase"] %></td>
          <td><%= run["status"] %></td>
          <td><%= run["builder_sprite"] || "–" %></td>
          <td><%= run["turn_count"] || 0 %></td>
          <td>
            <a :if={run["pr_url"]} href={run["pr_url"]} target="_blank" style="color: #58a6ff;">
              #<%= run["pr_number"] %>
            </a>
            <span :if={!run["pr_url"]}>–</span>
          </td>
          <td style="color: #8b949e;"><%= format_time(run["updated_at"]) %></td>
        </tr>
        <tr :if={@runs == []}>
          <td colspan="9" style="text-align: center; color: #8b949e; padding: 24px;">No runs yet</td>
        </tr>
      </tbody>
    </table>

    <p class="refresh-note">Auto-refreshes on run updates · 30s poll fallback</p>
    """
  end

  # --- Helpers ---

  defp compute_stats(runs) do
    now = DateTime.utc_now()

    %{
      active: Enum.count(runs, &active?/1),
      merged_24h: Enum.count(runs, &merged_in_24h?(&1, now)),
      blocked: Enum.count(runs, fn r -> r["status"] == "blocked" end),
      total_turns: Enum.sum(Enum.map(runs, fn r -> r["turn_count"] || 0 end))
    }
  end

  defp active?(run), do: run["completed_at"] == nil and run["status"] not in ["blocked", "failed"]

  defp merged_in_24h?(run, now) do
    run["phase"] == "merged" and
      run["completed_at"] != nil and
      within_hours?(run["completed_at"], now, 24)
  end

  defp within_hours?(iso_str, now, hours) do
    case DateTime.from_iso8601(iso_str) do
      {:ok, dt, _} -> DateTime.diff(now, dt, :hour) <= hours
      _ -> false
    end
  end

  defp short_id(nil), do: "–"
  defp short_id(id), do: String.slice(id, 0, 20)

  defp format_time(nil), do: "–"

  defp format_time(iso_str) do
    case DateTime.from_iso8601(iso_str) do
      {:ok, dt, _} -> Calendar.strftime(dt, "%m-%d %H:%M")
      _ -> iso_str
    end
  end
end
