defmodule Conductor.CLI do
  @moduledoc "Escript entry point. Parses args and delegates to Conductor."

  @commands ~w(run-once loop show-runs show-events show-incidents show-waivers check-env)

  def main(args) do
    Application.ensure_all_started(:conductor)

    case args do
      ["run-once" | rest] ->
        cmd_run_once(rest)

      ["loop" | rest] ->
        cmd_loop(rest)

      ["show-runs" | rest] ->
        cmd_show_runs(rest)

      ["show-events" | rest] ->
        cmd_show_events(rest)

      ["show-incidents" | rest] ->
        cmd_show_incidents(rest)

      ["show-waivers" | rest] ->
        cmd_show_waivers(rest)

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
    run_id = Keyword.fetch!(opts, :run_id)

    incidents = Conductor.Store.list_incidents(run_id)

    IO.puts(
      Jason.encode!(%{
        run_id: run_id,
        incident_count: length(incidents),
        incidents: incidents
      })
    )
  end

  defp cmd_show_waivers(args) do
    {opts, _, _} = OptionParser.parse(args, strict: [run_id: :string])
    run_id = Keyword.fetch!(opts, :run_id)

    waivers = Conductor.Store.list_waivers(run_id)

    IO.puts(
      Jason.encode!(%{
        run_id: run_id,
        waiver_count: length(waivers),
        waivers: waivers
      })
    )
  end

  defp cmd_check_env do
    Conductor.Config.check_env!()
  rescue
    e ->
      IO.puts("environment check failed: #{Exception.message(e)}")
      System.halt(1)
  end
end
