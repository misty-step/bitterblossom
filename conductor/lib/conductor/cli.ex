defmodule Conductor.CLI do
  @moduledoc """
  CLI entry point. Provisions sprites, launches agent loops, monitors health.
  No judgment — just infrastructure plumbing.
  """

  @commands ~w(start fleet status check-env dashboard logs show-events sprite)

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
        cmd_status(args |> tl())

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

      ["sprite" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_sprite(rest)

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
    {mode, args} =
      case args do
        ["status" | rest] ->
          {:status, rest}

        ["audit" | rest] ->
          {:audit, rest}

        [<<"-", _::binary>> | _] ->
          {:status, args}

        [_subcommand | _] ->
          {:invalid, args}

        _ ->
          {:status, args}
      end

    if mode == :invalid do
      IO.puts("usage: bitterblossom fleet [status|audit] [--fleet path] [--reconcile] [--json]")
      System.halt(1)
    else
      {opts, positional, invalid} =
        OptionParser.parse(args,
          strict: [fleet: :string, reconcile: :boolean, help: :boolean, json: :boolean]
        )

      if invalid != [] or Keyword.get(opts, :help, false) or positional != [] do
        IO.puts("usage: bitterblossom fleet [status|audit] [--fleet path] [--reconcile] [--json]")
        if opts[:help], do: :ok, else: System.halt(1)
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

            rows = Enum.map(config.sprites, &fleet_row(&1, fleet_probe_module()))
            summary = fleet_summary(rows)

            if opts[:json] || mode == :audit do
              IO.puts(Jason.encode!(%{summary: summary, sprites: rows}))
            else
              IO.puts(
                "fleet: #{summary.total} total, #{summary.available_capacity} available, #{summary.running} running, #{summary.paused} paused"
              )

              Enum.each(rows, fn row ->
                IO.puts(render_fleet_row(row))
              end)
            end

          {:error, reason} ->
            IO.puts("fleet failed: #{reason}")
            System.halt(1)
        end
      end
    end
  end

  defp cmd_status(args) do
    cmd_fleet(["status" | args])
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
    {opts, positional, invalid} =
      OptionParser.parse(args,
        aliases: [f: :follow, n: :lines],
        strict: [follow: :boolean, lines: :integer, help: :boolean]
      )

    if invalid != [] or Keyword.get(opts, :help, false) or positional == [] do
      IO.puts("usage: bitterblossom logs <sprite> [--follow] [--lines N]")
      if opts[:help], do: :ok, else: System.halt(1)
    else
      case sprite_module().logs(hd(positional),
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

  defp cmd_sprite(args) do
    case args do
      ["status" | rest] ->
        cmd_sprite_status(rest)

      ["start" | rest] ->
        cmd_sprite_start(rest)

      ["stop" | rest] ->
        cmd_sprite_stop(rest)

      ["pause" | rest] ->
        cmd_sprite_pause(rest)

      ["resume" | rest] ->
        cmd_sprite_resume(rest)

      ["logs" | rest] ->
        cmd_logs(rest)

      _ ->
        IO.puts("usage: bitterblossom sprite <status|start|stop|pause|resume|logs> ...")
        System.halt(1)
    end
  end

  defp cmd_sprite_status(args) do
    with {:ok, sprite, opts, _config} <- fetch_sprite_args(args, json: :boolean) do
      row = fleet_row(sprite, sprite_module())

      if opts[:json] do
        IO.puts(Jason.encode!(row))
      else
        IO.puts(render_fleet_row(row))
      end
    else
      {:error, reason} ->
        IO.puts(reason)
        System.halt(1)
    end
  end

  defp cmd_sprite_start(args) do
    with {:ok, sprite, _opts, _config} <- fetch_sprite_args(args),
         status <- probe_status(sprite, sprite_module()),
         :ok <- ensure_start_admissible(status),
         :ok <- ensure_sprite_ready_for_start(sprite, status),
         :ok <-
           workspace_module().sync_persona(
             sprite.name,
             workspace_module().repo_root(sprite.repo),
             persona_for_role(sprite.role)
           ) do
      prompt = Conductor.Launcher.loop_prompt(sprite, sprite.repo)

      case sprite_module().start_loop(sprite.name, prompt, sprite.repo,
             workspace: workspace_module().repo_root(sprite.repo),
             persona_role: persona_for_role(sprite.role),
             harness: harness_module(sprite.harness),
             harness_opts: [reasoning_effort: sprite.reasoning_effort]
           ) do
        {:ok, pid} ->
          IO.puts("started #{sprite.name} (pid #{String.trim(pid)})")

        {:error, reason, _code} ->
          IO.puts(reason)
          System.halt(1)
      end
    else
      {:error, reason} ->
        IO.puts(reason)
        System.halt(1)

      :ok ->
        :ok
    end
  end

  defp cmd_sprite_stop(args) do
    with {:ok, sprite, _opts, _config} <- fetch_sprite_args(args),
         :ok <- sprite_module().stop_loop(sprite.name) do
      IO.puts("stopped #{sprite.name}")
    else
      {:error, reason} ->
        IO.puts(reason)
        System.halt(1)
    end
  end

  defp cmd_sprite_pause(args) do
    with {:ok, sprite, opts, _config} <- fetch_sprite_args(args, wait: :boolean),
         :ok <- sprite_module().pause(sprite.name),
         :ok <- maybe_stop_after_pause(sprite.name, opts) do
      IO.puts("paused #{sprite.name}")
    else
      {:error, reason} ->
        IO.puts(reason)
        System.halt(1)
    end
  end

  defp cmd_sprite_resume(args) do
    with {:ok, sprite, _opts, _config} <- fetch_sprite_args(args),
         :ok <- sprite_module().resume(sprite.name) do
      IO.puts("resumed #{sprite.name}")
    else
      {:error, reason} ->
        IO.puts(reason)
        System.halt(1)
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
    config_module().check_env!(opts)
  rescue
    e ->
      IO.puts("environment check failed: #{Exception.message(e)}")
      System.halt(1)
  end

  defp run_check_env_for_sprite(sprite) do
    run_check_env(require_codex_auth: requires_codex_auth?([sprite]))
  end

  defp requires_codex_auth?(sprites) do
    Enum.any?(sprites, fn sprite ->
      harness = Map.get(sprite, :harness) || Map.get(sprite, "harness") || "codex"
      harness == "codex"
    end)
  end

  defp fleet_row(sprite, probe_module) do
    status = probe_status(sprite, probe_module)

    %{
      name: sprite.name,
      role: sprite.role,
      repo: Map.get(sprite, :repo),
      reachable: Map.get(status, :reachable, false),
      healthy: Map.get(status, :healthy, false),
      paused: Map.get(status, :paused, false),
      busy: Map.get(status, :busy, false),
      lifecycle_status: Map.get(status, :lifecycle_status, "unknown"),
      health: health_label(status)
    }
  end

  defp render_fleet_row(row) do
    "  #{row.name} (#{row.role}) — #{row.health}, #{row.lifecycle_status}"
  end

  defp fleet_summary(rows) do
    %{
      total: length(rows),
      reachable: Enum.count(rows, & &1.reachable),
      healthy: Enum.count(rows, & &1.healthy),
      paused: Enum.count(rows, & &1.paused),
      running: Enum.count(rows, &(&1.lifecycle_status in ["running", "draining"])),
      available_capacity:
        Enum.count(rows, fn row ->
          row.reachable and row.healthy and not row.paused and not row.busy
        end)
    }
  end

  defp health_label(%{reachable: false}), do: "unreachable"
  defp health_label(%{healthy: true}), do: "healthy"
  defp health_label(_status), do: "needs setup"

  defp probe_status(sprite, probe_module) do
    name = sprite.name
    harness = Map.get(sprite, :harness)

    result =
      cond do
        function_exported?(probe_module, :status, 2) ->
          probe_module.status(name, harness: harness)

        function_exported?(probe_module, :status, 1) ->
          probe_module.status(name)

        function_exported?(probe_module, :probe, 2) ->
          probe_module.probe(name, [])

        true ->
          probe_module.probe(name)
      end

    case result do
      {:ok, status} when is_map(status) ->
        %{
          sprite: name,
          reachable: Map.get(status, :reachable, true),
          healthy: Map.get(status, :healthy, Map.get(status, :reachable, true)),
          paused: Map.get(status, :paused, false),
          busy: Map.get(status, :busy, false),
          lifecycle_status: Map.get(status, :lifecycle_status, "idle")
        }

      {:error, reason} ->
        %{
          sprite: name,
          reachable: false,
          healthy: false,
          paused: false,
          busy: false,
          lifecycle_status: "unreachable",
          error: reason
        }
    end
  rescue
    _ ->
      %{
        sprite: sprite.name,
        reachable: false,
        healthy: false,
        paused: false,
        busy: false,
        lifecycle_status: "unreachable"
      }
  end

  defp fetch_sprite_args(args, extra_opts \\ []) do
    {opts, positional, invalid} =
      OptionParser.parse(args,
        strict: Keyword.merge([fleet: :string, help: :boolean], extra_opts)
      )

    cond do
      invalid != [] or Keyword.get(opts, :help, false) or positional == [] ->
        {:error, "usage: bitterblossom sprite <command> <sprite> [--fleet path]"}

      true ->
        fleet_path = Keyword.get(opts, :fleet, fleet_default_path())
        sprite_name = hd(positional)

        with {:ok, config} <- Conductor.Fleet.Loader.load(fleet_path),
             %{} = sprite <- Enum.find(config.sprites, &(&1.name == sprite_name)) do
          {:ok, sprite, opts, config}
        else
          nil -> {:error, "sprite #{sprite_name} is not declared in #{fleet_path}"}
          {:error, reason} -> {:error, "fleet failed: #{reason}"}
        end
    end
  end

  defp maybe_force_sync_codex_auth(sprite) do
    if (Map.get(sprite, :harness) || "codex") == "codex" do
      sprite_module().force_sync_codex_auth(sprite.name)
    else
      :ok
    end
  end

  defp maybe_stop_after_pause(sprite_name, opts) do
    if opts[:wait], do: sprite_module().stop_loop(sprite_name), else: :ok
  end

  defp ensure_start_admissible(%{paused: true}), do: {:error, "sprite is paused"}
  defp ensure_start_admissible(%{busy: true}), do: {:error, "sprite already has an active loop"}
  defp ensure_start_admissible(_status), do: :ok

  defp ensure_sprite_ready_for_start(_sprite, %{reachable: true, healthy: true}), do: :ok

  defp ensure_sprite_ready_for_start(sprite, _status) do
    with :ok <- run_check_env_for_sprite(sprite),
         :ok <-
           sprite_module().provision(sprite.name,
             repo: sprite.repo,
             persona: sprite.persona,
             harness: sprite.harness
           ),
         :ok <- maybe_force_sync_codex_auth(sprite) do
      :ok
    end
  end

  defp persona_for_role(:builder), do: :weaver
  defp persona_for_role(:fixer), do: :thorn
  defp persona_for_role(:polisher), do: :fern
  defp persona_for_role(role), do: role

  defp harness_module("claude-code"), do: Conductor.ClaudeCode
  defp harness_module(_), do: Conductor.Codex

  defp fleet_probe_module do
    Application.get_env(:conductor, :worker_module, sprite_module())
  end

  defp workspace_module do
    Application.get_env(:conductor, :workspace_module, Conductor.Workspace)
  end

  defp config_module do
    Application.get_env(:conductor, :config_module, Conductor.Config)
  end

  defp sprite_module do
    Application.get_env(:conductor, :sprite_module, Conductor.Sprite)
  end
end
