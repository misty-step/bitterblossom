defmodule Conductor.RecoveryTest do
  use ExUnit.Case, async: true

  alias Conductor.Recovery

  # --- classify_check/2 ---

  describe "classify_check/2" do
    test "check matching a trusted surface → :known_false_red" do
      check = %{"name" => "cerberus", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      assert Recovery.classify_check(check, ["cerberus"]) == :known_false_red
    end

    test "check matching a trusted surface (case-insensitive) → :known_false_red" do
      check = %{"name" => "Cerberus", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      assert Recovery.classify_check(check, ["cerberus"]) == :known_false_red
    end

    test "check app field matching a trusted surface → :known_false_red" do
      check = %{"name" => "Deploy check", "app" => "cerberus", "conclusion" => "FAILURE"}
      assert Recovery.classify_check(check, ["cerberus"]) == :known_false_red
    end

    test "check not on trusted surface with timeout in name → :transient_infra" do
      check = %{"name" => "e2e-timeout-check", "conclusion" => "FAILURE"}
      assert Recovery.classify_check(check, []) == :transient_infra
    end

    test "check not matching any pattern → :unknown" do
      check = %{"name" => "unit-tests", "conclusion" => "FAILURE"}
      assert Recovery.classify_check(check, []) == :unknown
    end

    test "empty trusted surfaces → never :known_false_red from surface match" do
      check = %{"name" => "cerberus", "conclusion" => "FAILURE"}
      result = Recovery.classify_check(check, [])
      refute result == :known_false_red
    end
  end

  # --- classify_reason/1 ---

  describe "classify_reason/1" do
    test "timeout reason → :transient_infra" do
      assert Recovery.classify_reason("CI did not pass within 15 minutes timeout") ==
               :transient_infra
    end

    test "network reason → :transient_infra" do
      assert Recovery.classify_reason("network connection refused") == :transient_infra
    end

    test "auth reason → :auth_config" do
      assert Recovery.classify_reason("unauthorized: token expired") == :auth_config
    end

    test "test failure reason → :semantic_code" do
      assert Recovery.classify_reason("test suite failed with 3 errors") == :semantic_code
    end

    test "compile reason → :semantic_code" do
      assert Recovery.classify_reason("compile error in module Foo") == :semantic_code
    end

    test "unrecognized reason → :unknown" do
      assert Recovery.classify_reason("something completely different") == :unknown
    end

    test "nil reason → :unknown" do
      assert Recovery.classify_reason(nil) == :unknown
    end
  end

  # --- evaluate_with_policy/2 ---

  describe "evaluate_with_policy/2" do
    test "all green checks → :pass" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert Recovery.evaluate_with_policy(checks, []) == :pass
    end

    test "NEUTRAL and SKIPPED are green → :pass" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Optional", "conclusion" => "NEUTRAL", "status" => "COMPLETED"},
        %{"name" => "Skip", "conclusion" => "SKIPPED", "status" => "COMPLETED"}
      ]

      assert Recovery.evaluate_with_policy(checks, []) == :pass
    end

    test "empty checks → :pending (no real signal)" do
      assert Recovery.evaluate_with_policy([], []) == :pending
    end

    test "only null-conclusion/null-status (annotations) → :pending" do
      checks = [
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert Recovery.evaluate_with_policy(checks, []) == :pending
    end

    test "in-progress check → :pending" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "IN_PROGRESS"}
      ]

      assert Recovery.evaluate_with_policy(checks, []) == :pending
    end

    test "failing check on trusted surface with otherwise green → {:waiver_eligible, [check]}" do
      green = %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      false_red = %{"name" => "cerberus", "conclusion" => "FAILURE", "status" => "COMPLETED"}

      result = Recovery.evaluate_with_policy([green, false_red], ["cerberus"])
      assert {:waiver_eligible, [^false_red]} = result
    end

    test "all failing checks are on trusted surfaces → {:waiver_eligible, checks}" do
      red1 = %{"name" => "cerberus", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      red2 = %{"name" => "external-gate", "conclusion" => "FAILURE", "status" => "COMPLETED"}

      result = Recovery.evaluate_with_policy([red1, red2], ["cerberus", "external-gate"])
      assert {:waiver_eligible, waivers} = result
      assert length(waivers) == 2
    end

    test "non-green check not on trusted surface → {:blocked, [check]}" do
      green = %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      fail = %{"name" => "unit-tests", "conclusion" => "FAILURE", "status" => "COMPLETED"}

      result = Recovery.evaluate_with_policy([green, fail], [])
      assert {:blocked, [^fail]} = result
    end

    test "true semantic failure takes priority over false-red → {:blocked, blockers}" do
      false_red = %{"name" => "cerberus", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      semantic = %{"name" => "unit-tests", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      green = %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}

      result = Recovery.evaluate_with_policy([green, false_red, semantic], ["cerberus"])
      assert {:blocked, blockers} = result
      assert semantic in blockers
      refute false_red in blockers
    end

    test "transient failure (timeout in name) without trusted surface → {:retryable, [check]}" do
      ok = %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      flaky = %{"name" => "e2e-timeout", "conclusion" => "FAILURE", "status" => "COMPLETED"}

      result = Recovery.evaluate_with_policy([ok, flaky], [])
      assert {:retryable, [^flaky]} = result
    end
  end

  # --- failure_class_to_string/1 ---

  describe "failure_class_to_string/1" do
    test "each class converts to expected string" do
      assert Recovery.failure_class_to_string(:transient_infra) == "transient_infra"
      assert Recovery.failure_class_to_string(:auth_config) == "auth_config"
      assert Recovery.failure_class_to_string(:semantic_code) == "semantic_code"
      assert Recovery.failure_class_to_string(:flaky_check) == "flaky_check"
      assert Recovery.failure_class_to_string(:known_false_red) == "known_false_red"
      assert Recovery.failure_class_to_string(:human_policy_block) == "human_policy_block"
      assert Recovery.failure_class_to_string(:unknown) == "unknown"
    end
  end

  # --- retryable?/1 and waivable?/1 ---

  describe "retryable?/1" do
    test "transient_infra is retryable" do
      assert Recovery.retryable?(:transient_infra)
    end

    test "flaky_check is retryable" do
      assert Recovery.retryable?(:flaky_check)
    end

    test "known_false_red is not retryable (it's waivable)" do
      refute Recovery.retryable?(:known_false_red)
    end

    test "semantic_code is not retryable" do
      refute Recovery.retryable?(:semantic_code)
    end

    test "unknown is not retryable" do
      refute Recovery.retryable?(:unknown)
    end
  end

  describe "waivable?/1" do
    test "known_false_red is waivable" do
      assert Recovery.waivable?(:known_false_red)
    end

    test "semantic_code is not waivable" do
      refute Recovery.waivable?(:semantic_code)
    end

    test "transient_infra is not waivable (it's retryable)" do
      refute Recovery.waivable?(:transient_infra)
    end

    test "unknown is not waivable" do
      refute Recovery.waivable?(:unknown)
    end
  end
end
