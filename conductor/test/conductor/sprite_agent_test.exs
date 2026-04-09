defmodule Conductor.SpriteAgentTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite
  import Conductor.TestSupport.EnvHelpers

  setup do
    original_env =
      for key <-
            ~w(CODEX_HOME OPENAI_API_KEY GITHUB_TOKEN EXA_API_KEY CANARY_ENDPOINT CANARY_API_KEY),
          into: %{} do
        {key, System.get_env(key)}
      end

    codex_home = fresh_codex_home()
    System.put_env("CODEX_HOME", codex_home)
    System.delete_env("OPENAI_API_KEY")
    System.delete_env("GITHUB_TOKEN")
    System.put_env("EXA_API_KEY", "exa-test-456")
    System.delete_env("CANARY_ENDPOINT")
    System.delete_env("CANARY_API_KEY")

    on_exit(fn ->
      File.rm_rf(codex_home)
      Enum.each(original_env, fn {key, value} -> restore_env(key, value) end)
    end)

    :ok
  end

  test "status reports paused, busy, loop pid, and draining lifecycle" do
    assert {:ok, status} =
             Sprite.status("bb-weaver",
               harness: "codex",
               exec_fn: fn _sprite, command, _opts ->
                 cond do
                   command == "printf '__bb_probe__'" ->
                     {:ok, "__bb_probe__"}

                   String.contains?(command, "command -v codex") ->
                     {:ok, "/usr/bin/codex\n"}

                   String.contains?(command, "test -s '/home/sprite/.codex/auth.json'") ->
                     {:ok, ""}

                   String.contains?(command, "gh auth status") ->
                     {:ok, "github.com\n"}

                   String.contains?(command, "git config --global --get credential.helper") ->
                     {:ok, "!gh auth git-credential"}

                   String.contains?(command, "printf 'paused'") ->
                     {:ok, "paused"}

                   String.contains?(command, "pgrep -x codex") ->
                     {:ok, "4242\n"}

                   String.contains?(command, "kill -0 \"$pid\"") ->
                     {:ok, "4242"}

                   true ->
                     {:ok, ""}
                 end
               end
             )

    assert status.paused == true
    assert status.busy == true
    assert status.loop_pid == 4242
    assert status.lifecycle_status == "draining"
  end

  test "start_loop uploads prompt/env and launches a detached loop wrapper" do
    test_pid = self()
    System.put_env("OPENAI_API_KEY", "sk-test-123")

    exec_fn = fn _sprite, command, opts ->
      uploaded_files =
        opts
        |> Keyword.get(:files, [])
        |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

      send(test_pid, {:exec_called, command, opts, uploaded_files})

      cond do
        String.contains?(command, "setsid bash -lc") ->
          {:ok, "__bb_started__:123\n"}

        true ->
          {:ok, ""}
      end
    end

    assert {:ok, "123\n"} =
             Sprite.start_loop("bb-weaver", "# Loop prompt", "test/repo",
               workspace: "/tmp/worktree",
               persona_role: :weaver,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    assert_received {:exec_called, "true", upload_opts, uploaded_files}
    assert Keyword.has_key?(upload_opts, :files)
    assert {"/tmp/worktree/PROMPT.md", "# Loop prompt"} in uploaded_files

    assert Enum.any?(uploaded_files, fn
             {"/tmp/worktree/.bb-runtime-env", content} ->
               exa_index = :binary.match(content, "export EXA_API_KEY='exa-test-456'")
               repo_index = :binary.match(content, "export REPO='test/repo'")

               String.contains?(content, "export OPENAI_API_KEY='sk-test-123'") and
                 String.contains?(content, "export CODEX_API_KEY='sk-test-123'") and
                 not String.contains?(content, "export CANARY_ENDPOINT=") and
                 not String.contains?(content, "export CANARY_API_KEY=") and
                 match?({_, _}, exa_index) and match?({_, _}, repo_index) and
                 elem(exa_index, 0) < elem(repo_index, 0)

             _ ->
               false
           end)

    assert_received {:exec_called, detached_cmd, _, _}
    assert detached_cmd =~ "flock -n 9"
    assert detached_cmd =~ "__bb_started__:"
    assert detached_cmd =~ "__bb_busy__"
    assert detached_cmd =~ "__bb_paused__"
    assert detached_cmd =~ "setsid bash -lc"
    assert detached_cmd =~ "/home/sprite/.bitterblossom/loop.pid"
    assert detached_cmd =~ "codex exec"
  end

  test "start_loop injects Canary credentials only for the responder persona" do
    test_pid = self()
    System.put_env("CANARY_ENDPOINT", "https://canary-obs.fly.dev")
    System.put_env("CANARY_API_KEY", "canary-test-123")

    exec_fn = fn _sprite, command, opts ->
      uploaded_files =
        opts
        |> Keyword.get(:files, [])
        |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

      send(test_pid, {:exec_called, command, uploaded_files})

      if String.contains?(command, "setsid bash -lc") do
        {:ok, "__bb_started__:123\n"}
      else
        {:ok, ""}
      end
    end

    assert {:ok, "123\n"} =
             Sprite.start_loop("bb-tansy", "# Loop prompt", "misty-step/bitterblossom",
               workspace: "/tmp/worktree",
               persona_role: :tansy,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    assert_received {:exec_called, "true", uploaded_files}

    assert Enum.any?(uploaded_files, fn
             {"/tmp/worktree/.bb-runtime-env", content} ->
               String.contains?(content, "export CANARY_ENDPOINT='https://canary-obs.fly.dev'") and
                 String.contains?(content, "export CANARY_API_KEY='canary-test-123'")

             _ ->
               false
           end)
  end

  test "start_loop refuses to launch when the sprite is paused" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})
      {:ok, "__bb_paused__"}
    end

    assert {:error, "sprite is paused", 1} =
             Sprite.start_loop("bb-weaver", "# Loop prompt", "test/repo",
               workspace: "/tmp/worktree",
               persona_role: :weaver,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    assert_received {:exec_called, "true"}
    assert_received {:exec_called, detached_cmd}
    assert detached_cmd =~ "flock -n 9"
  end

  test "start_loop refuses to launch when another loop is active" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})
      if command == "true", do: {:ok, ""}, else: {:ok, "__bb_busy__"}
    end

    assert {:error, "sprite already has an active loop", 1} =
             Sprite.start_loop("bb-weaver", "# Loop prompt", "test/repo",
               workspace: "/tmp/worktree",
               persona_role: :weaver,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    assert_received {:exec_called, "true"}
    assert_received {:exec_called, detached_cmd}
    assert detached_cmd =~ "flock -n 9"
  end

  test "pause, resume, and stop_loop use the runtime markers" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})
      {:ok, ""}
    end

    assert :ok = Sprite.pause("bb-weaver", exec_fn: exec_fn)
    assert_received {:exec_called, pause_cmd}
    assert pause_cmd =~ "touch '/home/sprite/.bitterblossom/paused'"

    assert :ok = Sprite.resume("bb-weaver", exec_fn: exec_fn)
    assert_received {:exec_called, resume_cmd}
    assert resume_cmd == "rm -f '/home/sprite/.bitterblossom/paused'"

    assert :ok = Sprite.stop_loop("bb-weaver", exec_fn: exec_fn)
    assert_received {:exec_called, stop_cmd}
    assert stop_cmd =~ "/home/sprite/.bitterblossom/loop.pid"
    assert stop_cmd =~ "kill -- -"
    assert stop_cmd =~ "pkill -9 -f codex"
  end
