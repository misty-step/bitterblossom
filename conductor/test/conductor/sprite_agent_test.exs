defmodule Conductor.SpriteAgentTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite
  import Conductor.TestSupport.EnvHelpers

  setup do
    original_env =
      for key <- ~w(CODEX_HOME OPENAI_API_KEY GITHUB_TOKEN EXA_API_KEY), into: %{} do
        {key, System.get_env(key)}
      end

    codex_home = fresh_codex_home()
    System.put_env("CODEX_HOME", codex_home)
    System.delete_env("OPENAI_API_KEY")
    System.delete_env("GITHUB_TOKEN")
    System.put_env("EXA_API_KEY", "exa-test-456")

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

    exec_fn = fn _sprite, command, opts ->
      uploaded_files =
        opts
        |> Keyword.get(:files, [])
        |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

      send(test_pid, {:exec_called, command, opts, uploaded_files})

      cond do
        String.contains?(command, "printf 'paused'") ->
          {:ok, ""}

        String.contains?(command, "pgrep -x codex") ->
          {:error, "", 1}

        String.contains?(command, "nohup bash -lc") ->
          {:ok, "123\n"}

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

    assert_received {:exec_called,
                     "if [ -e '/home/sprite/.bitterblossom/paused' ]; then printf 'paused'; fi",
                     _, _}

    assert_received {:exec_called, detect_cmd, _, _}
    assert detect_cmd =~ "pgrep -x codex"

    assert_received {:exec_called, "true", upload_opts, uploaded_files}
    assert Keyword.has_key?(upload_opts, :files)
    assert {"/tmp/worktree/PROMPT.md", "# Loop prompt"} in uploaded_files

    assert Enum.any?(uploaded_files, fn
             {"/tmp/worktree/.bb-runtime-env", content} ->
               String.contains?(content, "export EXA_API_KEY='exa-test-456'")

             _ ->
               false
           end)

    assert_received {:exec_called, detached_cmd, _, _}
    assert detached_cmd =~ "nohup bash -lc"
    assert detached_cmd =~ "/home/sprite/.bitterblossom/loop.pid"
    assert detached_cmd =~ "codex exec"
  end

  test "start_loop refuses to launch when the sprite is paused" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      if String.contains?(command, "printf 'paused'") do
        {:ok, "paused"}
      else
        {:ok, ""}
      end
    end

    assert {:error, "sprite is paused", 1} =
             Sprite.start_loop("bb-weaver", "# Loop prompt", "test/repo",
               workspace: "/tmp/worktree",
               persona_role: :weaver,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    assert_received {:exec_called,
                     "if [ -e '/home/sprite/.bitterblossom/paused' ]; then printf 'paused'; fi"}

    refute_received {:exec_called, "true"}
  end

  test "start_loop refuses to launch when another loop is active" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "printf 'paused'") ->
          {:ok, ""}

        String.contains?(command, "pgrep -x codex") ->
          {:ok, "4242\n"}

        true ->
          {:ok, ""}
      end
    end

    assert {:error, "sprite already has an active loop", 1} =
             Sprite.start_loop("bb-weaver", "# Loop prompt", "test/repo",
               workspace: "/tmp/worktree",
               persona_role: :weaver,
               harness: Conductor.Codex,
               exec_fn: exec_fn
             )

    assert_received {:exec_called,
                     "if [ -e '/home/sprite/.bitterblossom/paused' ]; then printf 'paused'; fi"}

    assert_received {:exec_called, detect_cmd}
    assert detect_cmd =~ "pgrep -x codex"
    refute_received {:exec_called, "true"}
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
    assert stop_cmd =~ "pkill -9 -f codex"
  end
end
