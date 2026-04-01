defmodule Conductor.SpriteTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite
  import Conductor.TestSupport.EnvHelpers

  setup do
    original_env =
      for key <- ~w(CODEX_HOME OPENAI_API_KEY GITHUB_TOKEN), into: %{} do
        {key, System.get_env(key)}
      end

    codex_home = fresh_codex_home()
    System.put_env("CODEX_HOME", codex_home)
    System.delete_env("OPENAI_API_KEY")
    System.delete_env("GITHUB_TOKEN")

    on_exit(fn ->
      File.rm_rf(codex_home)
      Enum.each(original_env, fn {key, value} -> restore_env(key, value) end)
    end)

    :ok
  end

  defp exec_fn(responses) do
    fn _sprite, command, _opts ->
      Enum.find_value(responses, {:ok, ""}, fn {pattern, result} ->
        if String.contains?(command, pattern), do: result
      end)
    end
  end

  defp drain_exec_calls(acc \\ []) do
    receive do
      {:exec_called, command, opts, uploaded_files} ->
        drain_exec_calls([{command, opts, uploaded_files} | acc])
    after
      0 -> Enum.reverse(acc)
    end
  end

  defp uploads_to?(opts, dest) do
    opts
    |> Keyword.get(:files, [])
    |> Enum.any?(fn {_src, uploaded_dest} -> uploaded_dest == dest end)
  end

  test "status reports gh auth, Codex auth, and harness readiness" do
    System.put_env("OPENAI_API_KEY", "sk-test")

    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        exec_fn:
          exec_fn([
            {"__bb_probe__", {:ok, "__bb_probe__"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              codex_auth_ready: true,
              gh_authenticated: true,
              git_credential_helper: true,
              healthy: true
            }} = status
  end

  test "status marks missing gh auth as unhealthy" do
    System.put_env("OPENAI_API_KEY", "sk-test")

    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        exec_fn:
          exec_fn([
            {"__bb_probe__", {:ok, "__bb_probe__"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:error, "not logged in", 1}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              codex_auth_ready: true,
              gh_authenticated: false,
              git_credential_helper: true,
              healthy: false
            }} = status
  end

  test "status marks missing git credential helper as unhealthy" do
    System.put_env("OPENAI_API_KEY", "sk-test")

    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        exec_fn:
          exec_fn([
            {"__bb_probe__", {:ok, "__bb_probe__"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "cache"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              codex_auth_ready: true,
              gh_authenticated: true,
              git_credential_helper: false,
              healthy: false
            }} = status
  end

  test "status marks missing Codex auth as unhealthy" do
    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        exec_fn:
          exec_fn([
            {"__bb_probe__", {:ok, "__bb_probe__"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"test -s '/home/sprite/.codex/auth.json'", {:error, "", 1}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              codex_auth_ready: false,
              gh_authenticated: true,
              git_credential_helper: true,
              healthy: false
            }} = status
  end

  test "status treats an implicit Codex harness as missing auth when the remote cache is absent" do
    status =
      Sprite.status("bb-weaver",
        harness: nil,
        exec_fn:
          exec_fn([
            {"echo ok", {:ok, "ok\n"}},
            {"test -s '/home/sprite/.codex/auth.json'", {:error, "", 1}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              codex_auth_ready: false,
              gh_authenticated: true,
              git_credential_helper: true,
              healthy: false
            }} = status
  end

  test "status treats an existing remote auth cache as healthy without API key fallback" do
    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        exec_fn:
          exec_fn([
            {"__bb_probe__", {:ok, "__bb_probe__"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"test -s '/home/sprite/.codex/auth.json'", {:ok, ""}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              codex_auth_ready: true,
              gh_authenticated: true,
              git_credential_helper: true,
              healthy: true
            }} = status
  end

  describe "exec_args/3 argument construction" do
    test "includes -- separator before bash to prevent flag parsing" do
      args = Sprite.exec_args("my-org", "my-sprite", "echo hello")
      assert "--" in args

      # "--" must appear before "bash" — otherwise sprite CLI parses "-lc" as its own flag
      separator_index = Enum.find_index(args, &(&1 == "--"))
      bash_index = Enum.find_index(args, &(&1 == "bash"))
      assert separator_index < bash_index
    end

    test "passes org and sprite name as sprite CLI flags" do
      args = Sprite.exec_args("my-org", "bb-weaver", "ls")
      assert ["-o", "my-org", "-s", "bb-weaver" | _] = args
    end

    test "passes command as bash -lc argument" do
      args = Sprite.exec_args("org", "sprite", "cd /ws && mix test")
      assert List.last(args) == "cd /ws && mix test"
      assert Enum.at(args, -2) == "-lc"
    end

    test "supports HTTP POST transport for wake-safe execs" do
      args = Sprite.exec_args("org", "sprite", [], "echo hello", transport: :http_post)
      assert "--http-post" in args
    end
  end

  test "status returns error when sprite is unreachable" do
    assert {:error, "timeout"} =
             Sprite.status("bb-weaver",
               harness: "codex",
               exec_fn: fn _sprite, _command, _opts -> {:error, "timeout", 255} end
             )
  end

  test "probe treats a missing HTTP exit frame as reachable when the marker was printed" do
    assert {:ok, %{reachable: true}} =
             Sprite.probe("bb-weaver",
               exec_fn: fn _sprite, command, _opts ->
                 assert command == "printf '__bb_probe__'"
                 {:error, "__bb_probe__\nError: no exit frame received", 1}
               end
             )
  end

  test "wake treats a missing HTTP exit frame as success when the marker was printed" do
    shell_cmd_fn = fn "sprite", _args, _opts ->
      {:error, "__bb_wake__\nError: no exit frame received", 1}
    end

    assert :ok =
             Sprite.wake("bb-weaver",
               org: "misty-step",
               shell_cmd_fn: shell_cmd_fn
             )
  end

  test "exec wakes and retries via websocket after handshake failure" do
    parent = self()
    call_count = :counters.new(1, [:atomics])

    shell_cmd_fn = fn "sprite", args, opts ->
      :counters.add(call_count, 1, 1)
      count = :counters.get(call_count, 1)
      send(parent, {:shell_cmd, args, opts})

      cond do
        # Wake call (printf marker)
        List.last(args) == "printf '__bb_wake__'" ->
          {:ok, ""}

        # First exec fails (cold sprite 502)
        count == 1 ->
          {:error, "websocket: bad handshake (HTTP 502)", 1}

        # Retry succeeds after wake
        true ->
          {:ok, "recovered"}
      end
    end

    assert {:ok, "recovered"} =
             Sprite.exec("bb-weaver", "echo ok",
               org: "misty-step",
               shell_cmd_fn: shell_cmd_fn
             )

    assert_received {:shell_cmd, first_args, _opts}
    refute "--http-post" in first_args
    assert List.last(first_args) == "echo ok"

    assert_received {:shell_cmd, wake_args, _opts}
    refute "--http-post" in wake_args
    assert List.last(wake_args) == "printf '__bb_wake__'"

    assert_received {:shell_cmd, retry_args, _opts}
    refute "--http-post" in retry_args
    assert List.last(retry_args) == "echo ok"
  end

  test "exec returns non-recoverable websocket errors without attempting wake" do
    parent = self()

    shell_cmd_fn = fn "sprite", args, opts ->
      send(parent, {:shell_cmd, args, opts})
      {:error, "permission denied", 1}
    end

    assert {:error, "permission denied", 1} =
             Sprite.exec("bb-weaver", "echo ok",
               org: "misty-step",
               shell_cmd_fn: shell_cmd_fn
             )

    assert_received {:shell_cmd, first_args, _opts}
    refute "--http-post" in first_args
    assert List.last(first_args) == "echo ok"
    refute_received {:shell_cmd, _, _}
  end

  test "provision uploads persona, settings, and metadata through sprite exec files" do
    test_pid = self()
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")

    try do
      exec_fn = fn _sprite, command, opts ->
        uploaded_files =
          opts
          |> Keyword.get(:files, [])
          |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

        send(test_pid, {:exec_called, command, opts, uploaded_files})
        {:ok, ""}
      end

      assert :ok =
               Sprite.provision("bb-weaver",
                 repo: "misty-step/bitterblossom",
                 persona: "You are Weaver.",
                 force: true,
                 exec_fn: exec_fn
               )

      calls = drain_exec_calls()
      [{mkdir_cmd, _mkdir_opts, _mkdir_files} | _] = calls
      assert mkdir_cmd =~ "mkdir -p"

      {_, upload_opts, uploaded_files} =
        Enum.find(calls, fn {_command, _opts, uploaded_files} ->
          {"/home/sprite/workspace/PERSONA.md", "You are Weaver.\n"} in uploaded_files
        end)

      assert Keyword.has_key?(upload_opts, :files)
      assert {"/home/sprite/workspace/PERSONA.md", "You are Weaver.\n"} in uploaded_files

      assert Enum.any?(uploaded_files, fn
               {"/home/sprite/.claude/settings.json", content} ->
                 String.contains?(content, "\"model\"")

               _ ->
                 false
             end)

      {codex_cmd, _codex_opts, _codex_files} =
        Enum.find(calls, fn {command, _opts, _files} ->
          String.contains?(command, "@openai/codex")
        end)

      assert codex_cmd =~ "@openai/codex"

      {git_auth_cmd, git_auth_opts, git_auth_files} =
        Enum.find(calls, fn {command, _opts, _files} ->
          String.contains?(command, "gh auth login --with-token")
        end)

      assert git_auth_cmd =~ "gh auth login --with-token"
      assert Keyword.has_key?(git_auth_opts, :files)

      assert Enum.any?(git_auth_files, fn
               {dest, content} ->
                 String.starts_with?(dest, "/tmp/bb-gh-token-") and content == "ghp-test-token\n"

               _ ->
                 false
             end)

      {repo_cmd, _repo_opts, _repo_files} =
        Enum.find(calls, fn {command, _opts, _files} ->
          String.contains?(command, "git clone 'https://github.com/misty-step/bitterblossom.git'")
        end)

      assert repo_cmd =~ "git clone 'https://github.com/misty-step/bitterblossom.git'"
      assert repo_cmd =~ "/home/sprite/workspace/misty-step/bitterblossom"

      {_, _runtime_env_opts, runtime_env_files} =
        Enum.find(calls, fn {_command, _opts, uploaded_files} ->
          Enum.any?(uploaded_files, fn {dest, _content} ->
            dest == "/home/sprite/.bitterblossom/runtime.env"
          end)
        end)

      assert Enum.any?(runtime_env_files, fn
               {"/home/sprite/.bitterblossom/runtime.env", content} ->
                 String.contains?(content, "export REPO='misty-step/bitterblossom'")

               _ ->
                 false
             end)

      {_, _metadata_opts, metadata_files} =
        Enum.find(calls, fn {_command, _opts, uploaded_files} ->
          Enum.any?(uploaded_files, fn {dest, _content} ->
            dest == "/home/sprite/workspace/misty-step/bitterblossom/.bb/workspace.json"
          end)
        end)

      assert Enum.any?(metadata_files, fn
               {"/home/sprite/workspace/misty-step/bitterblossom/.bb/workspace.json", content} ->
                 String.contains?(content, "\"repo\":\"misty-step/bitterblossom\"")

               _ ->
                 false
             end)
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "provision uploads Codex auth.json when local ChatGPT auth is available and remote auth is missing" do
    test_pid = self()
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")
    write_auth_json(%{"auth_mode" => "chatgpt", "tokens" => %{"refresh_token" => "rt-test"}})

    try do
      exec_fn = fn _sprite, command, opts ->
        uploaded_files =
          opts
          |> Keyword.get(:files, [])
          |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

        send(test_pid, {:exec_called, command, opts, uploaded_files})

        case command do
          "test -s '/home/sprite/.codex/auth.json'" -> {:error, "", 1}
          _ -> {:ok, ""}
        end
      end

      assert :ok =
               Sprite.provision("bb-weaver",
                 repo: "misty-step/bitterblossom",
                 persona: "You are Weaver.",
                 force: true,
                 exec_fn: exec_fn
               )

      calls = drain_exec_calls()

      {auth_cmd, _auth_opts, auth_files} =
        Enum.find(calls, fn {_command, _opts, uploaded_files} ->
          Enum.any?(uploaded_files, fn {dest, _content} ->
            dest == "/home/sprite/.codex/auth.json"
          end)
        end)

      assert auth_cmd == "chmod 600 '/home/sprite/.codex/auth.json'"

      assert Enum.any?(auth_files, fn
               {"/home/sprite/.codex/auth.json", content} ->
                 String.contains?(content, "\"auth_mode\":\"chatgpt\"") and
                   String.contains?(content, "\"refresh_token\":\"rt-test\"")

               _ ->
                 false
             end)
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "provision preserves an existing remote Codex auth cache" do
    test_pid = self()
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")
    write_auth_json(%{"auth_mode" => "chatgpt", "tokens" => %{"refresh_token" => "rt-test"}})

    try do
      exec_fn = fn _sprite, command, opts ->
        uploaded_files =
          opts
          |> Keyword.get(:files, [])
          |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

        send(test_pid, {:exec_called, command, opts, uploaded_files})

        case command do
          "test -s '/home/sprite/.codex/auth.json'" -> {:ok, ""}
          _ -> {:ok, ""}
        end
      end

      assert :ok =
               Sprite.provision("bb-weaver",
                 repo: "misty-step/bitterblossom",
                 persona: "You are Weaver.",
                 force: true,
                 exec_fn: exec_fn
               )

      calls = drain_exec_calls()

      refute Enum.any?(calls, fn {_command, _opts, uploaded_files} ->
               Enum.any?(uploaded_files, fn {dest, _content} ->
                 dest == "/home/sprite/.codex/auth.json"
               end)
             end)
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "provision skips Codex auth sync on non-codex harnesses" do
    test_pid = self()
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")
    write_auth_json(%{"auth_mode" => "chatgpt", "tokens" => %{"refresh_token" => "rt-test"}})

    try do
      exec_fn = fn _sprite, command, opts ->
        uploaded_files =
          opts
          |> Keyword.get(:files, [])
          |> Enum.map(fn {src, dest} -> {dest, File.read!(src)} end)

        send(test_pid, {:exec_called, command, opts, uploaded_files})
        {:ok, ""}
      end

      assert :ok =
               Sprite.provision("bb-weaver",
                 repo: "misty-step/bitterblossom",
                 persona: "You are Weaver.",
                 harness: "claude-code",
                 force: true,
                 exec_fn: exec_fn
               )

      calls = drain_exec_calls()

      refute Enum.any?(calls, fn {command, _opts, uploaded_files} ->
               command == "test -s '/home/sprite/.codex/auth.json'" or
                 Enum.any?(uploaded_files, fn {dest, _content} ->
                   dest == "/home/sprite/.codex/auth.json"
                 end)
             end)
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "provision returns the chmod failure when syncing Codex auth" do
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")
    write_auth_json(%{"auth_mode" => "chatgpt", "tokens" => %{"refresh_token" => "rt-test"}})

    try do
      exec_fn = fn _sprite, command, _opts ->
        case command do
          "test -s '/home/sprite/.codex/auth.json'" -> {:error, "", 1}
          "chmod 600 '/home/sprite/.codex/auth.json'" -> {:error, "Permission denied", 1}
          _ -> {:ok, ""}
        end
      end

      assert {:error, "Permission denied"} =
               Sprite.provision("bb-weaver",
                 repo: "misty-step/bitterblossom",
                 persona: "You are Weaver.",
                 force: true,
                 exec_fn: exec_fn
               )
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "provision propagates failures from each setup step" do
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")

    try do
      cases = [
        {"remote dir creation", "mkdir failed",
         fn command, _opts -> String.contains?(command, "mkdir -p") end},
        {"base config upload", "upload failed",
         fn command, opts ->
           command == "true" and uploads_to?(opts, "/home/sprite/workspace/PERSONA.md")
         end},
        {"codex install", "codex failed",
         fn command, _opts -> String.contains?(command, "@openai/codex") end},
        {"runtime env upload", "runtime env failed",
         fn command, opts ->
           command == "true" and uploads_to?(opts, "/home/sprite/.bitterblossom/runtime.env")
         end},
        {"git auth", "git auth failed",
         fn command, _opts -> String.contains?(command, "gh auth login --with-token") end},
        {"repo setup", "repo setup failed",
         fn command, _opts ->
           String.contains?(
             command,
             "git clone 'https://github.com/misty-step/bitterblossom.git'"
           )
         end},
        {"workspace metadata upload", "metadata upload failed",
         fn command, opts ->
           command == "true" and
             uploads_to?(
               opts,
               "/home/sprite/workspace/misty-step/bitterblossom/.bb/workspace.json"
             )
         end}
      ]

      Enum.each(cases, fn {stage, reason, matcher} ->
        result =
          Sprite.provision("bb-weaver",
            repo: "misty-step/bitterblossom",
            persona: "You are Weaver.",
            force: true,
            exec_fn: fn _sprite, command, opts ->
              if matcher.(command, opts), do: {:error, reason, 1}, else: {:ok, ""}
            end
          )

        assert result == {:error, reason},
               "expected #{stage} failure to propagate, got: #{inspect(result)}"
      end)
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "provision rejects invalid repo formats before clone commands" do
    test_pid = self()
    prev_gh = System.get_env("GITHUB_TOKEN")
    System.put_env("GITHUB_TOKEN", "ghp-test-token")

    try do
      exec_fn = fn _sprite, command, _opts ->
        send(test_pid, {:exec_called, command})
        {:ok, ""}
      end

      assert {:error, reason} =
               Sprite.provision("bb-weaver",
                 repo: "bad repo;",
                 persona: "You are Weaver.",
                 force: true,
                 exec_fn: exec_fn
               )

      assert reason =~ "invalid repo format"

      calls = drain_exec_calls()

      refute Enum.any?(calls, fn {command, _opts, _files} ->
               String.contains?(command, "git clone")
             end)
    after
      if prev_gh,
        do: System.put_env("GITHUB_TOKEN", prev_gh),
        else: System.delete_env("GITHUB_TOKEN")
    end
  end

  test "logs tails the workspace log file" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "test -s '/tmp/worktree/ralph.log'") -> {:ok, ""}
        true -> {:ok, "/tmp/worktree\n"}
      end
    end

    runner_fn = fn _sprite, command, _opts ->
      send(test_pid, {:runner_called, command})
      {:ok, ""}
    end

    assert :ok =
             Sprite.logs("bb-weaver",
               workspace: "/tmp/worktree",
               lines: 25,
               exec_fn: exec_fn,
               runner_fn: runner_fn
             )

    assert_received {:exec_called, "test -s '/tmp/worktree/ralph.log'"}

    assert_received {:runner_called,
                     "touch '/tmp/worktree/ralph.log' && tail -n 25 '/tmp/worktree/ralph.log'"}
  end

  test "logs follows the workspace log file with the default tail window" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "test -s '/tmp/worktree/ralph.log'") -> {:ok, ""}
        true -> {:error, "", 1}
      end
    end

    runner_fn = fn _sprite, command, _opts ->
      send(test_pid, {:runner_called, command})
      {:ok, ""}
    end

    assert :ok =
             Sprite.logs("bb-weaver",
               workspace: "/tmp/worktree",
               follow: true,
               exec_fn: exec_fn,
               runner_fn: runner_fn
             )

    assert_received {:runner_called,
                     "touch '/tmp/worktree/ralph.log' && tail -n 50 -f '/tmp/worktree/ralph.log'"}
  end

  test "logs follows the workspace log file with an explicit line window" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "test -s '/tmp/worktree/ralph.log'") -> {:ok, ""}
        true -> {:error, "", 1}
      end
    end

    runner_fn = fn _sprite, command, _opts ->
      send(test_pid, {:runner_called, command})
      {:ok, ""}
    end

    assert :ok =
             Sprite.logs("bb-weaver",
               workspace: "/tmp/worktree",
               follow: true,
               lines: 10,
               exec_fn: exec_fn,
               runner_fn: runner_fn
             )

    assert_received {:runner_called,
                     "touch '/tmp/worktree/ralph.log' && tail -n 10 -f '/tmp/worktree/ralph.log'"}
  end

  test "logs cats the whole workspace log file when no flags are given" do
    test_pid = self()

    exec_fn = fn _sprite, command, _opts ->
      send(test_pid, {:exec_called, command})

      cond do
        String.contains?(command, "test -s '/tmp/worktree/ralph.log'") -> {:ok, ""}
        true -> {:error, "", 1}
      end
    end

    runner_fn = fn _sprite, command, _opts ->
      send(test_pid, {:runner_called, command})
      {:ok, ""}
    end

    assert :ok =
             Sprite.logs("bb-weaver",
               workspace: "/tmp/worktree",
               exec_fn: exec_fn,
               runner_fn: runner_fn
             )

    assert_received {:runner_called,
                     "touch '/tmp/worktree/ralph.log' && cat '/tmp/worktree/ralph.log'"}
  end

  test "logs returns the idle message when no task is active and the log is empty" do
    exec_fn = fn _sprite, command, _opts ->
      cond do
        String.contains?(command, "test -s") -> {:error, "", 1}
        String.contains?(command, "pgrep") -> {:error, "", 1}
        true -> {:ok, ""}
      end
    end

    assert {:error, reason} =
             Sprite.logs("bb-weaver", workspace: "/tmp/worktree", exec_fn: exec_fn)

    assert reason =~ ~s(No active task on "bb-weaver".)
    assert reason =~ "dispatch log is empty"
  end

  test "logs preserves transport failures from log availability checks" do
    exec_fn = fn _sprite, command, _opts ->
      cond do
        String.contains?(command, "test -s") -> {:error, "connection refused", 255}
        String.contains?(command, "pgrep") -> {:error, "", 1}
        true -> {:ok, ""}
      end
    end

    assert {:error, "connection refused"} =
             Sprite.logs("bb-weaver", workspace: "/tmp/worktree", exec_fn: exec_fn)
  end
end
