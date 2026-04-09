defmodule Conductor.Fleet.HealthMonitorTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.Fleet.HealthMonitor
  alias Conductor.Store

  defmodule MockState do
    def put(key, value), do: :persistent_term.put({__MODULE__, key}, value)

    def get(key, default \\ nil) do
      try do
        :persistent_term.get({__MODULE__, key})
      rescue
        ArgumentError -> default
      end
    end

    def cleanup do
      for {{__MODULE__, _} = k, _} <- :persistent_term.get(), do: :persistent_term.erase(k)
    end
  end

  defmodule MockReconciler do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def reconcile_sprite(sprite) do
      case MockState.get({:sprite_health, sprite.name}, :healthy) do
        :healthy ->
          loop_alive = MockState.get({:loop_alive, sprite.name}, false)

          %{
            name: sprite.name,
            role: sprite.role,
            healthy: true,
            loop_alive: loop_alive,
            action: :none
          }

        :unhealthy ->
          %{
            name: sprite.name,
            role: sprite.role,
            healthy: false,
            loop_alive: false,
            action: :unreachable
          }
      end
    end
  end

  defmodule MockLauncher do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def launch(sprite, repo) do
      MockState.put({:launch, sprite.name}, repo)
      send(MockState.get(:test_pid), {:launched, sprite.name, repo})
      {:ok, "launched"}
    end
  end

  defmodule MockSprite do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def exec(sprite, command, _opts \\ []) do
      send(MockState.get(:test_pid), {:sprite_exec, sprite, command})

      cond do
        String.contains?(command, "git fetch") ->
          repo = extract_repo(command)
          branch = extract_branch(command)

          sha =
            MockState.get({:remote_head_sha, repo, branch}) ||
              MockState.get(:remote_master_sha, "abc123")

          {:ok, sha}

        true ->
          {:ok, "ok"}
      end
    end

    defp extract_repo(command) do
      case Regex.run(~r{cd '?/home/sprite/workspace/([^']+)'? &&}, command,
             capture: :all_but_first
           ) do
        [repo] -> repo
        _ -> "test/repo"
      end
    end

    defp extract_branch(command) do
      case Regex.run(~r{origin/([^']+)'?$}, command, capture: :all_but_first) do
        [branch] -> branch
        _ -> "master"
      end
    end

    def detect_auth_failure(name, _opts \\ []) do
      result = MockState.get({:auth_failure, name}, :ok)
      send(MockState.get(:test_pid), {:detect_auth_failure_called, name})
      result
    end
  end

  defmodule MockBootstrap do
    alias Conductor.Fleet.HealthMonitorTest.MockState

    def ensure_spellbook(sprite, _opts \\ []) do
      send(MockState.get(:test_pid), {:bootstrap, sprite})
      :ok
    end
  end

  setup do
    db_path = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.db")
    event_log = Path.join(System.tmp_dir!(), "health_mon_test_#{:rand.uniform(999_999)}.jsonl")

    stop_conductor_app()
    stop_process(HealthMonitor)
    stop_process(Store)
    {:ok, _} = Store.start_link(db_path: db_path, event_log: event_log)

    stop_process(Conductor.TaskSupervisor)
    {:ok, _} = Task.Supervisor.start_link(name: Conductor.TaskSupervisor)

    orig_reconciler = Application.get_env(:conductor, :reconciler_module)
    orig_launcher = Application.get_env(:conductor, :launcher_module)
    orig_sprite = Application.get_env(:conductor, :sprite_module)
    orig_bootstrap = Application.get_env(:conductor, :bootstrap_module)
    Application.put_env(:conductor, :reconciler_module, MockReconciler)
    Application.put_env(:conductor, :launcher_module, MockLauncher)
    Application.put_env(:conductor, :sprite_module, MockSprite)
    Application.put_env(:conductor, :bootstrap_module, MockBootstrap)
    MockState.put(:test_pid, self())

    on_exit(fn ->
      stop_process(HealthMonitor)
      stop_process(Conductor.TaskSupervisor)
      stop_process(Store)
      MockState.cleanup()

      if orig_reconciler,
        do: Application.put_env(:conductor, :reconciler_module, orig_reconciler),
        else: Application.delete_env(:conductor, :reconciler_module)

      if orig_launcher,
        do: Application.put_env(:conductor, :launcher_module, orig_launcher),
        else: Application.delete_env(:conductor, :launcher_module)

      if orig_sprite,
        do: Application.put_env(:conductor, :sprite_module, orig_sprite),
        else: Application.delete_env(:conductor, :sprite_module)

      if orig_bootstrap,
        do: Application.put_env(:conductor, :bootstrap_module, orig_bootstrap),
        else: Application.delete_env(:conductor, :bootstrap_module)

      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  defp start_monitor(opts \\ []) do
    {:ok, pid} = HealthMonitor.start_link(interval_ms: Keyword.get(opts, :interval_ms, 60_000))
    pid
  end

  defp wait_for(fun, attempts \\ 20)
  defp wait_for(_fun, 0), do: flunk("timed out waiting for health monitor state")

  defp wait_for(fun, attempts) do
    case fun.() do
      nil ->
        Process.sleep(25)
        wait_for(fun, attempts - 1)

      value ->
        value
    end
  end

  test "starts with empty state and reports status" do
    start_monitor()

    status = HealthMonitor.status()
    assert status.sprites == %{}
    assert status.repo == nil
  end

  test "configure sets sprites and initial health with launching state" do
    start_monitor()

    sprites = [
      %{name: "bb-polisher", role: :polisher, harness: "codex"},
      %{name: "bb-fixer", role: :fixer, harness: "codex"}
    ]

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    status = HealthMonitor.status()
    assert status.sprites["bb-polisher"] == :launching
    assert status.sprites["bb-fixer"] == :unhealthy
  end

  test "launching sprite with loop alive transitions to healthy" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    assert HealthMonitor.status().sprites["bb-polisher"] == :launching

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher loop confirmed"
    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
  end

  test "launching sprite without loop stays launching" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(100)

    assert HealthMonitor.status().sprites["bb-polisher"] == :launching
  end

  test "launching sprite times out after max ticks" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    log =
      capture_log(fn ->
        # Tick 3 times to exceed @max_launch_ticks (3)
        for _ <- 1..3 do
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end
      end)

    assert log =~ "launch timed out"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end

  test "healthy sprite with loop exiting transitions to unhealthy" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    # Start as healthy (with confirmed loop)
    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-polisher"])
    )

    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy

    # Simulate loop death
    MockState.put({:loop_alive, "bb-polisher"}, false)

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher loop exited"
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end

  test "detects unhealthy→recovered and relaunches when no loop" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher recovered, relaunching loop"
    assert HealthMonitor.status().sprites["bb-polisher"] == :launching
    assert_receive {:launched, "bb-polisher", "test/repo"}
  end

  test "detects unhealthy→recovered with loop already running" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-polisher recovered (loop already running)"
    assert HealthMonitor.status().sprites["bb-polisher"] == :healthy
  end

  test "detects healthy→unhealthy transition and logs degradation" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-fixer", role: :fixer, harness: "codex"}]
    MockState.put({:sprite_health, "bb-fixer"}, :unhealthy)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      healthy: MapSet.new(["bb-fixer"])
    )

    assert HealthMonitor.status().sprites["bb-fixer"] == :healthy

    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "bb-fixer degraded"
    assert HealthMonitor.status().sprites["bb-fixer"] == :unhealthy
  end

  test "no-ops when sprites list is empty" do
    start_monitor(interval_ms: 60_000)

    send(Process.whereis(HealthMonitor), :check)
    Process.sleep(50)

    assert HealthMonitor.status().sprites == %{}
  end

  test "check_now triggers immediate probe" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

    HealthMonitor.check_now()

    wait_for(fn ->
      if HealthMonitor.status().sprites["bb-polisher"] == :healthy, do: :ok
    end)
  end

  test "records recovery event in store" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(sprites: sprites, repo: "test/repo")

    HealthMonitor.check_now()

    wait_for(fn ->
      if HealthMonitor.status().sprites["bb-polisher"] == :healthy, do: :ok
    end)

    assert Enum.any?(Store.list_events("fleet"), &(&1["event_type"] == "sprite_recovered"))
  end

  test "relaunches a recovered sprite with its configured repo override" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex", repo: "other/repo"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, false)

    HealthMonitor.configure(sprites: sprites, repo: "default/repo")
    HealthMonitor.check_now()

    assert_receive {:launched, "bb-polisher", "other/repo"}
  end

  test "rapid exit detection and backoff suppresses relaunch after repeated fast exits" do
    start_monitor(interval_ms: 60_000)

    sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
    MockState.put({:sprite_health, "bb-polisher"}, :healthy)
    MockState.put({:loop_alive, "bb-polisher"}, true)

    HealthMonitor.configure(
      sprites: sprites,
      repo: "test/repo",
      launching: MapSet.new(["bb-polisher"])
    )

    # Cycle 3 rapid exits: launching → healthy (confirm) → unhealthy (rapid exit)
    for i <- 1..3 do
      # Confirm loop alive → :healthy (sets launch_time)
      MockState.put({:loop_alive, "bb-polisher"}, true)

      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      assert HealthMonitor.status().sprites["bb-polisher"] == :healthy

      # Kill loop → :unhealthy (rapid exit, since launch_time was just set)
      MockState.put({:loop_alive, "bb-polisher"}, false)

      log =
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end)

      assert log =~ "rapid exit (#{i}x)"
      assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy

      # Recovery → relaunch (count < 3 allows it)
      if i < 3 do
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end)

        assert HealthMonitor.status().sprites["bb-polisher"] == :launching
      end
    end

    # After 3 rapid exits, relaunch should be suppressed (backoff)
    log =
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

    assert log =~ "backing off relaunch"
    # Should stay :unhealthy, not transition to :launching
    assert HealthMonitor.status().sprites["bb-polisher"] == :unhealthy
  end

  # --- Reload tests ---

  describe "self-reload after merge" do
    test "detects SHA change and triggers reload for idle sprites" do
      start_monitor(interval_ms: 60_000)

      sprites = [
        %{name: "bb-polisher", role: :polisher, harness: "codex", default_branch: "main"}
      ]

      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, false)

      # Initial SHA
      MockState.put({:remote_head_sha, "test/repo", "main"}, "aaa111")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher"])
      )

      # First check: establishes initial SHA, sprite is idle (healthy + no loop)
      # First cycle with no prior SHA just records it — no reload
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      # Drain messages from first check
      flush_messages()

      # Simulate new commit on the configured origin branch
      MockState.put({:remote_head_sha, "test/repo", "main"}, "bbb222")

      # Second check: detects SHA change, triggers reload for idle sprite
      log =
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(200)
        end)

      assert log =~ "new commits on test/repo@origin/main"

      # Collect all messages from second check
      msgs = flush_messages()
      exec_cmds = for {:sprite_exec, _, cmd} <- msgs, do: cmd
      assert Enum.any?(exec_cmds, &String.contains?(&1, "git checkout -f 'origin/main'"))

      # Should have relaunched (launcher handles bootstrap internally)
      assert Enum.any?(msgs, &match?({:launched, "bb-polisher", "test/repo"}, &1))
    end

    test "does not interrupt active sprites on reload" do
      start_monitor(interval_ms: 60_000)

      sprites = [
        %{name: "bb-polisher", role: :polisher, harness: "codex"},
        %{name: "bb-builder", role: :builder, harness: "codex"}
      ]

      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, true)
      MockState.put({:sprite_health, "bb-builder"}, :healthy)
      MockState.put({:loop_alive, "bb-builder"}, false)

      MockState.put(:remote_master_sha, "aaa111")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher", "bb-builder"])
      )

      # First check: establishes SHA
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      flush_messages()

      # Change SHA
      MockState.put(:remote_master_sha, "ccc333")

      # Second check: only idle sprite (bb-builder) should be reloaded
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(200)
      end)

      msgs = flush_messages()

      # bb-builder (idle) should be relaunched
      assert Enum.any?(msgs, &match?({:launched, "bb-builder", "test/repo"}, &1))
      # bb-polisher (active) should NOT be relaunched
      refute Enum.any?(msgs, &match?({:launched, "bb-polisher", _}, &1))
    end

    test "records fleet_reload event in store" do
      start_monitor(interval_ms: 60_000)

      sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, false)
      MockState.put(:remote_master_sha, "aaa111")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher"])
      )

      # First check: establishes SHA
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      # Change SHA
      MockState.put(:remote_master_sha, "ddd444")

      # Second check: triggers reload
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(200)
      end)

      events = Store.list_events("fleet")

      reload_event =
        Enum.find(events, &(&1["event_type"] == "fleet_reload"))

      assert reload_event
      assert reload_event["payload"]["old_sha"] == "aaa111"
      assert reload_event["payload"]["new_sha"] == "ddd444"
    end

    test "no reload when SHA unchanged" do
      start_monitor(interval_ms: 60_000)

      sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, false)
      MockState.put(:remote_master_sha, "aaa111")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher"])
      )

      # First check: establishes SHA
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      # SHA stays the same — clear any existing messages
      flush_messages()

      # Second check: same SHA, no reload
      log =
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end)

      refute log =~ "new commits on origin/master"

      events = Store.list_events("fleet")
      refute Enum.any?(events, &(&1["event_type"] == "fleet_reload"))
    end

    test "updates master_sha after reload so subsequent checks are no-ops" do
      start_monitor(interval_ms: 60_000)

      sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, false)
      MockState.put(:remote_master_sha, "aaa111")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher"])
      )

      # First check: establishes SHA
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      # Change SHA
      MockState.put(:remote_master_sha, "eee555")

      # Second check: triggers reload, updates SHA
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(200)
      end)

      # Verify reload happened
      events = Store.list_events("fleet")
      assert Enum.any?(events, &(&1["event_type"] == "fleet_reload"))

      # Third check: same SHA as updated, no new reload event
      reload_count_before = Enum.count(events, &(&1["event_type"] == "fleet_reload"))

      # Need sprite to be healthy again for a meaningful test
      MockState.put({:loop_alive, "bb-polisher"}, true)

      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      events_after = Store.list_events("fleet")
      reload_count_after = Enum.count(events_after, &(&1["event_type"] == "fleet_reload"))

      assert reload_count_after == reload_count_before
    end

    test "fetches once per cycle not per sprite" do
      start_monitor(interval_ms: 60_000)

      sprites = [
        %{name: "bb-polisher", role: :polisher, harness: "codex"},
        %{name: "bb-builder", role: :builder, harness: "codex"}
      ]

      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, false)
      MockState.put({:sprite_health, "bb-builder"}, :healthy)
      MockState.put({:loop_alive, "bb-builder"}, false)
      MockState.put(:remote_master_sha, "aaa111")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher", "bb-builder"])
      )

      # Run one check cycle
      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      # Count fetch commands — should be exactly one per check cycle
      msgs = flush_messages()

      fetch_count =
        Enum.count(msgs, fn
          {:sprite_exec, _, cmd} -> String.contains?(cmd, "git fetch")
          _ -> false
        end)

      assert fetch_count == 1
    end

    test "reload only applies to sprites on the changed repo and branch" do
      start_monitor(interval_ms: 60_000)

      sprites = [
        %{
          name: "bb-polisher",
          role: :polisher,
          harness: "codex",
          repo: "test/repo",
          default_branch: "main"
        },
        %{
          name: "bb-builder",
          role: :builder,
          harness: "codex",
          repo: "other/repo",
          default_branch: "main"
        }
      ]

      Enum.each(["bb-polisher", "bb-builder"], fn name ->
        MockState.put({:sprite_health, name}, :healthy)
        MockState.put({:loop_alive, name}, false)
      end)

      MockState.put({:remote_head_sha, "test/repo", "main"}, "aaa111")
      MockState.put({:remote_head_sha, "other/repo", "main"}, "zzz999")

      HealthMonitor.configure(
        sprites: sprites,
        repo: "fallback/repo",
        launching: MapSet.new(["bb-polisher", "bb-builder"])
      )

      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      flush_messages()

      MockState.put({:remote_head_sha, "test/repo", "main"}, "bbb222")
      MockState.put({:remote_head_sha, "other/repo", "main"}, "zzz999")

      log =
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(200)
        end)

      assert log =~ "new commits on test/repo@origin/main"

      msgs = flush_messages()
      assert Enum.any?(msgs, &match?({:launched, "bb-polisher", "test/repo"}, &1))
      refute Enum.any?(msgs, &match?({:launched, "bb-builder", _}, &1))
    end
  end

  defp flush_messages(acc \\ []) do
    receive do
      msg -> flush_messages([msg | acc])
    after
      0 -> Enum.reverse(acc)
    end
  end

  describe "auth failure detection" do
    test "loop exit with auth failure emits sprite_auth_failure event" do
      start_monitor(interval_ms: 60_000)

      sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, true)
      MockState.put({:auth_failure, "bb-polisher"}, {:auth_failure, "refresh_token_reused"})

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher"])
      )

      # Simulate loop death
      MockState.put({:loop_alive, "bb-polisher"}, false)

      log =
        capture_log(fn ->
          send(Process.whereis(HealthMonitor), :check)
          Process.sleep(100)
        end)

      assert log =~ "auth failure detected"
      assert Enum.any?(Store.list_events("fleet"), &(&1["event_type"] == "sprite_auth_failure"))
    end

    test "loop exit without auth failure does not emit sprite_auth_failure event" do
      start_monitor(interval_ms: 60_000)

      sprites = [%{name: "bb-polisher", role: :polisher, harness: "codex"}]
      MockState.put({:sprite_health, "bb-polisher"}, :healthy)
      MockState.put({:loop_alive, "bb-polisher"}, true)

      HealthMonitor.configure(
        sprites: sprites,
        repo: "test/repo",
        healthy: MapSet.new(["bb-polisher"])
      )

      MockState.put({:loop_alive, "bb-polisher"}, false)

      capture_log(fn ->
        send(Process.whereis(HealthMonitor), :check)
        Process.sleep(100)
      end)

      refute Enum.any?(Store.list_events("fleet"), &(&1["event_type"] == "sprite_auth_failure"))
    end
  end
end
