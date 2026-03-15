defmodule Conductor.Fleet.Reconciler do
  @moduledoc """
  Idempotent sprite provisioning. On boot, ensures every sprite declared
  in fleet.toml is reachable and has the correct harness, config, and repo.

  Calls `bb setup` under the hood until #621 absorbs setup into Elixir.
  If a sprite is unreachable after setup, it's marked degraded but doesn't
  block the conductor from starting with the healthy fleet.
  """

  require Logger
  alias Conductor.{Sprite, Shell}

  @doc """
  Reconcile all declared sprites. Returns `{:ok, results}` where each
  result indicates the sprite's health status after reconciliation.
  """
  @spec reconcile_all([map()]) :: {:ok, [map()]}
  def reconcile_all(sprites) do
    results =
      sprites
      |> Task.async_stream(&reconcile_sprite/1, timeout: 600_000, ordered: false)
      |> Enum.map(fn {:ok, result} -> result end)

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
  @spec reconcile_sprite(map()) :: map()
  def reconcile_sprite(sprite) do
    name = sprite.name
    Logger.info("[fleet] reconciling #{name} (role=#{sprite.role})")

    case check_health(sprite) do
      :healthy ->
        Logger.info("[fleet] #{name} healthy")
        %{name: name, role: sprite.role, healthy: true, action: :none}

      :needs_setup ->
        Logger.info("[fleet] #{name} needs setup, provisioning...")
        provision_and_verify(sprite)

      :unreachable ->
        Logger.error("[fleet] #{name} unreachable")
        %{name: name, role: sprite.role, healthy: false, action: :unreachable}
    end
  end

  # --- Private ---

  defp provision_and_verify(sprite) do
    case run_setup(sprite) do
      :ok ->
        # Re-check health after provisioning to confirm it actually worked
        case check_health(sprite) do
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

  defp check_health(sprite) do
    case Sprite.reachable?(sprite.name) do
      false ->
        :unreachable

      true ->
        harness_cmd =
          case sprite.harness do
            "codex" -> "command -v codex"
            "claude-code" -> "command -v claude"
            _ -> "echo ok"
          end

        case Sprite.exec(sprite.name, harness_cmd, timeout: 15_000) do
          {:ok, _} -> :healthy
          {:error, _, _} -> :needs_setup
        end
    end
  end

  defp run_setup(sprite) do
    bb_path = find_bb()
    repo_flag = if sprite.repo, do: ["--repo", sprite.repo], else: []

    # Pass persona via --persona flag if set (writes to sprite as PERSONA.md)
    {persona_flag, tmp_file} =
      if sprite.persona do
        tmp = Path.join(System.tmp_dir!(), "bb-persona-#{sprite.name}.md")
        File.write!(tmp, sprite.persona)
        {["--persona", tmp], tmp}
      else
        {[], nil}
      end

    args = ["setup", sprite.name] ++ repo_flag ++ persona_flag ++ ["--force"]
    result = Shell.cmd(bb_path, args, timeout: 300_000)
    if tmp_file, do: File.rm(tmp_file)

    case result do
      {:ok, _output} -> :ok
      {:error, output, _code} -> {:error, output}
    end
  end

  defp find_bb do
    cond do
      File.exists?("../bin/bb") -> "../bin/bb"
      File.exists?("./bin/bb") -> "./bin/bb"
      System.find_executable("bb") -> "bb"
      true -> raise "bb binary not found — build with: go build -o bin/bb ./cmd/bb"
    end
  end
end
