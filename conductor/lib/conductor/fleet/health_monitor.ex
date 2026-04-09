defmodule Conductor.Fleet.HealthMonitor do
  @moduledoc """
  Periodic fleet health monitor. Probes sprites and tracks lifecycle state.

  Tri-state per sprite:
  - `:launching` — dispatched but loop not yet confirmed alive
  - `:healthy`   — provisioned + loop alive
  - `:unhealthy` — degraded, loop died, or launch timed out

  On recovery (`:unhealthy` → probe ready):
  - If loop already alive → `:healthy` (external restart)
  - If no loop → `:launching` + relaunch

  Self-reload: polls the configured remote branch once per health check cycle.
  When new commits are detected, idle sprites (reachable + no loop) are
  re-provisioned with fresh code and relaunched. Active sprites are
  left alone — they pick up new code on their next relaunch.

  Deep module: hides all sprite lifecycle recovery behind a simple status/0 interface.
  """

  use GenServer
  require Logger

  alias Conductor.{Config, Shell, Store, Workspace}
  alias Conductor.Fleet.Reconciler

  @max_launch_ticks 3
  @rapid_exit_threshold_ms 120_000
  @max_rapid_exits 3
  @rapid_exit_backoff_cap_ms 1_800_000

  defstruct [
    :repo,
    :interval_ms,
    :timer_ref,
    sprites: [],
    tracked_heads: %{},
    known_health: %{},
    launch_ticks: %{},
    launch_times: %{},
    rapid_exit_counts: %{}
  ]

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec configure(keyword()) :: :ok
  def configure(opts) do
    GenServer.call(__MODULE__, {:configure, opts})
  end

  @spec status() :: map()
  def status do
    GenServer.call(__MODULE__, :status)
  end

  @spec check_now() :: :ok
  def check_now do
    send(__MODULE__, :check)
    :ok
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    interval_ms = Keyword.get(opts, :interval_ms, Config.fleet_health_check_interval_ms())
    {:ok, %__MODULE__{interval_ms: interval_ms}}
  end

  @impl true
  def handle_call({:configure, opts}, _from, state) do
    sprites = Keyword.get(opts, :sprites, [])
    repo = Keyword.fetch!(opts, :repo)
    initial_launching = Keyword.get(opts, :launching, MapSet.new())
    initial_healthy = Keyword.get(opts, :healthy, MapSet.new())

    known_health =
      Map.new(sprites, fn s ->
        cond do
          MapSet.member?(initial_healthy, s.name) -> {s.name, :healthy}
          MapSet.member?(initial_launching, s.name) -> {s.name, :launching}
          true -> {s.name, :unhealthy}
        end
      end)

    launch_ticks = Map.new(sprites, fn s -> {s.name, 0} end)
    launch_times = Map.new(sprites, fn s -> {s.name, nil} end)
    rapid_exit_counts = Map.new(sprites, fn s -> {s.name, 0} end)

    if state.timer_ref, do: Process.cancel_timer(state.timer_ref)
    ref = schedule_check(state.interval_ms)

    tracked_heads =
      state.tracked_heads
      |> Map.take(Enum.map(sprites, &sprite_ref_key(&1, repo)))

    state = %{
      state
      | sprites: sprites,
        repo: repo,
        tracked_heads: tracked_heads,
        known_health: known_health,
        launch_ticks: launch_ticks,
        launch_times: launch_times,
        rapid_exit_counts: rapid_exit_counts,
        timer_ref: ref
    }

    {:reply, :ok, state}
  end

  @impl true
  def handle_call(:status, _from, state) do
    {:reply, %{sprites: state.known_health, repo: state.repo}, state}
  end

  @impl true
  def handle_info(:check, %{sprites: []} = state), do: {:noreply, state}

  @impl true
  def handle_info(:check, state) do
    state = check_and_recover(state)
    ref = schedule_check(state.interval_ms)
    {:noreply, %{state | timer_ref: ref}}
  end

  @impl true
  def handle_info(_msg, state), do: {:noreply, state}

  # --- Private ---

  defp check_and_recover(state) do
    # 1. Poll each configured repo/branch ref once per cycle
    {state, changed_refs} = poll_remote_heads(state)

    # 2. Probe all sprites, collect results
    probed =
      Enum.map(state.sprites, fn sprite ->
        {ready?, loop_alive?} = probe_sprite(sprite)
        {sprite, ready?, loop_alive?}
      end)

    # 3. Process each sprite: reload idle ones if needed, else normal transition
    Enum.reduce(probed, state, fn {sprite, ready?, loop_alive?}, acc ->
      old = Map.get(acc.known_health, sprite.name, :unhealthy)
      ref_changed? = MapSet.member?(changed_refs, sprite_ref_key(sprite, acc.repo))

      if ref_changed? and ready? and not loop_alive? do
        reload_sprite(acc, sprite)
      else
        transition(acc, sprite, old, ready?, loop_alive?)
      end
    end)
  end

  # Poll each configured repo/branch head on one representative sprite per ref.
  # Returns {updated_state, changed_ref_keys}.
  defp poll_remote_heads(%{sprites: []} = state), do: {state, MapSet.new()}

  defp poll_remote_heads(state) do
    state.sprites
    |> Enum.group_by(&sprite_ref_key(&1, state.repo))
    |> Enum.reduce({state, MapSet.new()}, fn {{repo, branch} = ref_key, sprites},
                                             {acc, changed} ->
      case find_probe_sprite(acc, sprites) do
        nil ->
          {acc, changed}

        sprite ->
          workspace = Conductor.Workspace.repo_root(repo)

          case Workspace.validate_branch(branch) do
            :ok ->
              cmd =
                "cd #{shell_quote(workspace)} && git fetch origin --quiet && " <>
                  "git rev-parse #{shell_quote(Workspace.origin_branch_ref(branch))}"

              case sprite_mod().exec(sprite.name, cmd, timeout: 30_000) do
                {:ok, output} ->
                  sha = String.trim(output)
                  previous_sha = Map.get(acc.tracked_heads, ref_key)

                  cond do
                    previous_sha == nil ->
                      {%{acc | tracked_heads: Map.put(acc.tracked_heads, ref_key, sha)}, changed}

                    sha != previous_sha ->
                      Logger.info(
                        "[health] new commits on #{repo}@origin/#{branch}: #{String.slice(previous_sha, 0, 7)}..#{String.slice(sha, 0, 7)}"
                      )

                      record_fleet_reload_event(previous_sha, sha, repo, branch)

                      {put_in(acc.tracked_heads[ref_key], sha), MapSet.put(changed, ref_key)}

                    true ->
                      {acc, changed}
                  end

                {:error, _msg, _code} ->
                  {acc, changed}
              end

            {:error, :invalid_branch} ->
              Logger.warning(
                "[health] invalid default_branch for #{sprite.name}: #{inspect(branch)}"
              )

              {acc, changed}
          end
      end
    end)
  end

  defp find_probe_sprite(state, sprites) do
    Enum.find(sprites, fn sprite ->
      Map.get(state.known_health, sprite.name) in [:healthy, :launching]
    end) || List.first(sprites)
  end

  # Re-provision an idle sprite with fresh code from master.
  # All heavy work (git pull, bootstrap, launch) runs in a background task
  # to avoid blocking the health monitor GenServer loop.
  defp reload_sprite(state, sprite) do
    repo = sprite_repo(sprite, state.repo)
    branch = sprite_default_branch(sprite)

    Task.Supervisor.start_child(Conductor.TaskSupervisor, fn ->
      workspace = Workspace.repo_root(repo)

      case Workspace.validate_branch(branch) do
        :ok ->
          pull_cmd =
            "cd #{shell_quote(workspace)} && git fetch origin --quiet && " <>
              "#{Workspace.checkout_branch_command(branch)} && " <>
              "git clean -fd"

          case sprite_mod().exec(sprite.name, pull_cmd, timeout: 30_000) do
            {:ok, _} ->
              launcher_mod().launch(sprite, repo)

            {:error, msg, _code} ->
              Logger.warning("[health] #{sprite.name} reload pull failed: #{msg}")
          end

        {:error, :invalid_branch} ->
          Logger.warning("[health] invalid default_branch for #{sprite.name}: #{inspect(branch)}")
      end
    end)

    state
    |> put_health(sprite.name, :launching)
    |> put_launch_ticks(sprite.name, 0)
  end

  defp record_fleet_reload_event(old_sha, new_sha, repo, branch) do
    try do
      Store.record_event("fleet", "fleet_reload", %{
        old_sha: old_sha,
        new_sha: new_sha,
        repo: repo,
        branch: branch
      })
    catch
      :exit, _ -> :ok
    end
  end

  # --- Transition table ---
  # See backlog.d/004 for the full design rationale.

  # :launching + ready + loop alive → :healthy (confirmed)
  defp transition(state, sprite, :launching, true, true) do
    Logger.info("[health] #{sprite.name} loop confirmed")

    record_fleet_event("sprite_loop_confirmed", sprite)

    state
    |> put_health(sprite.name, :healthy)
    |> put_launch_ticks(sprite.name, 0)
    |> put_launch_time(sprite.name, System.monotonic_time(:millisecond))
  end

  # :launching + ready + no loop → stay :launching (still starting), but timeout
  defp transition(state, sprite, :launching, true, false) do
    ticks = Map.get(state.launch_ticks, sprite.name, 0) + 1

    if ticks >= @max_launch_ticks do
      Logger.warning("[health] #{sprite.name} launch timed out after #{ticks} probe(s)")

      record_fleet_event("sprite_launch_timeout", sprite, %{ticks: ticks})

      state
      |> put_health(sprite.name, :unhealthy)
      |> put_launch_ticks(sprite.name, 0)
    else
      put_launch_ticks(state, sprite.name, ticks)
    end
  end

  # :launching + unhealthy → :unhealthy (setup failed)
  defp transition(state, sprite, :launching, false, _loop_alive) do
    Logger.warning("[health] #{sprite.name} degraded during launch")

    record_fleet_event("sprite_degraded", sprite)

    state
    |> put_health(sprite.name, :unhealthy)
    |> put_launch_ticks(sprite.name, 0)
  end

  # :healthy + ready + loop alive → no-op
  defp transition(state, _sprite, :healthy, true, true), do: state

  # :healthy + ready + no loop → :unhealthy (loop exited)
  defp transition(state, sprite, :healthy, true, false) do
    launch_time = Map.get(state.launch_times, sprite.name)
    rapid? = rapid_exit?(launch_time)

    check_auth_failure(sprite)

    if rapid? do
      count = Map.get(state.rapid_exit_counts, sprite.name, 0) + 1
      Logger.warning("[health] #{sprite.name} rapid exit (#{count}x) — likely no work available")
      record_fleet_event("sprite_loop_exited", sprite, %{rapid: true, count: count})

      state
      |> put_health(sprite.name, :unhealthy)
      |> put_rapid_exit_count(sprite.name, count)
    else
      Logger.warning("[health] #{sprite.name} loop exited")
      record_fleet_event("sprite_loop_exited", sprite)

      state
      |> put_health(sprite.name, :unhealthy)
      |> put_rapid_exit_count(sprite.name, 0)
    end
  end

  # :healthy + unhealthy → :unhealthy (degraded)
  defp transition(state, sprite, :healthy, false, _loop_alive) do
    Logger.warning("[health] #{sprite.name} degraded")

    record_fleet_event("sprite_degraded", sprite)

    put_health(state, sprite.name, :unhealthy)
  end

  # :unhealthy + ready + loop alive → :healthy (recovered, loop already running)
  defp transition(state, sprite, :unhealthy, true, true) do
    Logger.info("[health] #{sprite.name} recovered (loop already running)")

    record_fleet_event("sprite_recovered", sprite)

    put_health(state, sprite.name, :healthy)
  end

  # :unhealthy + ready + no loop → :launching + relaunch (with backoff)
  defp transition(state, sprite, :unhealthy, true, false) do
    rapid_count = Map.get(state.rapid_exit_counts, sprite.name, 0)

    if rapid_count >= @max_rapid_exits and not backoff_elapsed?(state, sprite) do
      backoff_ms = rapid_exit_backoff_ms(rapid_count, state.interval_ms)

      Logger.info(
        "[health] #{sprite.name} backing off relaunch (#{rapid_count} rapid exits, #{div(backoff_ms, 1000)}s)"
      )

      state
    else
      Logger.info("[health] #{sprite.name} recovered, relaunching loop")

      record_fleet_event("sprite_recovered", sprite)

      repo = sprite_repo(sprite, state.repo)

      if repo do
        Task.Supervisor.start_child(Conductor.TaskSupervisor, fn ->
          launcher_mod().launch(sprite, repo)
        end)
      end

      # Reset rapid exit counter only when relaunching after backoff elapsed,
      # not on normal relaunches (counter < threshold). This gives the sprite
      # a fresh budget of rapid exits after it's waited out the backoff.
      reset_count = rapid_count >= @max_rapid_exits

      state
      |> put_health(sprite.name, :launching)
      |> put_launch_ticks(sprite.name, 0)
      |> then(fn s -> if reset_count, do: put_rapid_exit_count(s, sprite.name, 0), else: s end)
    end
  end

  # :unhealthy + unhealthy → no-op
  defp transition(state, _sprite, :unhealthy, false, _loop_alive), do: state

  # --- Probing ---

  defp probe_sprite(sprite) do
    case reconciler_mod().reconcile_sprite(sprite) do
      %{healthy: true, loop_alive: loop_alive} ->
        {true, loop_alive == true}

      %{healthy: true} ->
        # Backwards compat: reconciler doesn't report loop_alive yet
        {true, false}

      _ ->
        {false, false}
    end
  end

  defp put_health(state, name, health) do
    %{state | known_health: Map.put(state.known_health, name, health)}
  end

  defp put_launch_ticks(state, name, ticks) do
    %{state | launch_ticks: Map.put(state.launch_ticks, name, ticks)}
  end

  defp schedule_check(interval_ms) when is_integer(interval_ms) do
    Process.send_after(self(), :check, interval_ms)
  end

  defp record_fleet_event(event_type, sprite, extra \\ %{}) do
    payload = Map.merge(%{name: sprite.name, role: to_string(sprite.role)}, extra)

    try do
      Store.record_event("fleet", event_type, payload)
    catch
      :exit, _ -> :ok
    end
  end

  defp put_launch_time(state, name, time) do
    %{state | launch_times: Map.put(state.launch_times, name, time)}
  end

  defp put_rapid_exit_count(state, name, count) do
    %{state | rapid_exit_counts: Map.put(state.rapid_exit_counts, name, count)}
  end

  defp rapid_exit?(nil), do: false

  defp rapid_exit?(launch_time) do
    System.monotonic_time(:millisecond) - launch_time < @rapid_exit_threshold_ms
  end

  defp backoff_elapsed?(state, sprite) do
    launch_time = Map.get(state.launch_times, sprite.name)
    rapid_count = Map.get(state.rapid_exit_counts, sprite.name, 0)

    case launch_time do
      nil ->
        true

      t ->
        System.monotonic_time(:millisecond) - t >=
          rapid_exit_backoff_ms(rapid_count, state.interval_ms)
    end
  end

  defp rapid_exit_backoff_ms(count, interval_ms) do
    min(trunc(interval_ms * :math.pow(2, count - @max_rapid_exits)), @rapid_exit_backoff_cap_ms)
  end

  defp sprite_repo(sprite, fallback_repo), do: Map.get(sprite, :repo, fallback_repo)
  defp sprite_default_branch(sprite), do: Map.get(sprite, :default_branch, "master")

  defp sprite_ref_key(sprite, fallback_repo),
    do: {sprite_repo(sprite, fallback_repo), sprite_default_branch(sprite)}

  defp shell_quote(value), do: Shell.quote_arg(to_string(value))

  defp launcher_mod do
    Application.get_env(:conductor, :launcher_module, Conductor.Launcher)
  end

  defp reconciler_mod do
    Application.get_env(:conductor, :reconciler_module, Reconciler)
  end

  defp sprite_mod do
    Application.get_env(:conductor, :sprite_module, Conductor.Sprite)
  end

  defp check_auth_failure(sprite) do
    case sprite_mod().detect_auth_failure(sprite.name) do
      {:auth_failure, reason} ->
        Logger.warning("[health] #{sprite.name} auth failure detected: #{reason}")
        record_fleet_event("sprite_auth_failure", sprite, %{reason: reason})

      :ok ->
        :ok
    end
  end
end
