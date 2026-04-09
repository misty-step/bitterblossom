defmodule Conductor.TestSupport.EnvHelpers do
  @moduledoc false

  def fresh_codex_home do
    home = Path.join(System.tmp_dir!(), "codex_home_#{System.unique_integer([:positive])}")
    File.rm_rf!(home)
    File.mkdir_p!(home)
    home
  end

  def write_auth_json(payload) do
    path = Path.join(System.fetch_env!("CODEX_HOME"), "auth.json")
    File.write!(path, Jason.encode!(payload))
  end

  def restore_env(key, nil), do: System.delete_env(key)
  def restore_env(key, value), do: System.put_env(key, value)
end
