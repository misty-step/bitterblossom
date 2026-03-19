defmodule Conductor.SelfUpdateTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog

  alias Conductor.SelfUpdate

  defmodule MockShell do
    def cmd(program, args, opts \\ []) do
      calls = Process.get(:self_update_shell_calls, [])
      Process.put(:self_update_shell_calls, calls ++ [{program, args, opts}])

      case Process.get(:self_update_shell_handler) do
        nil -> {:error, "unexpected command: #{program} #{Enum.join(args, " ")}", 1}
        handler -> handler.(program, args, opts)
      end
    end
  end

  defmodule MockCompiler do
    def recompile do
      calls = Process.get(:self_update_compile_calls, 0)
      Process.put(:self_update_compile_calls, calls + 1)
      :ok
    end
  end

  defmodule TestClock do
    def system_time(:millisecond), do: Process.get(:self_update_now_ms, 0)
  end

  @repo_root Path.expand("../../..", __DIR__)

  setup do
    original_shell = Application.get_env(:conductor, :self_update_shell_module)
    original_compiler = Application.get_env(:conductor, :self_update_compiler_module)
    original_clock = Application.get_env(:conductor, :self_update_clock_module)

    Application.put_env(:conductor, :self_update_shell_module, MockShell)
    Application.put_env(:conductor, :self_update_compiler_module, MockCompiler)
    Application.put_env(:conductor, :self_update_clock_module, TestClock)

    Process.delete(:self_update_shell_calls)
    Process.delete(:self_update_shell_handler)
    Process.delete(:self_update_compile_calls)
    Process.delete(:self_update_now_ms)
    Process.delete({SelfUpdate, :last_warning_ms})

    on_exit(fn ->
      restore_env(:self_update_shell_module, original_shell)
      restore_env(:self_update_compiler_module, original_compiler)
      restore_env(:self_update_clock_module, original_clock)
      Process.delete(:self_update_shell_calls)
      Process.delete(:self_update_shell_handler)
      Process.delete(:self_update_compile_calls)
      Process.delete(:self_update_now_ms)
      Process.delete({SelfUpdate, :last_warning_ms})
    end)

    :ok
  end

  describe "check_for_updates/0" do
    test "skips self-update when builder worktrees are active" do
      Process.put(:self_update_shell_handler, fn
        "git", ["-C", @repo_root, "worktree", "list", "--porcelain"], _opts ->
          {:ok,
           """
           worktree #{@repo_root}
           HEAD abc123
           branch refs/heads/master

           worktree #{@repo_root}/.bb/conductor/run-123/builder-worktree
           HEAD def456
           branch refs/heads/factory/123
           """}

        program, args, _opts ->
          flunk("unexpected command: #{program} #{inspect(args)}")
      end)

      assert SelfUpdate.check_for_updates() == :noop
      assert Process.get(:self_update_compile_calls, 0) == 0

      assert Process.get(:self_update_shell_calls) == [
               {"git", ["-C", @repo_root, "worktree", "list", "--porcelain"], [timeout: 10_000]}
             ]
    end

    test "fails closed when worktree inspection errors" do
      Process.put(:self_update_shell_handler, fn
        "git", ["-C", @repo_root, "worktree", "list", "--porcelain"], _opts ->
          {:error, "permission denied", 1}

        program, args, _opts ->
          flunk("unexpected command: #{program} #{inspect(args)}")
      end)

      log =
        capture_log(fn ->
          assert SelfUpdate.check_for_updates() == :noop
        end)

      assert log =~
               "[self-update] worktree inspection failed, skipping update to be safe: permission denied"

      assert Process.get(:self_update_compile_calls, 0) == 0
    end

    test "hard-resets to origin/master and recompiles when behind" do
      Process.put(:self_update_shell_handler, fn
        "git", ["-C", @repo_root, "worktree", "list", "--porcelain"], _opts ->
          {:ok, "worktree #{@repo_root}\nHEAD abc123\nbranch refs/heads/master\n"}

        "git", ["-C", @repo_root, "fetch", "origin", "master", "--quiet"], _opts ->
          {:ok, ""}

        "git", ["-C", @repo_root, "rev-list", "--count", "HEAD..origin/master"], _opts ->
          {:ok, "2\n"}

        "git", ["-C", @repo_root, "reset", "--hard", "origin/master"], _opts ->
          {:ok, "HEAD is now at abc123 update"}

        program, args, _opts ->
          flunk("unexpected command: #{program} #{inspect(args)}")
      end)

      assert SelfUpdate.check_for_updates() == :ok
      assert Process.get(:self_update_compile_calls, 0) == 1

      calls = Process.get(:self_update_shell_calls)

      assert {"git", ["-C", @repo_root, "reset", "--hard", "origin/master"], [timeout: 30_000]} in calls

      refute Enum.any?(calls, fn {_program, args, _opts} -> Enum.member?(args, "pull") end)
    end

    test "rate-limits self-update warnings to once per minute" do
      Process.put(:self_update_shell_handler, fn
        "git", ["-C", @repo_root, "worktree", "list", "--porcelain"], _opts ->
          {:ok, "worktree #{@repo_root}\nHEAD abc123\nbranch refs/heads/master\n"}

        "git", ["-C", @repo_root, "fetch", "origin", "master", "--quiet"], _opts ->
          {:ok, ""}

        "git", ["-C", @repo_root, "rev-list", "--count", "HEAD..origin/master"], _opts ->
          {:ok, "1\n"}

        "git", ["-C", @repo_root, "reset", "--hard", "origin/master"], _opts ->
          {:error, "diverged", 1}

        program, args, _opts ->
          flunk("unexpected command: #{program} #{inspect(args)}")
      end)

      log =
        capture_log(fn ->
          Process.put(:self_update_now_ms, 0)
          assert SelfUpdate.check_for_updates() == :noop

          Process.put(:self_update_now_ms, 13_000)
          assert SelfUpdate.check_for_updates() == :noop

          Process.put(:self_update_now_ms, 61_000)
          assert SelfUpdate.check_for_updates() == :noop
        end)

      assert Regex.scan(~r/\[self-update\] git reset failed: diverged/, log) |> length() == 2
    end
  end

  describe "maybe_reload/2" do
    test "returns :noop for non-self repo" do
      assert SelfUpdate.maybe_reload("other-org/other-repo", 1) == :noop
    end

    test "fetches latest remote state before resetting after a self-merge" do
      Process.put(:self_update_shell_handler, fn
        "git", ["-C", @repo_root, "remote", "get-url", "origin"], _opts ->
          {:ok, "https://github.com/misty-step/bitterblossom.git"}

        "gh",
        [
          "pr",
          "view",
          "42",
          "--repo",
          "misty-step/bitterblossom",
          "--json",
          "files",
          "--jq",
          ".files[].path"
        ],
        _opts ->
          {:ok, "conductor/lib/conductor/self_update.ex\n"}

        "git", ["-C", @repo_root, "worktree", "list", "--porcelain"], _opts ->
          {:ok, "worktree #{@repo_root}\nHEAD abc123\nbranch refs/heads/master\n"}

        "git", ["-C", @repo_root, "fetch", "origin", "master", "--quiet"], _opts ->
          {:ok, ""}

        "git", ["-C", @repo_root, "reset", "--hard", "origin/master"], _opts ->
          {:ok, "HEAD is now at fedcba merged update"}

        program, args, _opts ->
          flunk("unexpected command: #{program} #{inspect(args)}")
      end)

      assert SelfUpdate.maybe_reload("misty-step/bitterblossom", 42) == :ok
      assert Process.get(:self_update_compile_calls, 0) == 1

      assert Process.get(:self_update_shell_calls) == [
               {"git", ["-C", @repo_root, "remote", "get-url", "origin"], [timeout: 10_000]},
               {"gh",
                [
                  "pr",
                  "view",
                  "42",
                  "--repo",
                  "misty-step/bitterblossom",
                  "--json",
                  "files",
                  "--jq",
                  ".files[].path"
                ], []},
               {"git", ["-C", @repo_root, "worktree", "list", "--porcelain"], [timeout: 10_000]},
               {"git", ["-C", @repo_root, "fetch", "origin", "master", "--quiet"],
                [timeout: 30_000]},
               {"git", ["-C", @repo_root, "reset", "--hard", "origin/master"], [timeout: 30_000]}
             ]
    end
  end

  defp restore_env(key, nil), do: Application.delete_env(:conductor, key)
  defp restore_env(key, value), do: Application.put_env(:conductor, key, value)
end
