defmodule Conductor.LauncherTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog

  alias Conductor.Launcher

  defmodule MockSpriteModule do
    def stop_loop(name) do
      notify({:stop_loop_called, name})
      :ok
    end

    def detect_auth_failure(name, _opts \\ []) do
      notify({:detect_auth_failure_called, name})

      case Application.get_env(:conductor, :launcher_auth_failure_result, :ok) do
        :ok -> :ok
        {:auth_failure, _} = result -> result
      end
    end

    def force_sync_codex_auth(name) do
      notify({:force_sync_called, name})
      :ok
    end

    def exec(name, command, _opts) do
      notify({:exec_called, name, command})

      if Application.get_env(:conductor, :launcher_repo_checkout_present, false) do
        {:ok, ""}
      else
        {:error, "missing checkout", 1}
      end
    end

    def provision(name, opts) do
      notify({:provision_called, name, opts})
      :ok
    end

    def start_loop(name, prompt, repo, opts) do
      notify({:start_loop_called, name, prompt, repo, opts})
      {:ok, "123\n"}
    end

    defp notify(message) do
      if pid = Application.get_env(:conductor, :launcher_test_pid) do
        send(pid, message)
      end
    end
  end

  defmodule MockBootstrapModule do
    def ensure_spellbook(name) do
      if pid = Application.get_env(:conductor, :launcher_test_pid) do
        send(pid, {:ensure_spellbook_called, name})
      end

      :ok
    end
  end

  defmodule MockWorkspaceModule do
    def repo_root(repo), do: "/tmp/workspaces/#{repo}"
    def persona_for_role(:builder), do: :weaver
    def persona_for_role(:triage), do: :muse
    def persona_for_role(role), do: role

    def sync_persona(sprite, workspace, role, _opts \\ []) do
      if pid = Application.get_env(:conductor, :launcher_test_pid) do
        send(pid, {:sync_persona_called, sprite, workspace, role})
      end

      :ok
    end
  end

  setup do
    orig_sprite_module = Application.get_env(:conductor, :sprite_module)
    orig_bootstrap_module = Application.get_env(:conductor, :bootstrap_module)
    orig_workspace_module = Application.get_env(:conductor, :workspace_module)
    orig_test_pid = Application.get_env(:conductor, :launcher_test_pid)
    orig_checkout_present = Application.get_env(:conductor, :launcher_repo_checkout_present)
    orig_auth_failure = Application.get_env(:conductor, :launcher_auth_failure_result)

    Application.put_env(:conductor, :sprite_module, MockSpriteModule)
    Application.put_env(:conductor, :bootstrap_module, MockBootstrapModule)
    Application.put_env(:conductor, :workspace_module, MockWorkspaceModule)
    Application.put_env(:conductor, :launcher_test_pid, self())

    on_exit(fn ->
      restore_env(:sprite_module, orig_sprite_module)
      restore_env(:bootstrap_module, orig_bootstrap_module)
      restore_env(:workspace_module, orig_workspace_module)
      restore_env(:launcher_test_pid, orig_test_pid)
      restore_env(:launcher_repo_checkout_present, orig_checkout_present)
      restore_env(:launcher_auth_failure_result, orig_auth_failure)
    end)

    :ok
  end

  test "launch reprovisions when the expected repo checkout is missing" do
    Application.put_env(:conductor, :launcher_repo_checkout_present, false)

    sprite = %{
      name: "bb-builder",
      role: :builder,
      repo: "misty-step/bitterblossom",
      harness: "codex",
      reasoning_effort: "medium",
      persona: "You are Weaver."
    }

    assert {:ok, "123\n"} = Launcher.launch(sprite, "misty-step/bitterblossom")

    assert_received {:stop_loop_called, "bb-builder"}
    assert_received {:detect_auth_failure_called, "bb-builder"}
    assert_received {:force_sync_called, "bb-builder"}

    assert_received {:exec_called, "bb-builder",
                     "test -d '/tmp/workspaces/misty-step/bitterblossom/.git'"}

    assert_received {:provision_called, "bb-builder",
                     [
                       repo: "misty-step/bitterblossom",
                       persona: "You are Weaver.",
                       harness: "codex",
                       force: false
                     ]}

    assert_received {:sync_persona_called, "bb-builder",
                     "/tmp/workspaces/misty-step/bitterblossom", :weaver}

    assert_received {:start_loop_called, "bb-builder", prompt, "misty-step/bitterblossom", opts}
    assert prompt =~ "Repository: misty-step/bitterblossom"
    assert opts[:workspace] == "/tmp/workspaces/misty-step/bitterblossom"
  end

  test "launch skips reprovision when the expected repo checkout already exists" do
    Application.put_env(:conductor, :launcher_repo_checkout_present, true)

    sprite = %{
      name: "bb-builder",
      role: :builder,
      repo: "misty-step/bitterblossom",
      harness: "codex",
      reasoning_effort: "medium",
      persona: "You are Weaver."
    }

    assert {:ok, "123\n"} = Launcher.launch(sprite, "misty-step/bitterblossom")

    assert_received {:stop_loop_called, "bb-builder"}
    assert_received {:detect_auth_failure_called, "bb-builder"}
    assert_received {:force_sync_called, "bb-builder"}

    assert_received {:exec_called, "bb-builder",
                     "test -d '/tmp/workspaces/misty-step/bitterblossom/.git'"}

    refute_received {:provision_called, _, _}
    assert_received {:start_loop_called, "bb-builder", _, "misty-step/bitterblossom", _}
  end

  test "launch logs auth failure detection before force sync" do
    Application.put_env(:conductor, :launcher_repo_checkout_present, true)

    Application.put_env(
      :conductor,
      :launcher_auth_failure_result,
      {:auth_failure, "refresh_token_reused"}
    )

    sprite = %{
      name: "bb-builder",
      role: :builder,
      repo: "misty-step/bitterblossom",
      harness: "codex",
      reasoning_effort: "medium",
      persona: "You are Weaver."
    }

    log =
      capture_log(fn ->
        assert {:ok, "123\n"} = Launcher.launch(sprite, "misty-step/bitterblossom")
      end)

    assert log =~ "auth failure detected"
    assert log =~ "refresh_token_reused"
    assert_received {:detect_auth_failure_called, "bb-builder"}
    assert_received {:force_sync_called, "bb-builder"}
  end

  test "launch uses the muse persona for triage sprites" do
    Application.put_env(:conductor, :launcher_repo_checkout_present, true)

    sprite = %{
      name: "bb-muse",
      role: :triage,
      repo: "misty-step/bitterblossom",
      harness: "codex",
      reasoning_effort: "medium",
      persona: "You are Muse. Observe, reflect, and improve."
    }

    assert {:ok, "123\n"} = Launcher.launch(sprite, "misty-step/bitterblossom")

    assert_received {:sync_persona_called, "bb-muse", "/tmp/workspaces/misty-step/bitterblossom",
                     :muse}

    assert_received {:start_loop_called, "bb-muse", prompt, "misty-step/bitterblossom", opts}
    assert prompt =~ "You are Muse."
    assert opts[:persona_role] == :muse
  end

  defp restore_env(key, nil), do: Application.delete_env(:conductor, key)
  defp restore_env(key, value), do: Application.put_env(:conductor, key, value)
end
