defmodule Conductor.HarnessTest do
  use ExUnit.Case, async: true

  alias Conductor.Harness

  @tag :backoff_strategy
  test "classifies transient dispatch failures" do
    assert {:transient, :network_timeout} =
             Harness.classify_dispatch_failure("network timeout contacting sprite", 124)

    assert {:transient, :resource_contention} =
             Harness.classify_dispatch_failure("temporary resource contention", 75)

    assert {:transient, :worker_unavailable} =
             Harness.classify_dispatch_failure("sprite busy", 1)
  end

  @tag :backoff_strategy
  test "classifies permanent dispatch failures" do
    assert {:permanent, :harness_unsupported} =
             Harness.classify_dispatch_failure(
               "agent exited non-zero; harness does not support continuation",
               1
             )

    assert {:permanent, :auth} =
             Harness.classify_dispatch_failure("gh auth failed on sprite", 4)

    assert {:permanent, :unknown} =
             Harness.classify_dispatch_failure("unexpected squirrel failure", 2)
  end

  @tag :backoff_strategy
  test "computes bounded retry backoff by attempt" do
    Application.put_env(:conductor, :builder_retry_backoff_base_ms, 1_000)

    on_exit(fn ->
      Application.delete_env(:conductor, :builder_retry_backoff_base_ms)
    end)

    assert Harness.retry_backoff_ms(1) == 1_000
    assert Harness.retry_backoff_ms(2) == 2_000
    assert Harness.retry_backoff_ms(3) == 4_000
    assert Harness.retry_backoff_ms(4) == 4_000
  end
end
