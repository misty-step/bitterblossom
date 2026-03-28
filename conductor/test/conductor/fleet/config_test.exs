defmodule Conductor.Fleet.ConfigTest do
  use ExUnit.Case, async: true

  alias Conductor.Fleet.Config

  test "sprite_repo/2 prefers the sprite override" do
    assert Config.sprite_repo(%{repo: "override/repo"}, "default/repo") == "override/repo"
  end

  test "sprite_repo/2 falls back to the fleet default" do
    assert Config.sprite_repo(%{}, "default/repo") == "default/repo"
  end

  test "sprite_repo/2 ignores blank sprite overrides" do
    assert Config.sprite_repo(%{repo: ""}, "default/repo") == "default/repo"
  end

  test "launcher_module/0 returns the configured launcher" do
    original = Application.get_env(:conductor, :launcher_module)
    Application.put_env(:conductor, :launcher_module, __MODULE__.MockLauncher)

    on_exit(fn ->
      if original do
        Application.put_env(:conductor, :launcher_module, original)
      else
        Application.delete_env(:conductor, :launcher_module)
      end
    end)

    assert Config.launcher_module() == __MODULE__.MockLauncher
  end
end