end

defmodule Conductor.SpriteRetryLoopTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite
  import Conductor.TestSupport.EnvHelpers

  setup do
    original_env =
      for key <-
            ~w(CODEX_HOME OPENAI_API_KEY GITHUB_TOKEN EXA_API_KEY CANARY_ENDPOINT CANARY_API_KEY),
          into: %{} do
        {key, System.get_env(key)}
      end

    codex_home = fresh_codex_home()
    System.put_env("CODEX_HOME", codex_home)
    System.put_env("OPENAI_API_KEY", "sk-test-123")
    System.delete_env("GITHUB_TOKEN")
    System.put_env("EXA_API_KEY", "exa-test-456")
    System.delete_env("CANARY_ENDPOINT")
    System.delete_env("CANARY_API_KEY")

    on_exit(fn ->
      File.rm_rf(codex_home)
      Enum.each(original_env, fn {key, value} -> restore_env(key, value) end)
    end)

    :ok
  end

  defp start_loop_and_get_detached_cmd do
    test_pid = self()

    exec_fn = fn _sprite, command, opts ->
      uploaded_files =
        opts
        |> Keyword.get(:files, [])
        |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

      send(test_pid, {:exec_called, command, opts, uploaded_files})

      cond do
        String.contains?(command, "setsid bash -lc") ->
          {:ok, "__bb_started__:123\n"}

        true ->
          {:ok, ""}
      end
    end

    assert {:ok, "123\n"} =
             Sprite.start_loop("bb-weaver", "# Loop prompt", "test/repo",
               workspace: "/tmp/worktree",
               persona_role: :weaver,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    # Drain the upload call
    assert_received {:exec_called, "true", _, _}
    assert_received {:exec_called, detached_cmd, _, _}

    detached_cmd
  end

  test "detached agent command wraps agent_cmd in a retry loop" do
    detached_cmd = start_loop_and_get_detached_cmd()

    # The setsid block must contain a retry loop around the agent command
    assert detached_cmd =~ "for attempt in 1 2 3"
    assert detached_cmd =~ "exit_code=$?"
    assert detached_cmd =~ ~r/if \[ .*exit_code.* -eq 0 \]/
    assert detached_cmd =~ "sleep 10"
  end

  test "retry loop preserves PID file and EXIT trap in correct order" do
    detached_cmd = start_loop_and_get_detached_cmd()

    # PID file write and EXIT trap must still be present
    assert detached_cmd =~ "echo $$ > /home/sprite/.bitterblossom/loop.pid"
    assert detached_cmd =~ ~r|trap .+rm -f .+loop\.pid|

    # The retry loop must be inside the setsid block (after the trap)
    setsid_start = :binary.match(detached_cmd, "setsid bash -lc") |> elem(0)
    pid_write = :binary.match(detached_cmd, "echo $$ >") |> elem(0)
    trap_pos = :binary.match(detached_cmd, "trap ") |> elem(0)
    retry_pos = :binary.match(detached_cmd, "for attempt in") |> elem(0)

    assert pid_write > setsid_start, "PID write should be inside setsid block"
    assert trap_pos > pid_write, "EXIT trap should be after PID write"
    assert retry_pos > trap_pos, "retry loop should be after EXIT trap"
  end
end
