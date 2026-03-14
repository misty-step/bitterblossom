defmodule Conductor.CLI do
  @moduledoc "Escript entry point. Parses args and delegates to Conductor."

  @commands ~w(run-once loop shape setup show-runs show-events show-incidents show-waivers check-env dashboard)

  def main(args) do
    Application.ensure_all_started(:conductor)

    case args do
      ["run-once" | rest] ->
        cmd_run_once(rest)

      ["loop" | rest] ->
        cmd_loop(rest)

      ["shape" | rest] ->
        cmd_shape(rest)

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

      ["setup" | rest] ->
        cmd_setup(rest)

      ["check-env" | _] ->
        cmd_check_env()

      [cmd | _] ->
        IO.puts("unknown command: #{cmd}\navailable: #{Enum.join(@commands, ", ")}")

      [] ->
        IO.puts("usage: conductor <command> [options]\navailable: #{Enum.join(@commands, ", ")}")
    end
  end

  defp cmd_run_once(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [
          repo: :string,
          issue: :integer,
          worker: :string,
          trusted_external_surface: [:string, :keep]
        ]
      )

    repo = Keyword.fetch!(opts, :repo)
    issue = Keyword.fetch!(opts, :issue)
    worker = Keyword.fetch!(opts, :worker)
    surfaces = Keyword.get_values(opts, :trusted_external_surface)

    IO.puts("conductor run-once: issue ##{issue} on #{worker}")

    case Conductor.run_once(
           repo: repo,
           issue: issue,
           worker: worker,
           trusted_surfaces: surfaces
         ) do
      {:ok, :merged} ->
        IO.puts("run complete: merged")

      {:ok, :blocked} ->
        IO.puts("run complete: blocked")
        System.halt(2)

      {:ok, :failed} ->
        IO.puts("run complete: failed")
        System.halt(1)

      {:ok, phase} ->
        IO.puts("run complete: #{phase}")

      {:error, reason} ->
        IO.puts("run failed: #{inspect(reason)}")
        System.halt(1)
    end
  end

  defp cmd_loop(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [
          repo: :string,
          label: :string,
          worker: [:string, :keep],
          trusted_external_surface: [:string, :keep]
        ]
      )

    repo = Keyword.fetch!(opts, :repo)
    workers = Keyword.get_values(opts, :worker)
    label = Keyword.get(opts, :label, "autopilot")
    surfaces = Keyword.get_values(opts, :trusted_external_surface)

    IO.puts("conductor loop: repo=#{repo} workers=#{Enum.join(workers, ",")} label=#{label}")

    Conductor.Orchestrator.start_loop(
      repo: repo,
      workers: workers,
      label: label,
      trusted_surfaces: surfaces
    )

    # Block forever — the orchestrator runs in the supervision tree
    Process.sleep(:infinity)
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

    # The app is already running (started in main/1) without the endpoint,
    # because :start_dashboard was false at that point. Configure the endpoint
    # and start it directly into the running supervisor.
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

  defp cmd_setup(args) do
    {opts, _, _} =
      OptionParser.parse(args,
        strict: [worker: [:string, :keep]]
      )

    workers = Keyword.get_values(opts, :worker)

    if workers == [] do
      IO.puts("usage: conductor setup --worker SPRITE [--worker SPRITE ...]")
      System.halt(1)
    end

    token = Conductor.Config.github_token!()

    results =
      Enum.map(workers, fn worker ->
        IO.puts("setting up gh auth on #{worker}...")

        case Conductor.Sprite.setup_gh_auth(worker, token) do
          :ok ->
            IO.puts("  #{worker}: gh auth ok")
            :ok

          {:error, reason} ->
            IO.puts("  #{worker}: FAILED — #{reason}")
            :error
        end
      end)

    if Enum.any?(results, &(&1 == :error)) do
      System.halt(1)
    end
  end

  defp cmd_check_env do
    Conductor.Config.check_env!()
  rescue
    e ->
      IO.puts("environment check failed: #{Exception.message(e)}")
      System.halt(1)
  end
end
