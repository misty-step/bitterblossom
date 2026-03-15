defmodule Conductor.SpriteTest do
  use ExUnit.Case, async: true

  alias Conductor.Sprite

  defp exec_fn(responses) do
    fn _sprite, command, _opts ->
      Enum.find_value(responses, {:ok, ""}, fn {pattern, result} ->
        if String.contains?(command, pattern), do: result
      end)
    end
  end

  test "status reports gh auth and harness readiness" do
    status =
      Sprite.status("bb-builder",
        harness: "codex",
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
      Sprite.status("bb-builder",
        harness: "codex",
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
      Sprite.status("bb-builder",
        harness: "codex",
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

  test "status returns error when sprite is unreachable" do
    assert {:error, "timeout"} =
             Sprite.status("bb-builder",
               harness: "codex",
               exec_fn: fn _sprite, _command, _opts -> {:error, "timeout", 255} end
             )
  end
end
