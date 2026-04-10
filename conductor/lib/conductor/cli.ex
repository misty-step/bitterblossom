defmodule Conductor.CLI do
  @moduledoc """
  CLI entry point. Provisions sprites, launches agent loops, monitors health.
  No judgment — just infrastructure plumbing.
  """

  @commands ~w(start fleet status check-env dashboard logs show-events sprite canary)

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

      ["canary" | rest] ->
        Application.ensure_all_started(:conductor)
        cmd_canary(rest)

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
      run_check_env(env_check_opts(config.sprites))

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
              run_check_env(env_check_opts(config.sprites))

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

  defp cmd_canary(args) do
    case args do
      ["service" | rest] ->
        cmd_canary_service(rest)

      ["incidents" | rest] ->
        cmd_canary_incidents(rest)

      ["report" | rest] ->
        cmd_canary_report(rest)

      ["timeline" | rest] ->
        cmd_canary_timeline(rest)

      ["annotations" | rest] ->
        cmd_canary_annotations(rest)

      ["annotate" | rest] ->
        cmd_canary_annotate(rest)

      _ ->
        IO.puts(
          "usage: bitterblossom canary <service|incidents|report|timeline|annotations|annotate> ..."
        )

        System.halt(1)
    end
  end

  defp cmd_canary_service(args) do
    {opts, positional, invalid} =
      OptionParser.parse(args, strict: [catalog: :string, json: :boolean, help: :boolean])

    if invalid != [] or Keyword.get(opts, :help, false) or length(positional) != 1 do
      IO.puts("usage: bitterblossom canary service <service> [--catalog path] [--json]")
      if opts[:help], do: :ok, else: System.halt(1)
    else
      service_name = hd(positional)
      path = Keyword.get(opts, :catalog, Conductor.Config.canary_services_path())

      with {:ok, services} <- Conductor.Canary.ServiceCatalog.load(path),
           {:ok, service} <- Conductor.Canary.ServiceCatalog.fetch(services, service_name) do
        if opts[:json] do
          IO.puts(Jason.encode!(service))
        else
          IO.puts("service: #{service.name}")

          if service.aliases != [] do
            IO.puts("aliases: #{Enum.join(service.aliases, ", ")}")
          end

          IO.puts("repo: #{service.repo}")
          IO.puts("clone_url: #{service.clone_url}")
          IO.puts("default_branch: #{service.default_branch}")
          IO.puts("test_cmd: #{Enum.join(service.test_cmd, " ")}")
          IO.puts("auto_merge: #{service.auto_merge}")
          IO.puts("auto_deploy: #{service.auto_deploy}")

          if service.deploy_cmd do
            IO.puts("deploy_cmd: #{Enum.join(service.deploy_cmd, " ")}")
          end

          if service.rollback_cmd do
            IO.puts("rollback_cmd: #{Enum.join(service.rollback_cmd, " ")}")
          end
        end
      else
        {:error, :not_found} ->
          IO.puts("unknown Canary service: #{service_name}")
          System.halt(1)

        {:error, reason} ->
          IO.puts("canary catalog failed: #{reason}")
          System.halt(1)
      end
    end
  end

  defp cmd_canary_incidents(args) do
    {opts, positional, invalid} =
      OptionParser.parse(args,
        strict: [
          with_annotation: :string,
          without_annotation: :string,
          json: :boolean,
          help: :boolean
        ]
      )

    if invalid != [] or Keyword.get(opts, :help, false) or positional != [] do
      IO.puts(
        "usage: bitterblossom canary incidents [--with-annotation action] [--without-annotation action] [--json]"
      )

      if opts[:help], do: :ok, else: System.halt(1)
    else
      with {:ok, response} <-
             canary_client_module().incidents(
               with_annotation: opts[:with_annotation],
               without_annotation: opts[:without_annotation]
             ) do
        if opts[:json] do
          IO.puts(Jason.encode!(response))
        else
          render_incidents(response)
        end
      else
        {:error, reason} ->
          IO.puts(reason)
          System.halt(1)
      end
    end
  end

  defp cmd_canary_report(args) do
    {opts, positional, invalid} =
      OptionParser.parse(args,
        strict: [
          window: :string,
          q: :string,
          limit: :integer,
          cursor: :string,
          json: :boolean,
          help: :boolean
        ]
      )

    if invalid != [] or Keyword.get(opts, :help, false) or positional != [] do
      IO.puts(
        "usage: bitterblossom canary report [--window window] [--q query] [--limit n] [--cursor token] [--json]"
      )

      if opts[:help], do: :ok, else: System.halt(1)
    else
      with {:ok, response} <-
             canary_client_module().report(
               window: opts[:window],
               q: opts[:q],
               limit: opts[:limit],
               cursor: opts[:cursor]
             ) do
        if opts[:json] do
          IO.puts(Jason.encode!(response))
        else
          render_report(response)
        end
      else
        {:error, reason} ->
          IO.puts(reason)
          System.halt(1)
      end
    end
  end

  defp cmd_canary_timeline(args) do
    {opts, positional, invalid} =
      OptionParser.parse(args,
        strict: [
          service: :string,
          window: :string,
          limit: :integer,
          after: :string,
          cursor: :string,
          event_type: :string,
          json: :boolean,
          help: :boolean
        ]
      )

    if invalid != [] or Keyword.get(opts, :help, false) or positional != [] do
      IO.puts(
        "usage: bitterblossom canary timeline [--service service] [--window window] [--limit n] [--after cursor] [--cursor cursor] [--event-type type] [--json]"
      )

      if opts[:help], do: :ok, else: System.halt(1)
    else
      with {:ok, response} <-
             canary_client_module().timeline(
               service: opts[:service],
               window: opts[:window],
               limit: opts[:limit],
               after: opts[:after],
               cursor: opts[:cursor],
               event_type: opts[:event_type]
             ) do
        if opts[:json] do
          IO.puts(Jason.encode!(response))
        else
          render_timeline(response)
        end
      else
        {:error, reason} ->
          IO.puts(reason)
          System.halt(1)
      end
    end
  end

  defp cmd_canary_annotations(args) do
    {opts, positional, invalid} =
      OptionParser.parse(args, strict: [json: :boolean, help: :boolean])

    if invalid != [] or Keyword.get(opts, :help, false) or
         match_canary_annotations_args?(positional) == false do
      IO.puts("usage: bitterblossom canary annotations incident <incident-id> [--json]")
      if opts[:help], do: :ok, else: System.halt(1)
    else
      ["incident", incident_id] = positional

      with {:ok, response} <- canary_client_module().incident_annotations(incident_id) do
        if opts[:json] do
          IO.puts(Jason.encode!(response))
        else
          render_annotations(response)
        end
      else
        {:error, reason} ->
          IO.puts(reason)
          System.halt(1)
      end
    end
  end

  defp cmd_canary_annotate(args) do
    {opts, positional, invalid} =
      OptionParser.parse(args,
        strict: [
          agent: :string,
          action: :string,
          metadata: :string,
          json: :boolean,
          help: :boolean
        ]
      )

    if invalid != [] or Keyword.get(opts, :help, false) or
         match_canary_annotations_args?(positional) == false do
      IO.puts(
        "usage: bitterblossom canary annotate incident <incident-id> --agent name --action action [--metadata json] [--json]"
      )

      if opts[:help], do: :ok, else: System.halt(1)
    else
      ["incident", incident_id] = positional

      with {:ok, attrs} <- build_annotation_attrs(opts),
           {:ok, response} <- canary_client_module().annotate_incident(incident_id, attrs) do
        if opts[:json] do
          IO.puts(Jason.encode!(response))
        else
          render_annotation(response)
        end
      else
        {:error, reason} ->
          IO.puts(reason)
          System.halt(1)
      end
    end
  end

  defp cmd_sprite_status(args) do
    with {:ok, sprite, opts, _config} <- fetch_sprite_args(args, json: :boolean) do
      row = declared_sprite_row(sprite, sprite_module())

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
         status <- declared_sprite_status(sprite, sprite_module()),
         :ok <- ensure_start_admissible(status),
         :ok <- ensure_start_preflight(sprite),
         :ok <- ensure_sprite_ready_for_start(sprite, status),
         :ok <-
           workspace_module().sync_persona(
             sprite.name,
             workspace_module().repo_root(sprite.repo),
             workspace_module().persona_for_role(sprite.role)
           ) do
      prompt = Conductor.Launcher.loop_prompt(sprite, sprite.repo)

      case sprite_module().start_loop(sprite.name, prompt, sprite.repo,
             workspace: workspace_module().repo_root(sprite.repo),
             persona_role: workspace_module().persona_for_role(sprite.role),
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
              env_check_opts(config.sprites, opts)

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
    run_check_env(env_check_opts([sprite]))
  end

  defp env_check_opts(sprites, opts \\ []) do
    opts
    |> Keyword.put(:require_codex_auth, requires_codex_auth?(sprites))
    |> Keyword.put(:require_canary_auth, requires_canary_auth?(sprites))
    |> Keyword.put(:sprite_auth_probes, sprite_auth_probes(sprites))
    |> maybe_put_sprite_auth_probe_target(sprite_auth_probe_target(sprites))
  end

  defp maybe_put_sprite_auth_probe_target(opts, nil), do: opts

  defp maybe_put_sprite_auth_probe_target(opts, sprite_name),
    do: Keyword.put(opts, :sprite_auth_probe_target, sprite_name)

  defp sprite_auth_probe_target(sprites) do
    Enum.find_value(sprites, fn sprite ->
      Map.get(sprite, :name) || Map.get(sprite, "name")
    end)
  end

  defp sprite_auth_probes(sprites) do
    {_, probes} =
      Enum.reduce(sprites, {MapSet.new(), []}, fn sprite, {seen_orgs, acc} ->
        org = Map.get(sprite, :org) || Map.get(sprite, "org")

        cond do
          not (is_binary(org) and org != "") ->
            {seen_orgs, acc}

          MapSet.member?(seen_orgs, org) ->
            {seen_orgs, acc}

          true ->
            probe = %{
              org: org,
              sprite: Map.get(sprite, :name) || Map.get(sprite, "name")
            }

            {MapSet.put(seen_orgs, org), [probe | acc]}
        end
      end)

    Enum.reverse(probes)
  end

  defp requires_codex_auth?(sprites) do
    Enum.any?(sprites, fn sprite ->
      harness = Map.get(sprite, :harness) || Map.get(sprite, "harness") || "codex"
      harness == "codex"
    end)
  end

  defp requires_canary_auth?(sprites) do
    Enum.any?(sprites, fn sprite ->
      role = Map.get(sprite, :role) || Map.get(sprite, "role")
      role in [:responder, "responder"]
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
      loop_alive: Map.get(status, :loop_alive, false),
      lifecycle_status: Map.get(status, :lifecycle_status, "unknown"),
      health: health_label(status)
    }
  end

  defp declared_sprite_row(sprite, probe_module) do
    status = declared_sprite_status(sprite, probe_module)

    %{
      name: sprite.name,
      role: sprite.role,
      repo: Map.get(sprite, :repo),
      reachable: Map.get(status, :reachable, false),
      healthy: Map.get(status, :healthy, false),
      paused: Map.get(status, :paused, false),
      busy: Map.get(status, :busy, false),
      loop_alive: Map.get(status, :loop_alive, false),
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
      running: Enum.count(rows, & &1.loop_alive),
      available_capacity:
        Enum.count(rows, fn row ->
          row.reachable and row.healthy and not row.paused and not row.loop_alive and not row.busy
        end)
    }
  end

  defp health_label(%{reachable: false}), do: "unreachable"
  defp health_label(%{healthy: true}), do: "healthy"
  defp health_label(_status), do: "needs setup"

  defp declared_sprite_status(sprite, Conductor.Sprite) do
    sprite
    |> status_call(&Conductor.Sprite.status/2)
    |> normalize_status(sprite.name)
  rescue
    _ -> unreachable_status(sprite.name)
  end

  defp declared_sprite_status(sprite, probe_module) do
    cond do
      function_exported?(probe_module, :status, 2) ->
        sprite
        |> status_call(&probe_module.status/2)
        |> normalize_status(sprite.name)

      function_exported?(probe_module, :status, 1) ->
        probe_module
        |> apply(:status, [sprite.name])
        |> normalize_status(sprite.name)

      true ->
        probe_status(sprite, probe_module)
    end
  rescue
    _ -> unreachable_status(sprite.name)
  end

  defp probe_status(sprite, probe_module) do
    name = sprite.name
    harness = Map.get(sprite, :harness)

    result =
      cond do
        function_exported?(probe_module, :status, 2) ->
          probe_module.status(name,
            harness: harness,
            org: Map.get(sprite, :org),
            repo: Map.get(sprite, :repo),
            clone_url: Map.get(sprite, :clone_url)
          )

        function_exported?(probe_module, :status, 1) ->
          probe_module.status(name)

        function_exported?(probe_module, :probe, 2) ->
          probe_module.probe(name, org: Map.get(sprite, :org))

        true ->
          probe_module.probe(name)
      end

    case result do
      result ->
        normalize_status(result, name)
    end
  rescue
    _ -> unreachable_status(sprite.name)
  end

  defp status_call(sprite, status_fn) do
    status_fn.(sprite.name,
      harness: Map.get(sprite, :harness),
      org: Map.get(sprite, :org),
      repo: Map.get(sprite, :repo),
      clone_url: Map.get(sprite, :clone_url)
    )
  end

  defp normalize_status({:ok, status}, name) when is_map(status) do
    paused = Map.get(status, :paused, false)
    busy = Map.get(status, :busy, false)
    loop_alive = Map.get(status, :loop_alive, false)

    %{
      sprite: name,
      reachable: Map.get(status, :reachable, true),
      healthy: Map.get(status, :healthy, Map.get(status, :reachable, true)),
      paused: paused,
      busy: busy,
      loop_alive: loop_alive,
      lifecycle_status:
        Map.get(status, :lifecycle_status, inferred_lifecycle_status(paused, loop_alive))
    }
  end

  defp normalize_status({:error, reason}, name), do: unreachable_status(name, reason)

  defp inferred_lifecycle_status(true, true), do: "draining"
  defp inferred_lifecycle_status(true, false), do: "paused"
  defp inferred_lifecycle_status(false, true), do: "running"
  defp inferred_lifecycle_status(false, false), do: "idle"

  defp unreachable_status(name, reason \\ nil) do
    %{
      sprite: name,
      reachable: false,
      healthy: false,
      paused: false,
      busy: false,
      loop_alive: false,
      lifecycle_status: "unreachable",
      error: reason
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

  defp ensure_start_preflight(%{role: :responder} = sprite), do: run_check_env_for_sprite(sprite)

  defp ensure_start_preflight(%{"role" => "responder"} = sprite),
    do: run_check_env_for_sprite(sprite)

  defp ensure_start_preflight(_sprite), do: :ok

  defp ensure_sprite_ready_for_start(_sprite, %{reachable: true, healthy: true}), do: :ok

  defp ensure_sprite_ready_for_start(sprite, _status) do
    with :ok <- run_check_env_for_sprite(sprite),
         :ok <-
           sprite_module().provision(sprite.name,
             repo: sprite.repo,
             clone_url: sprite.clone_url,
             default_branch: sprite.default_branch,
             persona: sprite.persona,
             persona_role: workspace_module().persona_for_role(sprite.role),
             harness: sprite.harness
           ),
         :ok <- maybe_force_sync_codex_auth(sprite) do
      :ok
    end
  end

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

  defp canary_client_module do
    Application.get_env(:conductor, :canary_client_module, Conductor.Canary.Client)
  end

  defp sprite_module do
    Application.get_env(:conductor, :sprite_module, Conductor.Sprite)
  end

  defp match_canary_annotations_args?(["incident", _incident_id]), do: true
  defp match_canary_annotations_args?(_), do: false

  defp build_annotation_attrs(opts) do
    with {:ok, agent} <- required_canary_opt(opts, :agent),
         {:ok, action} <- required_canary_opt(opts, :action),
         {:ok, metadata} <- parse_annotation_metadata(opts[:metadata]) do
      {:ok, %{agent: agent, action: action, metadata: metadata}}
    end
  end

  defp required_canary_opt(opts, key) do
    case Keyword.get(opts, key) do
      value when is_binary(value) and value != "" -> {:ok, value}
      _ -> {:error, "missing required --#{key}"}
    end
  end

  defp parse_annotation_metadata(nil), do: {:ok, nil}

  defp parse_annotation_metadata(raw) do
    case Jason.decode(raw) do
      {:ok, metadata} when is_map(metadata) -> {:ok, metadata}
      {:ok, _} -> {:error, "--metadata must be a JSON object"}
      {:error, _} -> {:error, "invalid JSON for --metadata"}
    end
  end

  defp render_incidents(%{"incidents" => []}), do: IO.puts("no active incidents")

  defp render_incidents(%{"incidents" => incidents}) when is_list(incidents) do
    Enum.each(incidents, fn incident ->
      IO.puts(
        "#{incident["id"]} #{incident["service"]} #{incident["severity"]} #{incident["state"]} #{incident["title"]}"
      )
    end)
  end

  defp render_incidents(response), do: IO.puts(Jason.encode!(response))

  defp render_report(%{"status" => status, "summary" => summary} = response) do
    IO.puts("status: #{status}")
    IO.puts("summary: #{summary}")
    IO.puts("incidents: #{response |> Map.get("incidents", []) |> length()}")
    IO.puts("error_groups: #{response |> Map.get("error_groups", []) |> length()}")
    IO.puts("targets: #{response |> Map.get("targets", []) |> length()}")
  end

  defp render_report(response), do: IO.puts(Jason.encode!(response))

  defp render_timeline(%{"summary" => summary, "events" => events}) when is_list(events) do
    IO.puts("summary: #{summary}")

    Enum.each(events, fn event ->
      IO.puts(
        "#{event["created_at"]} #{event["service"]} #{event["event"]} #{event["severity"]} #{event["summary"]}"
      )
    end)
  end

  defp render_timeline(response), do: IO.puts(Jason.encode!(response))

  defp render_annotations(%{"annotations" => []}), do: IO.puts("no annotations")

  defp render_annotations(%{"annotations" => annotations}) when is_list(annotations) do
    Enum.each(annotations, fn annotation ->
      IO.puts("#{annotation["created_at"]} #{annotation["agent"]} #{annotation["action"]}")
    end)
  end

  defp render_annotations(response), do: IO.puts(Jason.encode!(response))

  defp render_annotation(%{"created_at" => created_at, "agent" => agent, "action" => action}) do
    IO.puts("#{created_at} #{agent} #{action}")
  end

  defp render_annotation(response), do: IO.puts(Jason.encode!(response))
end
