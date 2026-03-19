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
  @repo_root Path.expand("../../../../", __DIR__)

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
    case sprite_mod().status(sprite.name, harness: sprite.harness) do
      {:error, _reason} ->
        :unreachable

      {:ok, %{healthy: true}} ->
        :healthy

      {:ok, _status} ->
        :needs_setup
    end
  end

  defp run_setup(sprite) do
    with {:ok, bb_path} <- find_bb(),
         {:ok, persona_flag, tmp_file} <- build_persona_flag(sprite) do
      repo_flag = if sprite.repo, do: ["--repo", sprite.repo], else: []
      args = ["setup", sprite.name] ++ repo_flag ++ persona_flag ++ ["--force"]
      result = shell_mod().cmd(bb_path, args, timeout: 300_000, cd: repo_root())
      if tmp_file, do: File.rm(tmp_file)

      case result do
        {:ok, _output} -> :ok
        {:error, output, _code} -> {:error, output}
      end
    end
  end

  defp build_persona_flag(%{persona: nil}), do: {:ok, [], nil}
  defp build_persona_flag(%{persona: ""}), do: {:ok, [], nil}

  defp build_persona_flag(%{persona: persona, name: name}) do
    tmp = Path.join(System.tmp_dir!(), "bb-persona-#{name}.md")

    case File.write(tmp, persona) do
      :ok -> {:ok, ["--persona", tmp], tmp}
      {:error, reason} -> {:error, "cannot write persona temp file: #{inspect(reason)}"}
    end
  end

  defp find_bb do
    # Path.expand required: System.cmd/3 does not resolve ".." in executable paths
    repo_bb = Path.join(repo_root(), "bin/bb")
    configured_bb = Application.get_env(:conductor, :bb_path)

    cond do
      is_binary(configured_bb) and configured_bb != "" -> {:ok, configured_bb}
      File.exists?(repo_bb) -> {:ok, repo_bb}
      System.find_executable("bb") -> {:ok, "bb"}
      true -> {:error, "bb binary not found — build with: go build -o bin/bb ./cmd/bb"}
    end
  end

  defp repo_root, do: @repo_root

  defp shell_mod, do: Application.get_env(:conductor, :shell_module, Shell)
  defp sprite_mod, do: Application.get_env(:conductor, :sprite_module, Sprite)
end
