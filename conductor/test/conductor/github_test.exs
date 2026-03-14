defmodule Conductor.GitHubTest do
  use ExUnit.Case, async: true

  alias Conductor.GitHub

  describe "checks_green?/1 (unit, no CLI)" do
    # Test the pure logic extracted into evaluate_checks/1

    test "all SUCCESS → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "mixed case conclusions → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "success", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "NEUTRAL and SKIPPED are non-blocking → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Optional", "conclusion" => "NEUTRAL", "status" => "COMPLETED"},
        %{"name" => "Skipped", "conclusion" => "SKIPPED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "null conclusions are filtered out → true when remaining pass" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => nil, "conclusion" => nil, "status" => nil},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end

    test "only null conclusions → false (no real signal)" do
      checks = [
        %{"name" => nil, "conclusion" => nil, "status" => nil},
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "FAILURE among real checks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Tests", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "FAILURE plus null → false (real failure takes precedence)" do
      checks = [
        %{"name" => "CI", "conclusion" => "FAILURE", "status" => "COMPLETED"},
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "empty list → false" do
      assert GitHub.evaluate_checks([]) == false
    end

    test "in-progress check (nil conclusion, active status) blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "IN_PROGRESS"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "queued check blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "QUEUED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "waiting check (environment protection) blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "WAITING"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "requested check blocks → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => nil, "status" => "REQUESTED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "annotation (nil conclusion AND nil status) is ignored → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks(checks) == true
    end
  end
end
