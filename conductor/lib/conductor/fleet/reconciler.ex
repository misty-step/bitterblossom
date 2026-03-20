defmodule Conductor.Fleet.Reconciler do
  @moduledoc """
  Idempotent sprite provisioning. On boot, ensures every sprite declared
  in fleet.toml is reachable and has the correct harness, config, and repo.

  Provisioning now runs directly through `Conductor.Sprite`, so fleet repair
  no longer depends on the deleted Go transport CLI. If a sprite is unreachable
  after setup, it's marked degraded but doesn't block the conductor from
  starting with the healthy fleet.
  """

  require Logger
  alias Conductor.Sprite
  @missing_sprite_error_fragment "sprite not found"

  @doc """
  Reconcile all declared sprites. Returns `{:ok, results}` where each
  result indicates the sprite's health status after reconciliation.
  """
  @spec reconcile_all([map()], keyword()) :: {:ok, [map()]}
  def reconcile_all(sprites, opts \\ []) do
    results =
      sprites
      |> Task.async_stream(
        fn sprite ->
          try do
            {:ok, reconcile_sprite(sprite, opts)}
          rescue
            error -> {:error, Exception.message(error)}
          catch
            kind, reason -> {:error, {kind, reason}}
          end
        end,
        timeout: 600_000,
        ordered: true,
        on_timeout: :kill_task
      )
      |> Enum.zip(sprites)
      |> Enum.map(fn
        {{:ok, {:ok, result}}, _sprite} ->
          result

        {{:ok, {:error, reason}}, sprite} ->
          Logger.error("[fleet] #{sprite.name} reconcile crashed: #{inspect(reason)}")
          %{name: sprite.name, role: sprite.role, healthy: false, action: :failed}

        {{:exit, reason}, sprite} ->
          Logger.error("[fleet] #{sprite.name} reconcile crashed: #{inspect(reason)}")
          %{name: sprite.name, role: sprite.role, healthy: false, action: :failed}
      end)

    healthy = Enum.count(results, & &1.healthy)
    degraded = Enum.count(results, &(not &1.healthy))

    if healthy > 0 do
      Logger.info(
        "[fleet] reconciled #{length(sprites)} sprite(s): #{healthy} healthy, #{degraded} degraded"
      )
    else
      Logger.error("[fleet] all #{length(sprites)} sprite(s) degraded — no healthy workers")
    end

    {:ok, results}
  end

  @doc "Reconcile a single sprite. Returns health status."
  @spec reconcile_sprite(map(), keyword()) :: map()
  def reconcile_sprite(sprite, opts \\ []) do
    name = sprite.name
    Logger.info("[fleet] reconciling #{name} (role=#{sprite.role})")

    case check_health(sprite, opts) do
      :healthy ->
        Logger.info("[fleet] #{name} healthy")
        %{name: name, role: sprite.role, healthy: true, action: :none}

      {:missing, _reason} ->
        Logger.info("[fleet] #{name} missing, creating...")
        create_and_provision(sprite, opts)

      :needs_setup ->
        Logger.info("[fleet] #{name} needs setup, provisioning...")
        provision_and_verify(sprite, opts)

      :unreachable ->
        Logger.error("[fleet] #{name} unreachable")
        %{name: name, role: sprite.role, healthy: false, action: :unreachable}
    end
  end

  # --- Private ---

  defp provision_and_verify(sprite, opts) do
    provision_fn = Keyword.get(opts, :provision_fn, &Sprite.provision/2)

    case provision_fn.(sprite.name,
           repo: sprite.repo,
           persona: sprite.persona,
           force: true
         ) do
      :ok ->
        # Re-check health after provisioning to confirm it actually worked
        case check_health(sprite, opts) do
          :healthy ->
            Logger.info("[fleet] #{sprite.name} provisioned and verified healthy")
            %{name: sprite.name, role: sprite.role, healthy: true, action: :provisioned}

          status ->
            Logger.warning(
              "[fleet] #{sprite.name} provisioned but health check returned #{status}"
            )

            %{name: sprite.name, role: sprite.role, healthy: false, action: :setup_incomplete}
        end

      {:error, reason} ->
        Logger.error("[fleet] #{sprite.name} provisioning failed: #{reason}")
        %{name: sprite.name, role: sprite.role, healthy: false, action: :failed}
    end
  end

  defp check_health(sprite, opts) do
    status_fn =
      Keyword.get(opts, :status_fn, fn name, status_opts ->
        Sprite.status(name, status_opts)
      end)

    case status_fn.(sprite.name, harness: sprite.harness) do
      {:error, reason} ->
        if is_binary(reason) and missing_sprite_error?(reason) do
          {:missing, reason}
        else
          :unreachable
        end

      {:ok, %{healthy: true}} ->
        :healthy

      {:ok, _status} ->
        :needs_setup
    end
  end

  defp create_and_provision(sprite, opts) do
    create_fn = Keyword.get(opts, :create_fn, &Sprite.create/2)

    create_opts =
      case Map.get(sprite, :org) do
        nil -> []
        org -> [org: org]
      end

    case create_fn.(sprite.name, create_opts) do
      :ok ->
        case provision_and_verify(sprite, opts) do
          %{healthy: true} = result -> %{result | action: :created}
          result -> result
        end

      {:error, reason} ->
        Logger.error("[fleet] #{sprite.name} creation failed: #{reason}")
        %{name: sprite.name, role: sprite.role, healthy: false, action: :failed}
    end
  end

  defp missing_sprite_error?(reason) do
    String.contains?(String.downcase(reason), @missing_sprite_error_fragment)
  end
end
