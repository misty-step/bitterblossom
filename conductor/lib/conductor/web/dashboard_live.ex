defmodule Conductor.Web.DashboardLive do
  @moduledoc """
  Real-time operator dashboard.

  Shows fleet health, phase worker state, governor cooldowns, recent events,
  run history, and the recent run table from one LiveView.
  """

  use Phoenix.LiveView

  alias Conductor.{Config, Store}

  @topic "dashboard"
  @run_limit 50
  @history_limit 200
  @event_limit 50
  @event_filters [
    {"all", "All"},
    {"fleet", "Fleet"},
    {"fixer", "Thorn"},
    {"polisher", "Fern"},
    {"runs", "Runs"}
  ]

  @impl true
  def mount(_params, _session, socket) do
    if connected?(socket) do
      Phoenix.PubSub.subscribe(Conductor.PubSub, @topic)
      :timer.send_interval(30_000, self(), :refresh)
    end

    {:ok, refresh_dashboard(assign(socket, event_source: "all"))}
  end

  @impl true
  def handle_info(:runs_updated, socket), do: {:noreply, refresh_dashboard(socket)}

  def handle_info(:refresh, socket), do: {:noreply, refresh_dashboard(socket)}

  @impl true
  def handle_event("filter-events", %{"source" => source}, socket) do
    {:noreply, refresh_dashboard(assign(socket, event_source: source))}
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

    <section>
      <h2>Fleet Health</h2>
      <p>
        Check interval: <%= format_interval(@fleet.interval_ms) %>
        · Last check: <%= format_time(@fleet.last_check_at) %>
      </p>

      <table>
        <thead>
          <tr>
            <th>Sprite</th>
            <th>Role</th>
            <th>Status</th>
            <th>Last Probe</th>
            <th>Failures</th>
          </tr>
        </thead>
        <tbody>
          <tr :for={sprite <- @fleet.sprites}>
            <td><%= sprite.name %></td>
            <td><%= sprite.role %></td>
            <td><%= sprite.status %></td>
            <td><%= format_time(sprite.last_probe_at) %></td>
            <td><%= sprite.consecutive_failures %></td>
          </tr>
          <tr :if={@fleet.sprites == []}>
            <td colspan="5" style="text-align: center; color: #8b949e; padding: 24px;">
              No fleet health available
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section>
      <h2>Phase Workers</h2>

      <table>
        <thead>
          <tr>
            <th>Worker</th>
            <th>Sprite</th>
            <th>Health</th>
            <th>Failures</th>
            <th>Current Work</th>
            <th>Poll</th>
            <th>Last Dispatch</th>
            <th>Last Completion</th>
          </tr>
        </thead>
        <tbody>
          <tr :for={worker <- @phase_workers}>
            <td><%= worker.label %></td>
            <td><%= worker.sprite %></td>
            <td><%= worker.health %></td>
            <td><%= worker.failure_count %></td>
            <td><%= format_in_flight(worker.in_flight) %></td>
            <td><%= format_interval(worker.poll_ms) %></td>
            <td><%= format_time(worker.last_dispatch_at) %></td>
            <td><%= format_time(worker.last_completion_at) %></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section>
      <h2>Governor</h2>
      <p>
        Repo: <%= @governor.repo || "–" %>
        · max starts/tick: <%= @governor.max_starts_per_tick %>
        · max concurrent runs: <%= @governor.max_concurrent_runs %>
      </p>

      <table>
        <thead>
          <tr>
            <th>Issue</th>
            <th>Failure Streak</th>
            <th>Cooldown Window</th>
            <th>Time Remaining</th>
            <th>Last Failed</th>
          </tr>
        </thead>
        <tbody>
          <tr :for={issue <- @governor.cooldowns}>
            <td>
              <a href={issue.url} target="_blank" style="color: #58a6ff;">
                #<%= issue.number %>
              </a>
              <%= if issue.title, do: " – #{issue.title}" %>
            </td>
            <td><%= issue.streak %></td>
            <td><%= issue.cooldown_minutes %>m</td>
            <td><%= format_duration(issue.remaining_seconds) %></td>
            <td><%= format_time(issue.last_failed_at) %></td>
          </tr>
          <tr :if={@governor.cooldowns == []}>
            <td colspan="5" style="text-align: center; color: #8b949e; padding: 24px;">
              No issues in cooldown
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section>
      <h2>Recent Events</h2>

      <div style="display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap;">
        <button
          :for={{value, label} <- @event_filters}
          phx-click="filter-events"
          phx-value-source={value}
          style={filter_button_style(value == @event_source)}
        >
          <%= label %>
        </button>
      </div>

      <table>
        <thead>
          <tr>
            <th>Source</th>
            <th>Event</th>
            <th>Details</th>
            <th>When</th>
          </tr>
        </thead>
        <tbody>
          <tr :for={event <- @events}>
            <td><%= event_source(event) %></td>
            <td><%= event["event_type"] %></td>
            <td><%= format_payload(event["payload"]) %></td>
            <td><%= format_time(event["created_at"]) %></td>
          </tr>
          <tr :if={@events == []}>
            <td colspan="4" style="text-align: center; color: #8b949e; padding: 24px;">
              No recent events
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section>
      <h2>Run Timeline</h2>

      <table>
        <thead>
          <tr>
            <th>Day</th>
            <th>Merged</th>
            <th>Failed</th>
            <th>Blocked</th>
          </tr>
        </thead>
        <tbody>
          <tr :for={row <- @history}>
            <td><%= row.day %></td>
            <td><%= row.merged %></td>
            <td><%= row.failed %></td>
            <td><%= row.blocked %></td>
          </tr>
          <tr :if={@history == []}>
            <td colspan="4" style="text-align: center; color: #8b949e; padding: 24px;">
              No run history yet
            </td>
          </tr>
        </tbody>
      </table>
    </section>

    <section>
      <h2>Recent Runs</h2>

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
            <td class={phase_class(run["phase"])}><%= run["phase"] %></td>
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
    </section>

    <p class="refresh-note">Auto-refreshes on run and event updates · 30s poll fallback</p>
    """
  end

  defp refresh_dashboard(socket) do
    event_source = socket.assigns[:event_source] || "all"
    runs = Store.list_runs(limit: @run_limit)
    history_runs = Store.list_runs(limit: @history_limit)
    fleet = fleet_status()
    phase_workers = phase_worker_statuses()
    repo = detect_repo(fleet, phase_workers, runs)
    events = list_events(event_source)

    assign(socket,
      runs: runs,
      stats: compute_stats(runs),
      fleet: fleet,
      phase_workers: phase_workers,
      governor: %{
        repo: repo,
        cooldowns: governor_cooldowns(repo),
        max_starts_per_tick: Config.max_starts_per_tick(),
        max_concurrent_runs: Config.max_concurrent_runs()
      },
      events: events,
      history: run_history(history_runs),
      event_filters: @event_filters
    )
  end

  defp fleet_status do
    case safe_status(Conductor.Fleet.HealthMonitor) do
      %{sprites: sprites} = status when is_map(sprites) ->
        %{
          repo: status[:repo],
          interval_ms: status[:interval_ms],
          last_check_at: status[:last_check_at],
          sprites:
            sprites
            |> Map.values()
            |> Enum.map(fn sprite ->
              %{
                name: sprite[:name] || sprite["name"],
                role: sprite[:role] || sprite["role"],
                status: sprite[:status] || sprite["status"],
                last_probe_at: sprite[:last_probe_at] || sprite["last_probe_at"],
                consecutive_failures:
                  sprite[:consecutive_failures] || sprite["consecutive_failures"] || 0
              }
            end)
            |> Enum.sort_by(&{role_sort_key(&1.role), &1.name})
        }

      _ ->
        %{repo: nil, interval_ms: nil, last_check_at: nil, sprites: []}
    end
  end

  defp phase_worker_statuses do
    [
      worker_status(Conductor.Fixer, "Thorn", :fixer_sprite, "–"),
      worker_status(Conductor.Polisher, "Fern", :polisher_sprite, "–")
    ]
  end

  defp worker_status(module, label, sprite_key, fallback_sprite) do
    status = safe_status(module) || %{}

    %{
      label: label,
      sprite: Map.get(status, sprite_key, fallback_sprite),
      health: Map.get(status, :health, :stopped),
      failure_count: Map.get(status, :failure_count, 0),
      in_flight: Map.get(status, :in_flight, %{}),
      poll_ms: Map.get(status, :poll_ms),
      last_dispatch_at: Map.get(status, :last_dispatch_at),
      last_completion_at: Map.get(status, :last_completion_at),
      repo: Map.get(status, :repo)
    }
  end

  defp detect_repo(fleet, phase_workers, runs) do
    fleet.repo ||
      Enum.find_value(phase_workers, & &1.repo) ||
      runs |> List.first() |> then(&(&1 && &1["repo"]))
  end

  defp governor_cooldowns(nil), do: []

  defp governor_cooldowns(repo) do
    case issue_source_mod().list_issues(repo, limit: 100) do
      {:ok, issues} ->
        issues
        |> Enum.map(&cooldown_entry(repo, &1))
        |> Enum.reject(&is_nil/1)
        |> Enum.sort_by(&{&1.remaining_seconds, &1.number})

      _ ->
        []
    end
  end

  defp cooldown_entry(repo, issue) do
    issue_number = Map.get(issue, :number) || issue["number"]
    {streak, last_failed_at} = Store.issue_failure_streak(repo, issue_number)

    with true <- streak > 0,
         {:ok, cooldown_minutes, remaining_seconds} <- cooldown_window(streak, last_failed_at),
         true <- remaining_seconds > 0 do
      %{
        number: issue_number,
        title: Map.get(issue, :title) || issue["title"],
        url:
          Map.get(issue, :url) || issue["url"] ||
            "https://github.com/#{repo}/issues/#{issue_number}",
        streak: streak,
        cooldown_minutes: cooldown_minutes,
        remaining_seconds: remaining_seconds,
        last_failed_at: last_failed_at
      }
    else
      _ -> nil
    end
  end

  defp cooldown_window(streak, last_failed_at) do
    cooldown_minutes =
      min(trunc(:math.pow(2, min(streak, 20))), Config.issue_cooldown_cap_minutes())

    case DateTime.from_iso8601(last_failed_at || "") do
      {:ok, failed_at, _} ->
        expires_at = DateTime.add(failed_at, cooldown_minutes * 60, :second)
        {:ok, cooldown_minutes, max(DateTime.diff(expires_at, DateTime.utc_now(), :second), 0)}

      _ ->
        :error
    end
  end

  defp list_events(event_source) do
    Store.list_all_events(limit: @event_limit)
    |> Enum.filter(fn event -> event_matches_filter?(event, event_source) end)
  end

  defp event_matches_filter?(_event, "all"), do: true
  defp event_matches_filter?(event, filter), do: event_source(event) == filter

  defp event_source(%{"run_id" => run_id}) when run_id in ["fleet", "fixer", "polisher"],
    do: run_id

  defp event_source(_event), do: "runs"

  defp run_history(runs) do
    runs
    |> Enum.group_by(&history_day/1)
    |> Enum.map(fn {day, rows} ->
      %{
        day: day,
        merged: Enum.count(rows, &merged?/1),
        failed: Enum.count(rows, &failed?/1),
        blocked: Enum.count(rows, &blocked?/1)
      }
    end)
    |> Enum.sort_by(& &1.day, :desc)
    |> Enum.take(7)
  end

  defp history_day(run) do
    run
    |> completed_or_updated_at()
    |> case do
      nil ->
        "–"

      iso_str ->
        case DateTime.from_iso8601(iso_str) do
          {:ok, dt, _} -> Calendar.strftime(dt, "%Y-%m-%d")
          _ -> "–"
        end
    end
  end

  defp completed_or_updated_at(run),
    do: run["completed_at"] || run["updated_at"] || run["picked_at"]

  defp merged?(run), do: run["phase"] == "merged" or run["status"] == "merged"
  defp failed?(run), do: run["phase"] == "failed" or run["status"] == "failed"
  defp blocked?(run), do: run["status"] == "blocked" or run["phase"] == "blocked"

  defp compute_stats(runs) do
    now = DateTime.utc_now()

    %{
      active: Enum.count(runs, &active?/1),
      merged_24h: Enum.count(runs, &merged_in_24h?(&1, now)),
      blocked: Enum.count(runs, &blocked?/1),
      total_turns: Enum.sum(Enum.map(runs, fn r -> r["turn_count"] || 0 end))
    }
  end

  defp active?(run), do: run["completed_at"] == nil and run["status"] not in ["blocked", "failed"]

  defp merged_in_24h?(run, now) do
    merged?(run) and run["completed_at"] != nil and within_hours?(run["completed_at"], now, 24)
  end

  defp within_hours?(iso_str, now, hours) do
    case DateTime.from_iso8601(iso_str) do
      {:ok, dt, _} -> DateTime.diff(now, dt, :hour) <= hours
      _ -> false
    end
  end

  defp safe_status(module) do
    if Process.whereis(module) && function_exported?(module, :status, 0) do
      try do
        module.status()
      rescue
        _ -> nil
      catch
        :exit, _ -> nil
      end
    end
  end

  defp issue_source_mod do
    Application.get_env(:conductor, :dashboard_issue_source_module, Conductor.GitHub)
  end

  defp role_sort_key(:builder), do: 0
  defp role_sort_key(:fixer), do: 1
  defp role_sort_key(:polisher), do: 2
  defp role_sort_key(_), do: 9

  defp format_in_flight(in_flight) when map_size(in_flight) == 0, do: "idle"

  defp format_in_flight(in_flight),
    do: in_flight |> Map.keys() |> Enum.map_join(", ", &("#" <> to_string(&1)))

  defp format_payload(payload) when payload in [%{}, nil], do: "–"

  defp format_payload(payload) when is_map(payload) do
    payload
    |> Enum.sort_by(fn {key, _value} -> to_string(key) end)
    |> Enum.map_join(", ", fn {key, value} -> "#{key}=#{format_value(value)}" end)
  end

  defp format_payload(payload), do: to_string(payload)

  defp format_value(value) when is_binary(value), do: value
  defp format_value(value), do: inspect(value)

  defp short_id(nil), do: "–"
  defp short_id(id), do: String.slice(id, 0, 20)

  defp format_time(nil), do: "–"

  defp format_time(iso_str) do
    case DateTime.from_iso8601(iso_str) do
      {:ok, dt, _} -> Calendar.strftime(dt, "%m-%d %H:%M")
      _ -> iso_str
    end
  end

  defp format_interval(nil), do: "–"
  defp format_interval(ms), do: "#{div(ms, 1_000)}s"

  defp format_duration(seconds) when is_integer(seconds) and seconds >= 3600 do
    hours = div(seconds, 3600)
    minutes = div(rem(seconds, 3600), 60)
    "#{hours}h #{minutes}m"
  end

  defp format_duration(seconds) when is_integer(seconds) and seconds >= 60 do
    minutes = div(seconds, 60)
    "#{minutes}m"
  end

  defp format_duration(seconds) when is_integer(seconds), do: "#{seconds}s"

  defp phase_class(nil), do: "phase-pending"
  defp phase_class(phase), do: "phase-#{phase}"

  defp filter_button_style(true),
    do:
      "background: #58a6ff; color: #0d1117; border: 1px solid #58a6ff; border-radius: 6px; padding: 6px 10px;"

  defp filter_button_style(false),
    do:
      "background: #161b22; color: #c9d1d9; border: 1px solid #30363d; border-radius: 6px; padding: 6px 10px;"
end
