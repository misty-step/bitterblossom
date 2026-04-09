defmodule Conductor.Canary.ServiceCatalogTest do
  use ExUnit.Case, async: true

  alias Conductor.Canary.ServiceCatalog

  setup do
    path =
      Path.join(
        System.tmp_dir!(),
        "canary-services-#{System.unique_integer([:positive])}.toml"
      )

    on_exit(fn -> File.rm(path) end)
    %{path: path}
  end

  test "loads valid services with defaults", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "linejam"
      repo = "misty-step/linejam"
      default_branch = "master"
      test_cmd = ["pnpm", "ci:dagger:all-no-e2e"]

      [[service]]
      name = "canary"
      repo = "misty-step/canary"
      default_branch = "master"
      test_cmd = ["./bin/validate"]
      auto_merge = true
      auto_deploy = true
      deploy_cmd = ["flyctl", "deploy", "--app", "canary-obs", "--remote-only"]
      rollback_cmd = ["flyctl", "releases", "rollback", "--app", "canary-obs"]
      stabilization_window_s = 900
      """
    )

    assert {:ok, services} = ServiceCatalog.load(path)
    assert length(services) == 2

    assert {:ok, linejam} = ServiceCatalog.resolve(services, "linejam")
    assert linejam.repo == "misty-step/linejam"
    assert linejam.auto_merge == false
    assert linejam.auto_deploy == false
    assert linejam.stabilization_window_s == 600

    assert {:ok, canary} = ServiceCatalog.resolve(services, "canary")
    assert canary.auto_merge == true
    assert canary.auto_deploy == true
    assert canary.deploy_cmd == ["flyctl", "deploy", "--app", "canary-obs", "--remote-only"]
    assert canary.rollback_cmd == ["flyctl", "releases", "rollback", "--app", "canary-obs"]
    assert canary.stabilization_window_s == 900
  end

  test "returns not_found for unknown services", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "linejam"
      repo = "misty-step/linejam"
      default_branch = "master"
      test_cmd = ["pnpm", "test"]
      """
    )

    assert {:ok, services} = ServiceCatalog.load(path)
    assert {:error, :not_found} = ServiceCatalog.resolve(services, "unknown")
  end

  test "rejects missing service entries", %{path: path} do
    File.write!(path, "")
    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "no [[service]] entries"
  end

  test "rejects duplicate service names", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "linejam"
      repo = "misty-step/linejam"
      default_branch = "master"
      test_cmd = ["pnpm", "test"]

      [[service]]
      name = "linejam"
      repo = "misty-step/linejam"
      default_branch = "master"
      test_cmd = ["pnpm", "test"]
      """
    )

    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "duplicate service name 'linejam'"
  end

  test "rejects unknown keys", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "linejam"
      repo = "misty-step/linejam"
      default_branch = "master"
      test_cmd = ["pnpm", "test"]
      webhook_url = "https://example.com"
      """
    )

    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "unknown keys: webhook_url"
  end

  test "rejects invalid repos", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "linejam"
      repo = "../linejam"
      default_branch = "master"
      test_cmd = ["pnpm", "test"]
      """
    )

    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "invalid repo '../linejam'"
  end

  test "rejects shell command strings", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "linejam"
      repo = "misty-step/linejam"
      default_branch = "master"
      test_cmd = "pnpm test"
      """
    )

    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "test_cmd must be a non-empty array of strings"
  end

  test "requires rollback command for auto deploy", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "canary"
      repo = "misty-step/canary"
      default_branch = "master"
      test_cmd = ["./bin/validate"]
      auto_deploy = true
      deploy_cmd = ["flyctl", "deploy", "--app", "canary-obs", "--remote-only"]
      """
    )

    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "enables auto_deploy without rollback_cmd"
  end

  test "requires deploy command for auto deploy", %{path: path} do
    File.write!(
      path,
      """
      [[service]]
      name = "canary"
      repo = "misty-step/canary"
      default_branch = "master"
      test_cmd = ["./bin/validate"]
      auto_deploy = true
      rollback_cmd = ["flyctl", "releases", "rollback", "--app", "canary-obs"]
      """
    )

    assert {:error, msg} = ServiceCatalog.load(path)
    assert msg =~ "enables auto_deploy without deploy_cmd"
  end
end
