defmodule Conductor.CLIFleetTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.CLI

  @conductor_dir Path.expand("../..", __DIR__)

  defmodule MockWorker do
    def status("bb-weaver-1", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-weaver-1",
           reachable: true,
           harness_ready: true,
           git_ready: true,
           healthy: true
         }}

    def status("bb-weaver-2", _opts), do: {:error, "connection refused"}

    def status("bb-weaver-3", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-weaver-3",
           reachable: true,
           harness_ready: true,
           git_ready: false,
           healthy: false
         }}

    def status("bb-weaver-4", _opts),
      do:
        {:ok,
         %{
           sprite: "bb-weaver-4",
           reachable: true,
           harness_ready: true,
           git_ready: false,
           healthy: false
         }}
  end

  defmodule ProbeOnlyWorker do
    def probe("bb-weaver-1", _opts), do: {:ok, %{sprite: "bb-weaver-1", reachable: true}}
    def probe(_worker, _opts), do: {:error, "connection refused"}
  end

  defmodule MockReconciler do
    def reconcile_all(sprites, _opts \\ []) do
      send(self(), {:reconciled, Enum.map(sprites, & &1.name)})
      {:ok, Enum.map(sprites, &%{name: &1.name, healthy: true, action: :provisioned})}
    end
  end

  defmodule MockSpriteModule do
    def status("bb-weaver-1", opts) do
      notify({:status_called, "bb-weaver-1", opts})

      {:ok,
       %{
         sprite: "bb-weaver-1",
         reachable: true,
         harness_ready: true,
         codex_auth_ready: true,
         git_ready: true,
         healthy: true,
         paused: false,
         busy: true,
         loop_alive: true,
         lifecycle_status: "running"
       }}
    end

    def status("bb-weaver-3", opts) do
      notify({:status_called, "bb-weaver-3", opts})

      {:ok,
       %{
         sprite: "bb-weaver-3",
         reachable: true,
         harness_ready: true,
         codex_auth_ready: false,
         git_ready: false,
         healthy: false,
         paused: false,
         busy: false,
         loop_alive: false,
         lifecycle_status: "idle"
       }}
    end

    def status("bb-weaver-4", opts) do
      notify({:status_called, "bb-weaver-4", opts})

      {:ok,
       %{
         sprite: "bb-weaver-4",
         reachable: true,
         harness_ready: true,
         codex_auth_ready: true,
         git_ready: true,
         healthy: true,
         paused: false,
         busy: false,
         loop_alive: true,
         lifecycle_status: "running"
       }}
    end

    def status(name, opts) do
      notify({:status_called, name, opts})

      {:ok,
       %{
         sprite: name,
         reachable: true,
         harness_ready: true,
         codex_auth_ready: true,
         git_ready: true,
         healthy: true,
         paused: false,
         busy: false,
         loop_alive: false,
         lifecycle_status: "idle"
       }}
    end

    def pause(name) do
      notify({:pause_called, name})
      :ok
    end

    def stop_loop(name) do
      notify({:stop_called, name})
      :ok
    end

    def detect_auth_failure(name) do
      notify({:detect_auth_failure_called, name})
      :ok
    end

    def resume(name) do
      notify({:resume_called, name})
      :ok
    end

    def logs(_name, _opts), do: :ok

    def force_sync_codex_auth(name) do
      notify({:force_sync_called, name})
      :ok
    end

    def provision(name, opts) do
      notify({:provision_called, name, opts})
      :ok
    end

    def exec(name, command, _opts) do
      notify({:exec_called, name, command})
      {:ok, ""}
    end

    def start_loop(name, prompt, repo, opts) do
      notify({:start_loop_called, name, prompt, repo, opts})
      {:ok, "123\n"}
    end

    defp notify(message) do
      if pid = Application.get_env(:conductor, :sprite_test_pid) do
        send(pid, message)
      end
    end
  end

  defmodule MockWorkspaceModule do
    def repo_root(repo) do
      Path.join("/tmp", repo)
    end

    def sync_persona(sprite, workspace, role) do
      if pid = Application.get_env(:conductor, :sprite_test_pid) do
        send(pid, {:sync_persona_called, sprite, workspace, role})
      end

      :ok
    end

    defdelegate persona_for_role(role), to: Conductor.Workspace
  end

  defmodule MockConfigModule do
    def check_env!(opts) do
      if pid = Application.get_env(:conductor, :sprite_test_pid) do
        send(pid, {:check_env_called, opts})
      end

      :ok
    end
  end

  defmodule MockBootstrapModule do
    def ensure_spellbook(name) do
      if pid = Application.get_env(:conductor, :sprite_test_pid) do
        send(pid, {:ensure_spellbook_called, name})
      end

      :ok
    end
  end

  setup do
    db_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.jsonl")

    fleet_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"
      default_branch = "main"

      [[sprite]]
      name = "bb-weaver-1"
      role = "builder"
      capability_tags = ["elixir"]

      [[sprite]]
      name = "bb-weaver-2"
      role = "builder"

      [[sprite]]
      name = "bb-weaver-3"
      role = "builder"
      repo = "other/other-repo"
      clone_url = "https://git.example.com/other/other-repo.git"
      default_branch = "release"

      [[sprite]]
      name = "bb-weaver-4"
      role = "builder"
      """
    )

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)
    orig_worker = Application.get_env(:conductor, :worker_module)
    orig_reconciler = Application.get_env(:conductor, :fleet_reconciler)
    orig_sprite_module = Application.get_env(:conductor, :sprite_module)
    orig_sprite_test_pid = Application.get_env(:conductor, :sprite_test_pid)
    orig_workspace_module = Application.get_env(:conductor, :workspace_module)
    orig_config_module = Application.get_env(:conductor, :config_module)
    orig_bootstrap_module = Application.get_env(:conductor, :bootstrap_module)
    orig_openai_key = System.get_env("OPENAI_API_KEY")
    orig_canary_endpoint = System.get_env("CANARY_ENDPOINT")
    orig_canary_api_key = System.get_env("CANARY_API_KEY")
    Application.stop(:conductor)
    System.delete_env("OPENAI_API_KEY")
    System.delete_env("CANARY_ENDPOINT")
    System.delete_env("CANARY_API_KEY")
    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.put_env(:conductor, :worker_module, MockWorker)
    Application.ensure_all_started(:conductor)

    on_exit(fn ->
      Application.stop(:conductor)
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)

      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)

      if orig_reconciler,
        do: Application.put_env(:conductor, :fleet_reconciler, orig_reconciler),
        else: Application.delete_env(:conductor, :fleet_reconciler)

      if orig_sprite_module,
        do: Application.put_env(:conductor, :sprite_module, orig_sprite_module),
        else: Application.delete_env(:conductor, :sprite_module)

      if orig_sprite_test_pid,
        do: Application.put_env(:conductor, :sprite_test_pid, orig_sprite_test_pid),
        else: Application.delete_env(:conductor, :sprite_test_pid)

      if orig_workspace_module,
        do: Application.put_env(:conductor, :workspace_module, orig_workspace_module),
        else: Application.delete_env(:conductor, :workspace_module)

      if orig_config_module,
        do: Application.put_env(:conductor, :config_module, orig_config_module),
        else: Application.delete_env(:conductor, :config_module)

      if orig_bootstrap_module,
        do: Application.put_env(:conductor, :bootstrap_module, orig_bootstrap_module),
        else: Application.delete_env(:conductor, :bootstrap_module)

      case orig_openai_key do
        nil -> System.delete_env("OPENAI_API_KEY")
        value -> System.put_env("OPENAI_API_KEY", value)
      end

      case orig_canary_endpoint do
        nil -> System.delete_env("CANARY_ENDPOINT")
        value -> System.put_env("CANARY_ENDPOINT", value)
      end

      case orig_canary_api_key do
        nil -> System.delete_env("CANARY_API_KEY")
        value -> System.put_env("CANARY_API_KEY", value)
      end

      Application.ensure_all_started(:conductor)

      File.rm(db_path)
      File.rm(event_log)
      File.rm(fleet_path)
    end)

    %{fleet_path: fleet_path}
  end

  defp make_failing_sprite_cli_dir do
    dir = Path.join(System.tmp_dir!(), "fake_sprite_#{System.unique_integer([:positive])}")
    File.mkdir_p!(dir)

    sprite_path = Path.join(dir, "sprite")

    File.write!(
      sprite_path,
      """
      #!/bin/sh
      printf '%s' 'no token found for organization'
      exit 1
      """
    )

    File.chmod!(sprite_path, 0o755)
    dir
  end

  test "fleet prints declared workers with health status", %{fleet_path: fleet_path} do
    output =
      capture_io(fn ->
        CLI.main(["fleet", "--fleet", fleet_path])
      end)

    assert output =~ "bb-weaver-1"
    assert output =~ "healthy"

    assert output =~ "bb-weaver-2"
    assert output =~ "unreachable"

    assert output =~ "bb-weaver-3"
    assert output =~ "needs setup"

    assert output =~ "bb-weaver-4"
    assert output =~ "needs setup"
  end

  test "fleet handles probe-only workers", %{fleet_path: fleet_path} do
    orig_worker = Application.get_env(:conductor, :worker_module)
    Application.put_env(:conductor, :worker_module, ProbeOnlyWorker)

    try do
      output =
        capture_io(fn ->
          CLI.main(["fleet", "--fleet", fleet_path])
        end)

      assert output =~ "bb-weaver-1"
      assert output =~ "healthy"
    after
      if orig_worker,
        do: Application.put_env(:conductor, :worker_module, orig_worker),
        else: Application.delete_env(:conductor, :worker_module)
    end
  end

  test "fleet audit emits json summary", %{fleet_path: fleet_path} do
    output =
      capture_io(fn ->
        CLI.main(["fleet", "audit", "--fleet", fleet_path])
      end)

    payload = Jason.decode!(output)
    assert payload["summary"]["total"] == 4
    assert payload["summary"]["reachable"] == 3
    assert length(payload["sprites"]) == 4
  end

  test "fleet audit excludes loop-owned sprites from available capacity", %{
    fleet_path: fleet_path
  } do
    Application.put_env(:conductor, :worker_module, MockSpriteModule)

    output =
      capture_io(fn ->
        CLI.main(["fleet", "audit", "--fleet", fleet_path])
      end)

    payload = Jason.decode!(output)
    assert payload["summary"]["running"] == 2
    assert payload["summary"]["available_capacity"] == 1

    assert Enum.any?(payload["sprites"], fn row ->
             row["name"] == "bb-weaver-4" and row["loop_alive"] == true and
               row["busy"] == false and row["lifecycle_status"] == "running"
           end)
  end

  test "sprite status emits json with lifecycle state", %{fleet_path: fleet_path} do
    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    output =
      capture_io(fn ->
        CLI.main(["sprite", "status", "bb-weaver-1", "--fleet", fleet_path, "--json"])
      end)

    assert_received {:status_called, "bb-weaver-1", opts}
    assert opts[:org] == "misty-step"
    assert opts[:repo] == "test/repo"
    assert opts[:clone_url] == "https://git.example.com/test/repo.git"

    payload = Jason.decode!(output)
    assert payload["name"] == "bb-weaver-1"
    assert payload["loop_alive"] == true
    assert payload["lifecycle_status"] == "running"
    assert payload["healthy"] == true
  end

  test "sprite pause --wait stops the loop after pausing", %{fleet_path: fleet_path} do
    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    output =
      capture_io(fn ->
        CLI.main(["sprite", "pause", "bb-weaver-1", "--fleet", fleet_path, "--wait"])
      end)

    assert output =~ "paused bb-weaver-1"
    assert_received {:pause_called, "bb-weaver-1"}
    assert_received {:stop_called, "bb-weaver-1"}
  end

  test "sprite start uses the fast path for a healthy declared sprite", %{
    fleet_path: fleet_path
  } do
    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :workspace_module, MockWorkspaceModule)
    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :bootstrap_module, MockBootstrapModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    output =
      capture_io(fn ->
        CLI.main(["sprite", "start", "bb-weaver-2", "--fleet", fleet_path])
      end)

    assert output =~ "started bb-weaver-2 (pid 123)"
    refute_received {:provision_called, _, _}
    assert_received {:force_sync_called, "bb-weaver-2"}
    assert_received {:ensure_spellbook_called, "bb-weaver-2"}
    assert_received {:exec_called, "bb-weaver-2", git_probe_cmd}
    assert git_probe_cmd =~ "test -d '/tmp/test/repo/.git'"
    assert_received {:exec_called, "bb-weaver-2", refresh_cmd}
    assert refresh_cmd =~ "git fetch origin --quiet"
    assert refresh_cmd =~ "git checkout -f 'origin/main'"
    assert refresh_cmd =~ "git clean -fd"
    assert_received {:sync_persona_called, "bb-weaver-2", "/tmp/test/repo", :weaver}

    assert_received {:start_loop_called, "bb-weaver-2", prompt, "test/repo", opts}
    assert prompt =~ "# Weaver Loop"
    assert opts[:workspace] == "/tmp/test/repo"
    assert opts[:persona_role] == :weaver
    assert opts[:harness] == Conductor.Codex
  end

  test "sprite start provisions an unhealthy declared sprite before launch", %{
    fleet_path: fleet_path
  } do
    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :workspace_module, MockWorkspaceModule)
    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :bootstrap_module, MockBootstrapModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    output =
      capture_io(fn ->
        CLI.main(["sprite", "start", "bb-weaver-3", "--fleet", fleet_path])
      end)

    assert output =~ "started bb-weaver-3 (pid 123)"
    assert_received {:check_env_called, opts}
    assert opts[:require_codex_auth] == true
    assert opts[:sprite_auth_probes] == [%{org: "misty-step", sprite: "bb-weaver-3"}]
    assert opts[:sprite_auth_probe_target] == "bb-weaver-3"

    assert_received {:provision_called, "bb-weaver-3",
                     [
                       repo: "other/other-repo",
                       clone_url: "https://git.example.com/other/other-repo.git",
                       default_branch: "release",
                       persona: nil,
                       persona_role: :weaver,
                       harness: "codex"
                     ]}

    assert_received {:ensure_spellbook_called, "bb-weaver-3"}
    assert_received {:exec_called, "bb-weaver-3", git_probe_cmd}
    assert git_probe_cmd =~ "test -d '/tmp/other/other-repo/.git'"
    assert_received {:exec_called, "bb-weaver-3", refresh_cmd}
    assert refresh_cmd =~ "git fetch origin --quiet"
    assert refresh_cmd =~ "git checkout -f 'origin/release'"
    assert_received {:force_sync_called, "bb-weaver-3"}
    assert_received {:sync_persona_called, "bb-weaver-3", "/tmp/other/other-repo", :weaver}
    assert_received {:start_loop_called, "bb-weaver-3", prompt, "other/other-repo", opts}
    assert prompt =~ "Repository: other/other-repo"
    assert opts[:workspace] == "/tmp/other/other-repo"
  end

  test "sprite start runs preflight for healthy responder sprites" do
    responder_fleet_path =
      Path.join(
        System.tmp_dir!(),
        "fleet_cli_responder_#{System.unique_integer([:positive])}.toml"
      )

    File.write!(
      responder_fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"

      [[sprite]]
      name = "bb-tansy"
      role = "responder"
      """
    )

    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :workspace_module, MockWorkspaceModule)
    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :bootstrap_module, MockBootstrapModule)
    Application.put_env(:conductor, :sprite_test_pid, self())
    System.put_env("CANARY_ENDPOINT", "https://canary.example.test")
    System.put_env("CANARY_API_KEY", "test-canary-key")

    output =
      capture_io(fn ->
        CLI.main(["sprite", "start", "bb-tansy", "--fleet", responder_fleet_path])
      end)

    assert output =~ "started bb-tansy (pid 123)"
    assert_received {:check_env_called, opts}
    assert opts[:require_canary_auth] == true
    assert opts[:sprite_auth_probes] == [%{org: "misty-step", sprite: "bb-tansy"}]
    assert opts[:sprite_auth_probe_target] == "bb-tansy"
    assert_received {:ensure_spellbook_called, "bb-tansy"}

    File.rm(responder_fleet_path)
  end

  test "sprite resume resumes a declared sprite", %{fleet_path: fleet_path} do
    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    output =
      capture_io(fn ->
        CLI.main(["sprite", "resume", "bb-weaver-1", "--fleet", fleet_path])
      end)

    assert output =~ "resumed bb-weaver-1"
    assert_received {:resume_called, "bb-weaver-1"}
  end

  test "sprite stop stops a declared sprite", %{fleet_path: fleet_path} do
    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    output =
      capture_io(fn ->
        CLI.main(["sprite", "stop", "bb-weaver-1", "--fleet", fleet_path])
      end)

    assert output =~ "stopped bb-weaver-1"
    assert_received {:stop_called, "bb-weaver-1"}
  end

  test "configured reconciler receives the declared fleet sprites", %{fleet_path: fleet_path} do
    {:ok, config} = Conductor.Fleet.Loader.load(fleet_path)
    {:ok, _results} = MockReconciler.reconcile_all(config.sprites)

    assert_received {:reconciled, ["bb-weaver-1", "bb-weaver-2", "bb-weaver-3", "bb-weaver-4"]}
  end

  test "fleet --reconcile forwards declared sprites to config checks", %{fleet_path: fleet_path} do
    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :fleet_reconciler, MockReconciler)
    Application.put_env(:conductor, :sprite_test_pid, self())

    capture_io(fn ->
      CLI.main(["fleet", "--fleet", fleet_path, "--reconcile"])
    end)

    assert_received {:check_env_called, opts}
    assert opts[:require_codex_auth] == true
    assert opts[:sprite_auth_probes] == [%{org: "misty-step", sprite: "bb-weaver-1"}]
    assert opts[:sprite_auth_probe_target] == "bb-weaver-1"
  end

  test "mix conductor fleet --reconcile fails with environment preflight output", %{
    fleet_path: fleet_path
  } do
    codex_home =
      Path.join(System.tmp_dir!(), "codex_home_#{System.unique_integer([:positive])}")

    File.mkdir_p!(codex_home)

    {output, status} =
      System.cmd("mix", ["conductor", "fleet", "--fleet", fleet_path, "--reconcile"],
        cd: @conductor_dir,
        env: [
          {"MIX_ENV", "test"},
          {"OPENAI_API_KEY", ""},
          {"CODEX_HOME", codex_home},
          {"SPRITE_TOKEN", "sprite-test"}
        ],
        stderr_to_stdout: true
      )

    File.rm_rf(codex_home)

    assert status == 1

    assert output =~
             "environment check failed: missing: Codex ChatGPT auth cache or OPENAI_API_KEY"

    assert output =~ "Codex ChatGPT auth cache or OPENAI_API_KEY"
  end

  test "mix conductor fleet --reconcile skips Codex auth preflight for claude-only fleets" do
    fleet_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.toml")

    sprite_dir = make_failing_sprite_cli_dir()

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"
      harness = "claude-code"

      [[sprite]]
      name = "bb-fern-1"
      role = "polisher"
      """
    )

    {output, status} =
      System.cmd("mix", ["conductor", "fleet", "--fleet", fleet_path, "--reconcile"],
        cd: @conductor_dir,
        env: [
          {"MIX_ENV", "test"},
          {"OPENAI_API_KEY", ""},
          {"PATH", sprite_dir <> ":" <> System.get_env("PATH", "")}
        ],
        stderr_to_stdout: true
      )

    File.rm_rf(fleet_path)
    File.rm_rf(sprite_dir)

    assert status == 1
    assert output =~ "environment check failed: missing: SPRITE_TOKEN or sprite CLI auth"
    refute output =~ "Codex ChatGPT auth cache or OPENAI_API_KEY"
  end

  test "mix conductor start skips Codex auth preflight for claude-only fleets" do
    fleet_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.toml")

    sprite_dir = make_failing_sprite_cli_dir()

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"
      harness = "claude-code"

      [[sprite]]
      name = "bb-fern-1"
      role = "polisher"
      """
    )

    {output, status} =
      System.cmd("mix", ["conductor", "start", "--fleet", fleet_path],
        cd: @conductor_dir,
        env: [
          {"MIX_ENV", "test"},
          {"OPENAI_API_KEY", ""},
          {"PATH", sprite_dir <> ":" <> System.get_env("PATH", "")}
        ],
        stderr_to_stdout: true
      )

    File.rm_rf(fleet_path)
    File.rm_rf(sprite_dir)

    assert status == 1
    assert output =~ "environment check failed: missing: SPRITE_TOKEN or sprite CLI auth"
    refute output =~ "Codex ChatGPT auth cache or OPENAI_API_KEY"
  end

  test "mix conductor check-env --fleet skips Codex auth preflight for claude-only fleets" do
    fleet_path =
      Path.join(System.tmp_dir!(), "fleet_cli_test_#{System.unique_integer([:positive])}.toml")

    sprite_dir = make_failing_sprite_cli_dir()

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"
      harness = "claude-code"

      [[sprite]]
      name = "bb-fern-1"
      role = "polisher"
      """
    )

    {output, status} =
      System.cmd("mix", ["conductor", "check-env", "--fleet", fleet_path],
        cd: @conductor_dir,
        env: [
          {"MIX_ENV", "test"},
          {"OPENAI_API_KEY", ""},
          {"PATH", sprite_dir <> ":" <> System.get_env("PATH", "")}
        ],
        stderr_to_stdout: true
      )

    File.rm_rf(fleet_path)
    File.rm_rf(sprite_dir)

    assert status == 1
    assert output =~ "environment check failed: missing: SPRITE_TOKEN or sprite CLI auth"
    refute output =~ "Codex ChatGPT auth cache or OPENAI_API_KEY"
  end

  test "check-env forwards declared sprites to config checks", %{fleet_path: fleet_path} do
    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    capture_io(fn ->
      CLI.main(["check-env", "--fleet", fleet_path])
    end)

    assert_received {:check_env_called, opts}
    assert opts[:require_codex_auth] == true
    assert opts[:sprite_auth_probes] == [%{org: "misty-step", sprite: "bb-weaver-1"}]
    assert opts[:sprite_auth_probe_target] == "bb-weaver-1"
  end

  test "check-env forwards one sprite auth probe per declared org" do
    fleet_path =
      Path.join(System.tmp_dir!(), "fleet_cli_orgs_#{System.unique_integer([:positive])}.toml")

    File.write!(
      fleet_path,
      """
      version = "1"

      [defaults]
      org = "alpha"
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"

      [[sprite]]
      name = "bb-alpha-1"
      role = "builder"

      [[sprite]]
      name = "bb-alpha-2"
      role = "fixer"

      [[sprite]]
      name = "bb-beta-1"
      role = "polisher"
      org = "beta"
      """
    )

    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    capture_io(fn ->
      CLI.main(["check-env", "--fleet", fleet_path])
    end)

    assert_received {:check_env_called, opts}

    assert opts[:sprite_auth_probes] == [
             %{org: "alpha", sprite: "bb-alpha-1"},
             %{org: "beta", sprite: "bb-beta-1"}
           ]

    File.rm(fleet_path)
  end

  test "check-env requires Canary auth for responder fleets" do
    responder_fleet_path =
      Path.join(
        System.tmp_dir!(),
        "fleet_cli_responder_#{System.unique_integer([:positive])}.toml"
      )

    File.write!(
      responder_fleet_path,
      """
      version = "1"

      [defaults]
      repo = "test/repo"
      clone_url = "https://git.example.com/test/repo.git"

      [[sprite]]
      name = "bb-tansy"
      role = "responder"
      """
    )

    Application.put_env(:conductor, :config_module, MockConfigModule)
    Application.put_env(:conductor, :sprite_test_pid, self())

    capture_io(fn ->
      CLI.main(["check-env", "--fleet", responder_fleet_path])
    end)

    assert_received {:check_env_called, opts}
    assert opts[:require_codex_auth] == true
    assert opts[:require_canary_auth] == true
    assert opts[:sprite_auth_probes] == [%{org: "misty-step", sprite: "bb-tansy"}]
    assert opts[:sprite_auth_probe_target] == "bb-tansy"

    File.rm(responder_fleet_path)
  end

  test "mix conductor sprite rejects unknown subcommands with exit 1" do
    {output, status} =
      System.cmd("mix", ["conductor", "sprite", "sttaus"],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "usage: bitterblossom sprite <status|start|stop|pause|resume|logs> ..."
  end

  test "mix conductor fleet rejects unknown subcommands with exit 1" do
    {output, status} =
      System.cmd("mix", ["conductor", "fleet", "sttaus"],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1

    assert output =~
             "usage: bitterblossom fleet [status|audit] [--fleet path] [--reconcile] [--json]"
  end

  test "mix conductor fleet rejects unknown flags with exit 1" do
    {output, status} =
      System.cmd("mix", ["conductor", "fleet", "--bogus"],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1

    assert output =~
             "usage: bitterblossom fleet [status|audit] [--fleet path] [--reconcile] [--json]"
  end

  test "mix conductor sprite status rejects unknown flags with exit 1" do
    {output, status} =
      System.cmd(
        "mix",
        ["conductor", "sprite", "status", "bb-builder", "--fleet", "../fleet.toml", "--bogus"],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "usage: bitterblossom sprite <command> <sprite> [--fleet path]"
  end
end
