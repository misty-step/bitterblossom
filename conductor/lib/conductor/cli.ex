defmodule Conductor.CLI do
  @moduledoc "Escript entry point. Parses args and delegates to Conductor."

  @commands ~w(start pause resume shape fleet show-runs show-events show-incidents show-waivers check-env dashboard status)

  def main(args) do
    Application.ensure_all_started(:conductor)

    case args do
      ["start" | rest] ->
        cmd_start(rest)

      # Legacy aliases — clear error messages
      ["run-once" | _] ->
        IO.puts("run-once has been removed. Use: mix conductor start")
        IO.puts("The conductor now runs as an always-on service.")
        System.halt(1)

      ["loop" | _] ->
        IO.puts("loop has been removed. Use: mix conductor start")
        IO.puts("The conductor now runs as an always-on service.")
        System.halt(1)

      ["shape" | rest] ->
        cmd_shape(rest)

      ["pause"] ->
        cmd_pause()

      ["resume"] ->
        cmd_resume()

      ["fleet" | rest] ->
        cmd_fleet(rest)

      ["show-runs" | rest] ->
        cmd_show_runs(rest)

      ["show-events" | rest] ->
        cmd_show_events(rest)

      ["show-incidents" | rest] ->
        cmd_show_incidents(rest)

      ["show-waivers" | rest] ->
        cmd_show_waivers(rest)

      ["dashboard" | rest] ->
        cmd_dashboard(rest)

      ["status" | _] ->
        cmd_status()

      ["check-env" | _] ->
        cmd_check_env()

      [cmd | _] ->
        IO.puts("unknown command: #{cmd}\navailable: #{Enum.join(@commands, ", ")}")

      [] ->
        IO.puts("usage: conductor <command> [options]\navailable: #{Enum.join(@commands, ", ")}")
    end
  end

  defp cmd_start(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [
          fleet: :string
        ]
      )

    fleet_path = Keyword.get(opts, :fleet, fleet_default_path())

    IO.puts("bitterblossom starting — fleet: #{fleet_path}")

    # Validate environment before doing anything
    cmd_check_env()

    case Conductor.Application.boot_fleet(fleet_path) do
      :ok ->
        IO.puts("bitterblossom running. Press Ctrl+C to stop.")
        # Block forever — everything runs in the supervision tree
        Process.sleep(:infinity)

      {:error, reason} ->
        IO.puts("boot failed: #{inspect(reason)}")
        System.halt(1)
    end
  end

  defp fleet_default_path do
    # Look for fleet.toml relative to the conductor dir, then repo root
    cond do
      File.exists?("fleet.toml") -> "fleet.toml"
      File.exists?("../fleet.toml") -> "../fleet.toml"
      true -> "fleet.toml"
    end
  end

  defp cmd_shape(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [
          repo: :string,
          issue: :integer
        ]
      )

    repo = Keyword.fetch!(opts, :repo)
    issue = Keyword.fetch!(opts, :issue)

    IO.puts("conductor shape: issue ##{issue} on #{repo}")

    case Conductor.Shaper.shape(repo, issue) do
      {:ok, :already_shaped} ->
        IO.puts("issue ##{issue} is already shaped — no changes made")

      {:ok, :shaped} ->
        IO.puts("issue ##{issue} shaped successfully")

      {:error, reason} ->
        IO.puts("shape failed: #{inspect(reason)}")
        System.halt(1)
    end
  end

  defp cmd_pause do
    :ok = Conductor.Orchestrator.pause()
    IO.puts("conductor dispatch paused")
  end

  defp cmd_resume do
    :ok = Conductor.Orchestrator.resume()
    IO.puts("conductor dispatch resumed")
  end

  defp cmd_fleet(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [
          fleet: :string
        ]
      )

    fleet_path = Keyword.get(opts, :fleet, fleet_default_path())

    case Conductor.Fleet.Loader.load(fleet_path) do
      {:ok, config} ->
        assignments = active_builder_assignments(config.defaults.repo)

        config.sprites
        |> Enum.each(fn sprite ->
          tags = format_tags(sprite.capability_tags)
          health = probe_status(sprite.name)
          assignment = Map.get(assignments, sprite.name, "idle")
          IO.puts("#{sprite.name} role=#{sprite.role} #{health} assignment=#{assignment} #{tags}")
        end)

      {:error, reason} ->
        IO.puts("fleet failed: #{reason}")
        System.halt(1)
    end
  end

  defp cmd_show_runs(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [limit: :integer])
    limit = Keyword.get(opts, :limit, 20)

    runs = Conductor.Store.list_runs(limit: limit)

    Enum.each(runs, fn run ->
      IO.puts(Jason.encode!(run))
    end)
  end

  defp cmd_show_events(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [run_id: :string])
    run_id = Keyword.fetch!(opts, :run_id)

    events = Conductor.Store.list_events(run_id)

    IO.puts(
      Jason.encode!(%{
        run_id: run_id,
        event_count: length(events),
        events: events
      })
    )
  end

  defp cmd_show_incidents(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [run_id: :string])
    cmd_show_run_records(Keyword.fetch!(opts, :run_id), :incidents)
  end

  defp cmd_show_waivers(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [run_id: :string])
    cmd_show_run_records(Keyword.fetch!(opts, :run_id), :waivers)
  end

  defp cmd_show_run_records(run_id, kind) do
    {list_fn, count_key} =
      case kind do
        :incidents -> {&Conductor.Store.list_incidents/1, :incident_count}
        :waivers -> {&Conductor.Store.list_waivers/1, :waiver_count}
      end

    records = list_fn.(run_id)

    IO.puts(
      Jason.encode!(
        %{run_id: run_id}
        |> Map.put(kind, records)
        |> Map.put(count_key, length(records))
      )
    )
  end

  defp cmd_dashboard(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [port: :integer])
    port = Keyword.get(opts, :port, 4000)

    Application.put_env(:conductor, Conductor.Web.Endpoint,
      adapter: Bandit.PhoenixAdapter,
      http: [ip: {127, 0, 0, 1}, port: port],
      secret_key_base:
        System.get_env("DASHBOARD_SECRET_KEY_BASE") ||
          "bitterblossom-dashboard-dev-key-must-be-at-least-64-chars-long-x",
      live_view: [signing_salt: "bb_lv_salt"],
      server: true
    )

    {:ok, _} = Supervisor.start_child(Conductor.Supervisor, Conductor.Web.Endpoint)
    IO.puts("dashboard running at http://localhost:#{port}")
    Process.sleep(:infinity)
  end

  defp cmd_status do
    IO.puts("=== Fleet ===")

    fleet_sprites = Application.get_env(:conductor, :fleet_sprites, [])

    if fleet_sprites != [] do
      for s <- fleet_sprites do
        case Conductor.Sprite.status(s.name, harness: s.harness) do
          {:ok, status} ->
            auth = if status.gh_authenticated, do: "gh auth ok", else: "gh auth missing"
            git = if status.git_credential_helper, do: "git helper ok", else: "git helper missing"
            health = if status.healthy, do: "healthy", else: "needs setup"
            IO.puts("  #{s.name} (#{s.role}, #{s.harness}) — #{health}, #{auth}, #{git}")

          {:error, _reason} ->
            IO.puts("  #{s.name} (#{s.role}, #{s.harness}) — unreachable")
        end
      end
    else
      IO.puts("  (no fleet loaded — run 'conductor start' first)")
    end

    IO.puts("\n=== Phase Workers ===")

    if Process.whereis(Conductor.Fixer) do
      fixer = Conductor.Fixer.status()
      IO.puts("  fixer: #{fixer.fixer_sprite} — #{map_size(fixer.in_flight)} in-flight")
    else
      IO.puts("  fixer: not running")
    end

    if Process.whereis(Conductor.Polisher) do
      polisher = Conductor.Polisher.status()

      IO.puts(
        "  polisher: #{polisher.polisher_sprite} — #{map_size(polisher.in_flight)} in-flight"
      )
    else
      IO.puts("  polisher: not running")
    end

    IO.puts("\n=== Recent Runs ===")

    for run <- Conductor.Store.list_runs(limit: 5) do
      IO.puts("  #{run["run_id"]} — #{run["phase"]} (#{run["status"]})")
    end
  end

  defp cmd_check_env do
    Conductor.Config.check_env!()
  rescue
    e ->
      IO.puts("environment check failed: #{Exception.message(e)}")
      System.halt(1)
  end

  defp active_builder_assignments(repo) do
    repo
    |> Conductor.Store.list_active_runs()
    |> Map.new(fn run -> {run["builder_sprite"], "issue ##{run["issue_number"]}"} end)
  end

  defp probe_status(sprite) do
    worker_mod = Application.get_env(:conductor, :worker_module, Conductor.Sprite)

    result =
      try do
        Conductor.Orchestrator.probe_worker_module(worker_mod, sprite, [])
      rescue
        System.EnvError ->
          {:error, :missing_env}
      end

    case result do
      {:ok, _} -> "healthy"
      {:error, _} -> "unreachable"
    end
  end

  defp format_tags([]), do: "tags=-"
  defp format_tags(tags), do: "tags=#{Enum.join(tags, ",")}"
end
