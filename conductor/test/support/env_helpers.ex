defmodule Conductor.TestSupport.EnvHelpers do
  @moduledoc false

  def write_auth_json(payload) do
    path = Path.join(System.fetch_env!("CODEX_HOME"), "auth.json")
    File.write!(path, Jason.encode!(payload))
  end

  def restore_env(key, nil), do: System.delete_env(key)
  def restore_env(key, value), do: System.put_env(key, value)
end
