defmodule Conductor.HarnessTest do
  use ExUnit.Case, async: false

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
               "[bb harness] configured harness codex unavailable on sprite bb-weaver\n[bb harness] command -v codex -> missing\n[bb harness] command -v claude -> missing\n[bb harness] supported harnesses: codex (codex CLI), claude-code (claude CLI)",
               1
             )

    assert {:permanent, :auth} =
             Harness.classify_dispatch_failure("gh auth failed on sprite", 4)

    assert {:permanent, :unknown} =
             Harness.classify_dispatch_failure(
               "[bb harness] selected harness codex has no continuation command; returning initial failure\nunexpected squirrel failure",
               2
             )
  end

  test "extracts safe harness diagnostics from dispatch output" do
    output = """
    [bb harness] configured harness codex on sprite bb-weaver
    [bb harness] command -v codex -> ok
    raw secret: TOKEN=abc123
    [bb harness] selected harness codex has no continuation command; returning initial failure
    """

    assert Harness.safe_diagnostic_summary(output) ==
             "configured harness codex on sprite bb-weaver | command -v codex -> ok | selected harness codex has no continuation command; returning initial failure"
  end

  test "rejects unknown configured harnesses with actionable diagnostics" do
    exec_fn = fn _sprite, _command, _opts ->
      flunk("unexpected command probe for unknown harness")
    end

    assert {:error, msg, 78} =
             Harness.detect_dispatch_harness("bb-weaver", "claude_code", exec_fn)

    assert msg =~ "configured harness claude_code is unsupported on sprite bb-weaver"
    assert msg =~ "supported harnesses: codex (codex CLI), claude-code (claude CLI)"

    assert {:permanent, :harness_unsupported} =
             Harness.classify_dispatch_failure(msg, 78)
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
