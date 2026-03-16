defmodule Conductor.CanaryIntegrationTest do
  use ExUnit.Case, async: false

  describe "attach_canary/0" do
    test "skips when CANARY_ENDPOINT is not set" do
      System.delete_env("CANARY_ENDPOINT")
      System.delete_env("CANARY_API_KEY")

      # Should return :ok without crashing
      assert :ok == Conductor.Application.attach_canary()
    end

    test "skips when CANARY_API_KEY is not set" do
      System.put_env("CANARY_ENDPOINT", "https://canary-obs.fly.dev")
      System.delete_env("CANARY_API_KEY")

      assert :ok == Conductor.Application.attach_canary()
    after
      System.delete_env("CANARY_ENDPOINT")
    end

    test "calls CanarySdk.attach when both vars are set" do
      System.put_env("CANARY_ENDPOINT", "https://canary-obs.fly.dev")
      System.put_env("CANARY_API_KEY", "test-key")

      # CanarySdk.attach returns :ok (idempotent via :already_exist handling)
      assert :ok == Conductor.Application.attach_canary()
    after
      System.delete_env("CANARY_ENDPOINT")
      System.delete_env("CANARY_API_KEY")
      CanarySdk.detach()
    end
  end
end
