defmodule Conductor.SpriteCLIAuthTest do
  use ExUnit.Case, async: true

  alias Conductor.SpriteCLIAuth

  @valid_config Jason.encode!(%{
                  "current_selection" => %{
                    "url" => "https://api.machines.dev",
                    "org" => "personal"
                  },
                  "urls" => %{
                    "https://api.machines.dev" => %{
                      "orgs" => %{
                        "personal" => %{"keyring_key" => "sprites_personal_token"}
                      }
                    }
                  }
                })

  describe "read_config/1" do
    test "parses valid sprites.json" do
      dir = make_config_dir(@valid_config)

      assert {:ok, %{org: "personal", url: "https://api.machines.dev"}} =
               SpriteCLIAuth.read_config(dir)
    end

    test "returns error when config dir missing" do
      assert {:error, msg} = SpriteCLIAuth.read_config("/nonexistent")
      assert msg =~ "read sprites.json"
    end

    test "returns error on malformed JSON" do
      dir = make_config_dir("{bad")
      assert {:error, msg} = SpriteCLIAuth.read_config(dir)
      assert msg =~ "parse sprites.json"
    end

    test "returns error when current_selection empty" do
      config = Jason.encode!(%{"current_selection" => %{"url" => "", "org" => ""}, "urls" => %{}})
      dir = make_config_dir(config)
      assert {:error, msg} = SpriteCLIAuth.read_config(dir)
      assert msg =~ "missing current_selection"
    end

    test "returns error when current_selection.org missing" do
      config =
        Jason.encode!(%{
          "current_selection" => %{"url" => "https://api.machines.dev"},
          "urls" => %{}
        })

      dir = make_config_dir(config)
      assert {:error, msg} = SpriteCLIAuth.read_config(dir)
      assert msg =~ "missing current_selection"
    end
  end

  describe "current_org/1" do
    test "returns org from valid config" do
      dir = make_config_dir(@valid_config)
      assert {:ok, "personal"} = SpriteCLIAuth.current_org(dir)
    end

    test "returns error when config invalid" do
      assert {:error, _} = SpriteCLIAuth.current_org("/nonexistent")
    end
  end

  describe "authenticated?/1" do
    test "true when config valid with org and url" do
      dir = make_config_dir(@valid_config)
      assert SpriteCLIAuth.authenticated?(dir)
    end

    test "false when config missing" do
      refute SpriteCLIAuth.authenticated?("/nonexistent")
    end

    test "false when current_selection empty" do
      config = Jason.encode!(%{"current_selection" => %{"url" => "", "org" => ""}, "urls" => %{}})
      dir = make_config_dir(config)
      refute SpriteCLIAuth.authenticated?(dir)
    end
  end

  defp make_config_dir(content) do
    tmp = System.tmp_dir!()
    dir = Path.join(tmp, "sprite_cli_auth_test_#{:erlang.unique_integer([:positive])}")
    sprites_dir = Path.join(dir, ".sprites")
    File.mkdir_p!(sprites_dir)
    File.write!(Path.join(sprites_dir, "sprites.json"), content)
    dir
  end
end
