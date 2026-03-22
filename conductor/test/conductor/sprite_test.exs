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

    test "supports HTTP POST transport for wake-safe execs" do
      args = Sprite.exec_args("org", "sprite", [], "echo hello", transport: :http_post)
      assert "--http-post" in args
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

  test "status stays on the injected exec path when no state_fn is provided" do
    status =
      Sprite.status("bb-weaver",
        harness: "codex",
        shell_fn: fn _, _, _ -> flunk("unexpected shell state lookup") end,
        exec_fn:
          exec_fn([
            {"echo ok", {:ok, "ok\n"}},
            {"command -v codex", {:ok, "/usr/bin/codex\n"}},
            {"gh auth status", {:ok, "github.com\n"}},
            {"git config --global --get credential.helper", {:ok, "!gh auth git-credential"}}
          ])
      )

    assert {:ok, %{reachable: true, healthy: true}} = status
  end

  describe "gc_checkpoints/2" do
    test "prunes oldest checkpoints and keeps the newest configured count" do
      test_pid = self()

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

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert :ok =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 2
               )

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _},
               {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v1"], _},
               {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v2"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "accepts nested checkpoint payload shapes" do
      test_pid = self()

      Enum.each(["checkpoints", "data"], fn key ->
        checkpoints_json =
          Jason.encode!(%{
            key => [
              %{"id" => "v1", "created_at" => "2026-03-15T01:56:00Z"},
              %{"id" => "v2", "created_at" => "2026-03-16T01:56:00Z"}
            ]
          })

        shell_fn = fn "sprite", args, opts ->
          send(test_pid, {:shell_called, args, opts})

          case args do
            ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
              {:ok, checkpoints_json}

            ["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v1"] ->
              {:ok, "v1"}

            _ ->
              send(test_pid, {:unexpected_shell_args, args, opts})
              {:error, "unexpected shell args: #{inspect(args)}", 1}
          end
        end

        assert :ok =
                 Sprite.gc_checkpoints("bb-builder",
                   org: "misty-step",
                   shell_fn: shell_fn,
                   max_keep: 1
                 )

        assert [
                 {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _},
                 {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v1"], _}
               ] = drain_shell_calls()
      end)

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns ok without delete calls when there are no stale checkpoints" do
      test_pid = self()
      checkpoints_json = Jason.encode!([])

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:ok, checkpoints_json}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert :ok =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 2
               )

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when checkpoint listing fails" do
      test_pid = self()

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:error, "sprite api unavailable", 1}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "sprite api unavailable"} =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 2
               )

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when checkpoint payload is invalid json" do
      test_pid = self()

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:ok, "{not-json"}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, reason} =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 2
               )

      assert is_binary(reason)
      assert reason != ""

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when checkpoint payload contains non-map entries" do
      test_pid = self()
      checkpoints_json = Jason.encode!(["v1", nil])

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:ok, checkpoints_json}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "unexpected checkpoint entry"} =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 2
               )

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when a stale checkpoint is missing an id" do
      test_pid = self()

      checkpoints_json =
        Jason.encode!([
          %{"id" => "v2", "created_at" => "2026-03-16T01:56:00Z"},
          %{"created_at" => "2026-03-15T01:56:00Z"}
        ])

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:ok, checkpoints_json}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "checkpoint missing id"} =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 1
               )

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns the first delete error after partial checkpoint pruning" do
      test_pid = self()

      checkpoints_json =
        Jason.encode!([
          %{"id" => "v1", "created_at" => "2026-03-15T01:56:00Z"},
          %{"id" => "v2", "created_at" => "2026-03-16T01:56:00Z"},
          %{"id" => "v3", "created_at" => "2026-03-17T01:56:00Z"}
        ])

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"] ->
            {:ok, checkpoints_json}

          ["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v1"] ->
            {:ok, "v1"}

          ["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v2"] ->
            {:error, "delete failed", 1}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "delete failed"} =
               Sprite.gc_checkpoints("bb-builder",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 max_keep: 1
               )

      assert [
               {["api", "-o", "misty-step", "-s", "bb-builder", "/checkpoints"], _},
               {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v1"], _},
               {["-o", "misty-step", "-s", "bb-builder", "checkpoint", "delete", "v2"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end
  end

  test "exec wakes and retries over HTTP POST after websocket handshake failure" do
    parent = self()

    shell_cmd_fn = fn "sprite", args, opts ->
      send(parent, {:shell_cmd, args, opts})

      cond do
        "--http-post" in args and List.last(args) == "true" ->
          {:ok, ""}

        "--http-post" in args ->
          {:ok, "recovered"}

        true ->
          {:error, "websocket: bad handshake (HTTP 502)", 1}
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
    assert "--http-post" in wake_args
    assert List.last(wake_args) == "true"

    assert_received {:shell_cmd, retry_args, _opts}
    assert "--http-post" in retry_args
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

  describe "check_stuck/2" do
    test "recreates sprites that are old enough and report last_running_at null" do
      test_pid = self()

      sprites_json =
        Jason.encode!(%{
          "sprites" => [
            %{
              "name" => "bb-fixer",
              "created_at" => "2026-03-22T11:50:00Z",
              "last_running_at" => nil
            }
          ]
        })

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "/sprites"] ->
            {:ok, sprites_json}

          ["destroy", "-o", "misty-step", "--force", "bb-fixer"] ->
            {:ok, "destroyed"}

          ["create", "-o", "misty-step", "--skip-console", "bb-fixer"] ->
            {:ok, "created"}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:ok, :recreated} =
               Sprite.check_stuck("bb-fixer",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 now_fn: fn -> ~U[2026-03-22 12:00:00Z] end
               )

      assert [
               {["api", "-o", "misty-step", "/sprites"], _},
               {["destroy", "-o", "misty-step", "--force", "bb-fixer"], _},
               {["create", "-o", "misty-step", "--skip-console", "bb-fixer"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when listing sprites fails" do
      test_pid = self()

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "/sprites"] ->
            {:error, "sprite api unavailable", 1}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "sprite api unavailable"} =
               Sprite.check_stuck("bb-fixer",
                 org: "misty-step",
                 shell_fn: shell_fn
               )

      assert [{["api", "-o", "misty-step", "/sprites"], _}] = drain_shell_calls()
      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when the target sprite is missing from the payload" do
      test_pid = self()
      sprites_json = Jason.encode!(%{"sprites" => [%{"name" => "bb-polisher"}]})

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "/sprites"] ->
            {:ok, sprites_json}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "sprite not found"} =
               Sprite.check_stuck("bb-fixer",
                 org: "misty-step",
                 shell_fn: shell_fn
               )

      assert [{["api", "-o", "misty-step", "/sprites"], _}] = drain_shell_calls()
      refute_received {:unexpected_shell_args, _, _}
    end

    test "falls back to the configured org when opts pass nil or blank orgs" do
      test_pid = self()

      sprites_json =
        Jason.encode!(%{"sprites" => [%{"name" => "bb-fixer", "last_running_at" => nil}]})

      previous_sprites_org = System.get_env("SPRITES_ORG")
      previous_fly_org = System.get_env("FLY_ORG")

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "fallback-org", "/sprites"] ->
            {:ok, sprites_json}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      try do
        System.put_env("SPRITES_ORG", "fallback-org")
        System.delete_env("FLY_ORG")

        Enum.each([nil, ""], fn org ->
          assert {:ok, :not_stuck} =
                   Sprite.check_stuck("bb-fixer",
                     org: org,
                     shell_fn: shell_fn
                   )
        end)
      after
        if previous_sprites_org do
          System.put_env("SPRITES_ORG", previous_sprites_org)
        else
          System.delete_env("SPRITES_ORG")
        end

        if previous_fly_org do
          System.put_env("FLY_ORG", previous_fly_org)
        else
          System.delete_env("FLY_ORG")
        end
      end

      assert [
               {["api", "-o", "fallback-org", "/sprites"], _},
               {["api", "-o", "fallback-org", "/sprites"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when destroying a stuck sprite fails" do
      test_pid = self()

      sprites_json =
        Jason.encode!(%{
          "sprites" => [
            %{
              "name" => "bb-fixer",
              "created_at" => "2026-03-22T11:50:00Z",
              "last_running_at" => nil
            }
          ]
        })

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "/sprites"] ->
            {:ok, sprites_json}

          ["destroy", "-o", "misty-step", "--force", "bb-fixer"] ->
            {:error, "destroy failed", 1}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "destroy failed"} =
               Sprite.check_stuck("bb-fixer",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 now_fn: fn -> ~U[2026-03-22 12:00:00Z] end
               )

      assert [
               {["api", "-o", "misty-step", "/sprites"], _},
               {["destroy", "-o", "misty-step", "--force", "bb-fixer"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "returns an error when recreating a stuck sprite fails after destroy" do
      test_pid = self()

      sprites_json =
        Jason.encode!(%{
          "sprites" => [
            %{
              "name" => "bb-fixer",
              "created_at" => "2026-03-22T11:50:00Z",
              "last_running_at" => nil
            }
          ]
        })

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "/sprites"] ->
            {:ok, sprites_json}

          ["destroy", "-o", "misty-step", "--force", "bb-fixer"] ->
            {:ok, "destroyed"}

          ["create", "-o", "misty-step", "--skip-console", "bb-fixer"] ->
            {:error, "create failed", 1}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:error, "create failed"} =
               Sprite.check_stuck("bb-fixer",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 now_fn: fn -> ~U[2026-03-22 12:00:00Z] end
               )

      assert [
               {["api", "-o", "misty-step", "/sprites"], _},
               {["destroy", "-o", "misty-step", "--force", "bb-fixer"], _},
               {["create", "-o", "misty-step", "--skip-console", "bb-fixer"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
    end

    test "does not recreate sprites that are still recent or have run before" do
      test_pid = self()

      sprites_json =
        Jason.encode!(%{
          "sprites" => [
            %{
              "name" => "bb-fixer",
              "created_at" => "2026-03-22T11:58:00Z",
              "last_running_at" => nil
            },
            %{
              "name" => "bb-polisher",
              "created_at" => "2026-03-22T11:00:00Z",
              "last_running_at" => "2026-03-22T11:59:00Z"
            }
          ]
        })

      shell_fn = fn "sprite", args, opts ->
        send(test_pid, {:shell_called, args, opts})

        case args do
          ["api", "-o", "misty-step", "/sprites"] ->
            {:ok, sprites_json}

          _ ->
            send(test_pid, {:unexpected_shell_args, args, opts})
            {:error, "unexpected shell args: #{inspect(args)}", 1}
        end
      end

      assert {:ok, :not_stuck} =
               Sprite.check_stuck("bb-fixer",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 now_fn: fn -> ~U[2026-03-22 12:00:00Z] end
               )

      assert {:ok, :not_stuck} =
               Sprite.check_stuck("bb-polisher",
                 org: "misty-step",
                 shell_fn: shell_fn,
                 now_fn: fn -> ~U[2026-03-22 12:00:00Z] end
               )

      assert [
               {["api", "-o", "misty-step", "/sprites"], _},
               {["api", "-o", "misty-step", "/sprites"], _}
             ] = drain_shell_calls()

      refute_received {:unexpected_shell_args, _, _}
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
