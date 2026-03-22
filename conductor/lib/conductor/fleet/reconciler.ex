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
  alias Conductor.{Config, Sprite, Store}

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

      :needs_setup ->
        Logger.info("[fleet] #{name} needs setup, provisioning...")
        provision_and_verify(sprite, opts)

      :unreachable ->
        recover_unreachable_sprite(sprite, opts)
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
      {:error, _reason} ->
        :unreachable

      {:ok, %{healthy: true}} ->
        :healthy

      {:ok, _status} ->
        :needs_setup
    end
  end

  defp recover_unreachable_sprite(sprite, opts) do
    max_attempts = Keyword.get(opts, :max_attempts, Config.fleet_recovery_max_attempts())
    sleep_fn = Keyword.get(opts, :sleep_fn, &Process.sleep/1)
    wake_fn = Keyword.get(opts, :wake_fn, &Sprite.wake/2)
    event_fn = Keyword.get(opts, :event_fn, &record_recovery_failure_event/3)

    do_recover_unreachable_sprite(sprite, opts, wake_fn, sleep_fn, event_fn, 1, max_attempts, nil)
  end

  defp do_recover_unreachable_sprite(
         sprite,
         opts,
         wake_fn,
         sleep_fn,
         event_fn,
         attempt,
         max_attempts,
         reason
       ) do
    Logger.warning("[fleet] #{sprite.name} unreachable, wake attempt #{attempt}/#{max_attempts}")

    latest_reason =
      case wake_fn.(sprite.name, wake_opts(sprite, opts)) do
        :ok ->
          case check_health(sprite, opts) do
            :healthy ->
              Logger.info("[fleet] #{sprite.name} recovered after wake")
              return_woken(sprite)

            :needs_setup ->
              Logger.info("[fleet] #{sprite.name} reachable after wake, provisioning...")
              provision_and_verify(sprite, opts)

            :unreachable ->
              {:retry, "still unreachable after wake"}
          end

        {:error, wake_reason} ->
          {:retry, wake_reason}
      end

    case latest_reason do
      %{healthy: true} = result ->
        result

      %{healthy: false} = result ->
        result

      {:retry, latest_reason} when attempt < max_attempts ->
        backoff_ms = recovery_backoff_ms(attempt)
        safe_reason = sanitize_recovery_reason(latest_reason)

        Logger.warning(
          "[fleet] #{sprite.name} wake attempt #{attempt}/#{max_attempts} failed: #{safe_reason}; retrying in #{backoff_ms}ms"
        )

        sleep_fn.(backoff_ms)

        do_recover_unreachable_sprite(
          sprite,
          opts,
          wake_fn,
          sleep_fn,
          event_fn,
          attempt + 1,
          max_attempts,
          latest_reason
        )

      {:retry, latest_reason} ->
        log_recovery_failure(
          sprite,
          max_attempts,
          latest_reason || reason || "unknown",
          event_fn
        )

        %{name: sprite.name, role: sprite.role, healthy: false, action: :unreachable}
    end
  end

  defp wake_opts(sprite, opts) do
    [harness: sprite.harness]
    |> maybe_put(:org, Keyword.get(opts, :org))
  end

  defp maybe_put(opts, _key, nil), do: opts
  defp maybe_put(opts, _key, ""), do: opts
  defp maybe_put(opts, key, value), do: Keyword.put(opts, key, value)

  defp return_woken(sprite) do
    %{name: sprite.name, role: sprite.role, healthy: true, action: :woken}
  end

  defp recovery_backoff_ms(attempt) do
    base = Config.fleet_recovery_backoff_base_ms()
    cap = Config.fleet_recovery_backoff_cap_ms()
    min(trunc(base * :math.pow(2, attempt - 1)), cap)
  end

  defp log_recovery_failure(sprite, attempts, reason, event_fn) do
    safe_reason = sanitize_recovery_reason(reason)

    Logger.error(
      "[fleet] #{sprite.name} unreachable after #{attempts} wake attempt(s); operator attention required"
    )

    event_fn.(sprite, attempts, safe_reason)
  end

  defp record_recovery_failure_event(sprite, attempts, reason) do
    if Process.whereis(Store) do
      Store.record_event("fleet", "sprite_recovery_failed", %{
        name: sprite.name,
        role: to_string(sprite.role),
        attempts: attempts,
        reason: reason
      })
    end
  end

  defp sanitize_recovery_reason(reason) when is_binary(reason) do
    reason
    |> String.replace(~r/[\x00-\x1F\x7F]/u, " ")
    |> String.replace(~r/\s+/u, " ")
    |> String.trim()
    |> case do
      "" -> "unknown"
      sanitized -> sanitized
    end
  end

  defp sanitize_recovery_reason(reason), do: reason |> inspect() |> sanitize_recovery_reason()
end
