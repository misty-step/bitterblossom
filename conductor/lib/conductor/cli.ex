defmodule Conductor.CLI do
  @moduledoc "Escript entry point. Parses args and delegates to Conductor."

  @commands ~w(start pause resume shape fleet logs show-runs show-events show-incidents show-waivers check-env dashboard status)

  @doc "Dispatch the conductor CLI command selected by `args`."
  def main(args) do
    case args do
      ["run-once" | _] ->
        removed_command("run-once")

      ["loop" | _] ->
        removed_command("loop")

      ["start" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_start(rest)

      ["shape" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_shape(rest)

      ["pause"] ->
        Application.ensure_all_started(:conductor)
        cmd_pause()

      ["resume"] ->
        Application.ensure_all_started(:conductor)
        cmd_resume()

      ["fleet" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_fleet(rest)

      ["logs" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_logs(rest)

      ["show-runs" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_show_runs(rest)

      ["show-events" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_show_events(rest)

      ["show-incidents" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_show_incidents(rest)

      ["show-waivers" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_show_waivers(rest)

      ["dashboard" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_dashboard(rest)

      ["status" | _] ->
        Application.ensure_all_started(:conductor)
        cmd_status()

      ["check-env" | _] ->
        Application.ensure_all_started(:conductor)
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

    case Conductor.Application.start_dashboard() do
      :ok -> :ok
      {:error, reason} -> IO.puts("dashboard start failed: #{inspect(reason)}")
    end

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

  defp removed_command(command) do
    IO.puts("#{command} removed — use `mix conductor start`")
    System.halt(1)
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
          fleet: :string,
          reconcile: :boolean,
          help: :boolean
        ]
      )

    if opts[:help] do
      IO.puts("""
      usage: mix conductor fleet [--fleet path] [--reconcile]

      Options:
        --fleet PATH      fleet.toml path
        --reconcile       provision unhealthy sprites before printing status
      """)

      :ok
    else
      fleet_path = Keyword.get(opts, :fleet, fleet_default_path())

      case Conductor.Fleet.Loader.load(fleet_path) do
        {:ok, config} ->
          if opts[:reconcile] do
            cmd_check_env()

            reconciler =
              Application.get_env(:conductor, :fleet_reconciler, Conductor.Fleet.Reconciler)

            {:ok, _results} = reconciler.reconcile_all(config.sprites)
          end

          assignments = active_builder_assignments(config.defaults.repo)

          config.sprites
          |> Enum.each(fn sprite ->
            name = sprite_name(sprite)
            display_name = name || "(unnamed sprite)"
            role = Map.get(sprite, :role) || Map.get(sprite, "role") || "unknown"

            tags =
              format_tags(
                Map.get(sprite, :capability_tags) || Map.get(sprite, "capability_tags") || []
              )

            health = probe_status(sprite)
            assignment = if name, do: Map.get(assignments, name, "idle"), else: "idle"
            IO.puts("#{display_name} role=#{role} #{health} assignment=#{assignment} #{tags}")
          end)

        {:error, reason} ->
          IO.puts("fleet failed: #{reason}")
          System.halt(1)
      end
    end
  end

  defp cmd_logs(args) do
    {opts, positional, _} =
      OptionParser.parse(args,
        aliases: [f: :follow, n: :lines],
        strict: [
          follow: :boolean,
          lines: :integer,
          help: :boolean
        ]
      )

    if Keyword.get(opts, :help, false) or positional == [] do
      IO.puts("""
      usage: mix conductor logs <sprite> [--follow] [--lines N]

      Options:
        --follow, -f      follow log output
        --lines, -n N     last N lines (0 = all, default)
      """)

      if opts[:help], do: :ok, else: System.halt(1)
    else
      case Conductor.Sprite.logs(hd(positional),
             follow: Keyword.get(opts, :follow, false),
             lines: Keyword.get(opts, :lines, 0)
           ) do
        :ok ->
          :ok

        {:error, reason} ->
          IO.puts(reason)
          System.halt(1)
      end
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
    {opts, _, _} = OptionParser.parse(args, strict: [run_id: :string, limit: :integer])

    case Keyword.get(opts, :run_id) do
      nil ->
        limit = Keyword.get(opts, :limit, 50)
        events = Conductor.Store.list_all_events(limit: limit)
        %{event_count: length(events), events: events}

      run_id ->
        events = Conductor.Store.list_events(run_id)
        %{run_id: run_id, event_count: length(events), events: events}
    end
    |> Jason.encode!()
    |> IO.puts()
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

    Application.put_env(:conductor, :start_dashboard, true)
    :ok = Conductor.Application.start_dashboard(port: port)
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
      IO.puts("  thorn: #{fixer.fixer_sprite} — #{map_size(fixer.in_flight)} in-flight")
    else
      IO.puts("  thorn: not running")
    end

    if Process.whereis(Conductor.Polisher) do
      polisher = Conductor.Polisher.status()

      IO.puts("  fern: #{polisher.polisher_sprite} — #{map_size(polisher.in_flight)} in-flight")
    else
      IO.puts("  fern: not running")
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
    harness = Map.get(sprite, :harness) || Map.get(sprite, "harness")
    name = sprite_name(sprite)

    result =
      if is_binary(name) and name != "" do
        try do
          cond do
            function_exported?(worker_mod, :status, 2) ->
              worker_mod.status(name, harness: harness)

            function_exported?(worker_mod, :status, 1) ->
              worker_mod.status(name)

            true ->
              Conductor.Orchestrator.probe_worker_module(worker_mod, name, [])
          end
        rescue
          System.EnvError ->
            {:error, :missing_env}

          error in RuntimeError ->
            if String.starts_with?(Exception.message(error), "no sprite org:") do
              {:error, :missing_env}
            else
              reraise error, __STACKTRACE__
            end
        end
      else
        {:error, :missing_name}
      end

    case result do
      {:ok, %{healthy: true}} ->
        "healthy"

      {:ok, status} when is_map(status) ->
        if probe_only_status?(status) do
          "healthy"
        else
          missing =
            []
            |> maybe_missing(status, :harness_ready, "harness")
            |> maybe_missing(status, :gh_authenticated, "gh auth")
            |> maybe_missing(status, :git_credential_helper, "git helper")

          if missing == [] do
            "needs setup"
          else
            "needs setup (" <> Enum.join(missing, ", ") <> " missing)"
          end
        end

      {:ok, _} ->
        "healthy"

      {:error, :missing_name} ->
        "invalid config (name missing)"

      {:error, :missing_env} ->
        "missing env"

      {:error, _} ->
        "unreachable"
    end
  end

  defp sprite_name(sprite) when is_binary(sprite), do: sprite

  defp sprite_name(sprite) when is_map(sprite),
    do: Map.get(sprite, :name) || Map.get(sprite, "name")

  defp sprite_name(_), do: nil

  defp maybe_missing(acc, status, key, label) do
    case Map.fetch(status, key) do
      {:ok, false} -> acc ++ [label]
      _ -> acc
    end
  end

  defp probe_only_status?(status) do
    reachable = Map.get(status, :reachable, Map.get(status, "reachable"))

    reachable == true and
      not Map.has_key?(status, :healthy) and
      not Map.has_key?(status, "healthy") and
      not Map.has_key?(status, :gh_authenticated) and
      not Map.has_key?(status, "gh_authenticated") and
      not Map.has_key?(status, :git_credential_helper) and
      not Map.has_key?(status, "git_credential_helper") and
      not Map.has_key?(status, :harness_ready) and
      not Map.has_key?(status, "harness_ready")
  end

  defp format_tags([]), do: "tags=-"
  defp format_tags(tags), do: "tags=#{Enum.join(tags, ",")}"
end
