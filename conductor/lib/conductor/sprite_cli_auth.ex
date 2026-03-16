defmodule Conductor.SpriteCLIAuth do
  @moduledoc """
  Reads sprite CLI auth state from `~/.sprites/sprites.json`.

  The sprite CLI stores its active region and org in a JSON config file.
  Actual tokens live in the macOS keychain (referenced by `keyring_key`);
  this module only reads the config metadata — token retrieval is the
  sprite CLI's responsibility.
  """

  @config_rel_path ".sprites/sprites.json"

  @doc """
  Read and parse the sprite CLI config from `home_dir/.sprites/sprites.json`.
  Defaults to `$HOME` if no home_dir given.

  Returns `{:ok, %{org: org, url: url}}` or `{:error, reason}`.
  """
  @spec read_config() :: {:ok, %{org: binary(), url: binary()}} | {:error, binary()}
  def read_config do
    case resolve_home() do
      {:ok, home} -> read_config(home)
      error -> error
    end
  end

  @spec read_config(binary()) :: {:ok, %{org: binary(), url: binary()}} | {:error, binary()}
  def read_config(home_dir) do
    path = Path.join(home_dir, @config_rel_path)

    with {:ok, data} <- read_file(path),
         {:ok, parsed} <- parse_json(data),
         {:ok, selection} <- extract_selection(parsed) do
      {:ok, selection}
    end
  end

  @doc "Extract just the current org from sprite CLI config."
  @spec current_org() :: {:ok, binary()} | {:error, binary()}
  def current_org do
    case read_config() do
      {:ok, %{org: org}} -> {:ok, org}
      error -> error
    end
  end

  @spec current_org(binary()) :: {:ok, binary()} | {:error, binary()}
  def current_org(home_dir) do
    case read_config(home_dir) do
      {:ok, %{org: org}} -> {:ok, org}
      error -> error
    end
  end

  @doc "True if sprite CLI config exists with a valid current_selection."
  @spec authenticated?() :: boolean()
  def authenticated? do
    match?({:ok, _}, read_config())
  end

  @spec authenticated?(binary()) :: boolean()
  def authenticated?(home_dir) do
    match?({:ok, _}, read_config(home_dir))
  end

  # --- Private ---

  defp read_file(path) do
    case File.read(path) do
      {:ok, data} -> {:ok, data}
      {:error, _} -> {:error, "read sprites.json: #{path} not found or unreadable"}
    end
  end

  defp parse_json(data) do
    case Jason.decode(data) do
      {:ok, parsed} -> {:ok, parsed}
      {:error, _} -> {:error, "parse sprites.json: invalid JSON"}
    end
  end

  defp extract_selection(%{"current_selection" => %{"url" => url, "org" => org}})
       when is_binary(url) and url != "" and is_binary(org) and org != "" do
    {:ok, %{org: org, url: url}}
  end

  defp extract_selection(_), do: {:error, "missing current_selection in sprites.json"}

  defp resolve_home do
    case System.get_env("HOME") do
      nil -> {:error, "HOME not set; cannot locate sprite CLI config"}
      "" -> {:error, "HOME is empty; cannot locate sprite CLI config"}
      home -> {:ok, home}
    end
  end
end
