defmodule Conductor.SpriteTest do
  use ExUnit.Case, async: false

  alias Conductor.Sprite

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

  defp drain_shell_calls(acc \\ []) do
    receive do
      {:shell_called, args, opts} ->
        drain_shell_calls([{args, opts} | acc])
    after
      0 -> Enum.reverse(acc)
    end
  end

  defp uploads_to?(opts, dest) do
    opts
    |> Keyword.get(:files, [])
    |> Enum.any?(fn {_src, uploaded_dest} -> uploaded_dest == dest end)
  end

  test "status reports gh auth and harness readiness" do
    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        state_fn: fn _, _ -> :warm end,
        exec_fn:
          exec_fn([
            {"echo ok", {:ok, "ok\n"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              gh_authenticated: true,
              git_credential_helper: true,
              healthy: true
            }} = status
  end

  test "status marks missing gh auth as unhealthy" do
    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        state_fn: fn _, _ -> :warm end,
        exec_fn:
          exec_fn([
            {"echo ok", {:ok, "ok\n"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:error, "not logged in", 1}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              gh_authenticated: false,
              git_credential_helper: true,
              healthy: false
            }} = status
  end

  test "status marks missing git credential helper as unhealthy" do
    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        state_fn: fn _, _ -> :warm end,
        exec_fn:
          exec_fn([
            {"echo ok", {:ok, "ok\n"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "cache"}}
          ])
      )

    assert {:ok,
            %{
              reachable: true,
              harness_ready: true,
              gh_authenticated: true,
              git_credential_helper: false,
              healthy: false
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
  end

  test "status returns error when sprite is unreachable" do
    assert {:error, "timeout"} =
             Sprite.status("bb-weaver",
               harness: "codex",
               state_fn: fn _, _ -> :warm end,
               exec_fn: fn _sprite, _command, _opts -> {:error, "timeout", 255} end
             )
  end

  describe "gc_checkpoints/2" do
    test "prunes oldest checkpoints and keeps the newest configured count" do
      test_pid = self()
      original = Application.get_env(:conductor, :max_checkpoints_per_sprite)
      Application.put_env(:conductor, :max_checkpoints_per_sprite, 2)

      checkpoints_json =
        Jason.encode!([
          %{"id" => "v1", "created_at" => "2026-03-15T01:56:00Z"},
          %{"id" => "v2", "created_at" => "2026-03-16T01:56:00Z"},
          %{"id" => "v3", "created_at" => "2026-03-17T01:56:00Z"},
          %{"id" => "v4", "created_at" => "2026-03-18T01:56:00Z"}
        ])

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:ok, checkpoints_json}

          ["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", checkpoint_id] ->
            {:ok, checkpoint_id}
        end
      end

      try do
        assert :ok =
                 Sprite.gc_checkpoints("bb-builder", org: "misty-step", shell_fn: shell_fn)

        assert [
                 {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _},
                 {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v1"], _},
                 {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v2"], _}
               ] = drain_shell_calls()
      after
        if is_nil(original),
          do: Application.delete_env(:conductor, :max_checkpoints_per_sprite),
          else: Application.put_env(:conductor, :max_checkpoints_per_sprite, original)
      end
    end
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

      {_, _metadata_opts, metadata_files} =
        Enum.find(calls, fn {_command, _opts, uploaded_files} ->
          Enum.any?(uploaded_files, fn {dest, _content} ->
            dest == "/home/sprite/workspace/bitterblossom/.bb/workspace.json"
          end)
        end)

      assert Enum.any?(metadata_files, fn
               {"/home/sprite/workspace/bitterblossom/.bb/workspace.json", content} ->
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
             uploads_to?(opts, "/home/sprite/workspace/bitterblossom/.bb/workspace.json")
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
