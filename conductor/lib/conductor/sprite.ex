defmodule Conductor.Sprite do
  @moduledoc """
  Sprite operations via the `sprite` and `bb` CLIs.

  Deep module: hides all sprite protocol details — exec, dispatch,
  artifact retrieval, process cleanup.

  Implements `Conductor.Worker`.
  """

  @behaviour Conductor.Worker

  alias Conductor.{Shell, Config, Workspace}

  @spec exec(binary(), binary(), keyword()) :: {:ok, binary()} | {:error, binary(), integer()}
  def exec(sprite, command, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 60_000)
    org = Keyword.get(opts, :org, Config.sprites_org!())

    Shell.cmd("sprite", ["-o", org, "-s", sprite, "exec", "bash", "-lc", command],
      timeout: timeout
    )
  end

  @spec exec!(binary(), binary(), keyword()) :: binary()
  def exec!(sprite, command, opts \\ []) do
    case exec(sprite, command, opts) do
      {:ok, output} -> output
      {:error, output, code} -> raise "sprite exec failed (#{code}): #{output}"
    end
  end

  @spec dispatch(binary(), binary(), binary(), keyword()) ::
          {:ok, binary()} | {:error, binary(), integer()}
  def dispatch(sprite, prompt, repo, opts \\ []) do
    timeout_minutes = Keyword.get(opts, :timeout, Config.builder_timeout())
    template = Keyword.get(opts, :template)
    workspace = Keyword.get(opts, :workspace)

    args =
      ["dispatch", sprite, prompt, "--repo", repo, "--timeout", "#{timeout_minutes}m"]
      |> maybe_add("--prompt-template", template)
      |> maybe_add("--workspace", workspace)

    Shell.cmd(Config.bb_path(), args,
      timeout: (timeout_minutes + 5) * 60_000,
      env: Config.dispatch_env()
    )
  end

  @spec read_artifact(binary(), binary(), keyword()) :: {:ok, map()} | {:error, term()}
  def read_artifact(sprite, path, opts \\ []) do
    timeout = Keyword.get(opts, :timeout, 30_000)

    case exec(sprite, "cat #{path}", timeout: timeout) do
      {:ok, json} ->
        case Jason.decode(json) do
          {:ok, data} -> {:ok, data}
          {:error, _} -> {:error, "invalid JSON in artifact: #{String.slice(json, 0, 200)}"}
        end

      {:error, output, _} ->
        {:error, "artifact not found: #{output}"}
    end
  end

  @spec cleanup(binary(), binary(), binary()) :: :ok | {:error, term()}
  def cleanup(sprite, repo, run_id) do
    Workspace.cleanup(sprite, repo, run_id)
  end

  @spec kill(binary()) :: :ok | {:error, term()}
  def kill(sprite) do
    case Shell.cmd(Config.bb_path(), ["kill", sprite], timeout: 30_000) do
      {:ok, _} -> :ok
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec status(binary()) :: {:ok, map()} | {:error, term()}
  def status(sprite) do
    case Shell.cmd(Config.bb_path(), ["status", sprite], timeout: 30_000) do
      {:ok, output} -> {:ok, %{sprite: sprite, output: output, reachable: true}}
      {:error, msg, _} -> {:error, msg}
    end
  end

  @spec reachable?(binary()) :: boolean()
  def reachable?(sprite) do
    match?({:ok, _}, exec(sprite, "echo ok", timeout: 15_000))
  end

  defp maybe_add(args, _flag, nil), do: args
  defp maybe_add(args, flag, value), do: args ++ [flag, value]
end
