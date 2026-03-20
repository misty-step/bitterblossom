defmodule Conductor.ApplicationTest do
  use ExUnit.Case, async: true

  defmodule MockReconciler do
    def reconcile_all(sprites) do
      test_pid = :persistent_term.get({__MODULE__, :test_pid})
      send(test_pid, {:reconciled, sprites})
      {:ok, :persistent_term.get({__MODULE__, :results})}
    end
  end

  defmodule MockOrchestrator do
    def configure_polling(opts) do
      test_pid = :persistent_term.get({__MODULE__, :test_pid})
      send(test_pid, {:configure_polling, opts})
      :ok
    end
  end

  setup do
    :persistent_term.put({MockReconciler, :test_pid}, self())
    :persistent_term.put({MockOrchestrator, :test_pid}, self())

    on_exit(fn ->
      :persistent_term.erase({MockReconciler, :test_pid})
      :persistent_term.erase({MockReconciler, :results})
      :persistent_term.erase({MockOrchestrator, :test_pid})
      Application.delete_env(:conductor, :fleet_config)
      Application.delete_env(:conductor, :fleet_sprites)
      Application.delete_env(:conductor, :fleet_workers)
    end)

    :ok
  end

  test "maps renamed phase worker roles to sprite display names" do
    assert Conductor.Application.role_display_name(:fixer) == "thorn"
    assert Conductor.Application.role_display_name(:polisher) == "fern"
  end

  test "falls back to the raw role name for unmapped roles" do
    assert Conductor.Application.role_display_name(:builder) == "builder"
    assert Conductor.Application.role_display_name(:triage) == "triage"
  end

  test "boot keeps polling configured for builders that were transiently unhealthy" do
    builder = %{
      name: "bb-builder",
      role: :builder,
      capability_tags: [],
      persona: "You are Weaver."
    }

    fixer = %{
      name: "bb-thorn",
      role: :fixer,
      capability_tags: [],
      persona: "You are Thorn."
    }

    config = %{
      sprites: [builder, fixer],
      defaults: %{repo: "misty-step/bitterblossom", label: "autopilot"}
    }

    :persistent_term.put(
      {MockReconciler, :results},
      [
        %{name: builder.name, role: "builder", healthy: false, action: :unreachable},
        %{name: fixer.name, role: "fixer", healthy: true, action: :ok}
      ]
    )

    starter = fn role, sprite, repo ->
      send(self(), {:phase_worker_started, role, sprite.name, repo})
      :ok
    end

    assert :ok =
             Conductor.Application.boot_with_config(
               config,
               "fleet.toml",
               reconciler_mod: MockReconciler,
               orchestrator_mod: MockOrchestrator,
               phase_worker_starter: starter
             )

    assert_received {:reconciled, [^builder, ^fixer]}

    assert_received {:configure_polling, opts}
    assert Keyword.fetch!(opts, :repo) == "misty-step/bitterblossom"
    assert Keyword.fetch!(opts, :label) == "autopilot"
    assert Keyword.fetch!(opts, :workers) == [builder]

    assert_received {:phase_worker_started, :fixer, "bb-thorn", "misty-step/bitterblossom"}
    refute_received {:phase_worker_started, :builder, _, _}

    assert Application.get_env(:conductor, :fleet_workers) == [builder]
  end
end
