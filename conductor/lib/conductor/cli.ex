defmodule Conductor.CLI do
  @moduledoc """
  CLI entry point. Provisions sprites, launches agent loops, monitors health.
  No judgment — just infrastructure plumbing.
  """

  @commands ~w(start fleet status check-env dashboard logs show-events)

  def main(args) do
    case args do
      ["start" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_start(rest)

      ["fleet" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_fleet(rest)

      ["status" | _] ->
        Application.ensure_all_started(:conductor)
        cmd_status()

      ["check-env" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_check_env(rest)

      ["dashboard" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_dashboard(rest)

      ["logs" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_logs(rest)

      ["show-events" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_show_events(rest)

      [cmd | _] ->
        IO.puts("unknown command: #{cmd}\navailable: #{Enum.join(@commands, ", ")}")

      [] ->
        IO.puts(
          "usage: bitterblossom <command> [options]\navailable: #{Enum.join(@commands, ", ")}"
        )
    end
  end

  defp cmd_start(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [
          fleet: :string,
          timeout: :integer,
          no_timeout: :boolean
        ]
      )

    fleet_path = Keyword.get(opts, :fleet, fleet_default_path())

    timeout_minutes =
      cond do
        opts[:no_timeout] -> :infinity
        opts[:timeout] -> opts[:timeout]
        true -> Conductor.Config.session_timeout_minutes()
      end

    IO.puts("bitterblossom starting — fleet: #{fleet_path}")

    with {:ok, config} <- Conductor.Fleet.Loader.load(fleet_path) do
      run_check_env(require_codex_auth: requires_codex_auth?(config.sprites))

      case Conductor.Application.launch_fleet(fleet_path) do
        :ok ->
          case Conductor.Application.start_dashboard() do
            :ok -> :ok
            {:error, reason} -> raise "dashboard start failed: #{inspect(reason)}"
          end

          case timeout_minutes do
            :infinity ->
              IO.puts("bitterblossom running (no timeout). Press Ctrl+C to stop.")
              Process.sleep(:infinity)

            minutes ->
              IO.puts("bitterblossom running (#{minutes}m timeout). Press Ctrl+C to stop.")
              Process.sleep(minutes * 60_000)
              IO.puts("[session] timeout after #{minutes} minutes — shutting down gracefully")
              System.stop(0)
          end

        {:error, reason} ->
          IO.puts("launch failed: #{inspect(reason)}")
          System.halt(1)
      end
    else
      {:error, reason} ->
        IO.puts("fleet failed: #{reason}")
        System.halt(1)
    end
  end

  defp cmd_fleet(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [fleet: :string, reconcile: :boolean, help: :boolean]
      )

    if opts[:help] do
      IO.puts("usage: bitterblossom fleet [--fleet path] [--reconcile]")
    else
      fleet_path = Keyword.get(opts, :fleet, fleet_default_path())

      case Conductor.Fleet.Loader.load(fleet_path) do
        {:ok, config} ->
          if opts[:reconcile] do
            run_check_env(require_codex_auth: requires_codex_auth?(config.sprites))

            reconciler =
              Application.get_env(:conductor, :fleet_reconciler, Conductor.Fleet.Reconciler)

            {:ok, _results} = reconciler.reconcile_all(config.sprites)
          end

          for sprite <- config.sprites do
            name = sprite.name
            role = sprite.role
            health = probe_health(sprite)
            IO.puts("  #{name} (#{role}) — #{health}")
          end

        {:error, reason} ->
          IO.puts("fleet failed: #{reason}")
          System.halt(1)
      end
    end
  end

  defp cmd_status do
    fleet_sprites = Application.get_env(:conductor, :fleet_sprites, [])

    if fleet_sprites != [] do
      IO.puts("=== Fleet ===")

      for s <- fleet_sprites do
        case Conductor.Sprite.status(s.name, harness: s.harness) do
          {:ok, status} ->
            health = if status.healthy, do: "healthy", else: "needs setup"
            IO.puts("  #{s.name} (#{s.role}) — #{health}")

          {:error, _} ->
            IO.puts("  #{s.name} (#{s.role}) — unreachable")
        end
      end
    else
      IO.puts("(no fleet loaded — run 'bitterblossom start' first)")
    end
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

  defp cmd_logs(args) do
    {opts, positional, _} =
      OptionParser.parse(args,
        aliases: [f: :follow, n: :lines],
        strict: [follow: :boolean, lines: :integer, help: :boolean]
      )

    if opts[:help] or positional == [] do
      IO.puts("usage: bitterblossom logs <sprite> [--follow] [--lines N]")
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

  defp cmd_show_events(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [limit: :integer])
    limit = Keyword.get(opts, :limit, 50)

    events = Conductor.Store.list_all_events(limit: limit)
    IO.puts(Jason.encode!(%{event_count: length(events), events: events}))
  end

  defp cmd_check_env(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [fleet: :string])

    opts =
      case Keyword.get(opts, :fleet) do
        nil ->
          opts

        fleet_path ->
          case Conductor.Fleet.Loader.load(fleet_path) do
            {:ok, config} ->
              Keyword.put(opts, :require_codex_auth, requires_codex_auth?(config.sprites))

            {:error, reason} ->
              IO.puts("fleet failed: #{reason}")
              System.halt(1)
          end
      end

    run_check_env(opts)
  end

  defp fleet_default_path do
    cond do
      File.exists?("fleet.toml") -> "fleet.toml"
      File.exists?("../fleet.toml") -> "../fleet.toml"
      true -> "fleet.toml"
    end
  end

  defp run_check_env(opts) do
    Conductor.Config.check_env!(opts)
  rescue
    e ->
      IO.puts("environment check failed: #{Exception.message(e)}")
      System.halt(1)
  end

  defp requires_codex_auth?(sprites) do
    Enum.any?(sprites, fn sprite ->
      harness = Map.get(sprite, :harness) || Map.get(sprite, "harness") || "codex"
      harness == "codex"
    end)
  end

  defp probe_health(sprite) do
    worker_mod = Application.get_env(:conductor, :worker_module, Conductor.Sprite)
    name = sprite.name
    harness = Map.get(sprite, :harness)

    result =
      cond do
        function_exported?(worker_mod, :status, 2) ->
          worker_mod.status(name, harness: harness)

        function_exported?(worker_mod, :status, 1) ->
          worker_mod.status(name)

        function_exported?(worker_mod, :probe, 2) ->
          worker_mod.probe(name, [])

        true ->
          worker_mod.probe(name)
      end

    case result do
      {:ok, %{healthy: true}} -> "healthy"
      {:ok, %{reachable: true, healthy: false}} -> "needs setup"
      {:ok, %{reachable: true}} -> "healthy"
      {:ok, _} -> "needs setup"
      {:error, _} -> "unreachable"
    end
  rescue
    _ -> "unreachable"
  end
end
