defmodule Conductor.GitHubTest do
  use ExUnit.Case, async: false

  alias Conductor.{GitHub, Issue}

  describe "list_issue_args/2" do
    test "omits --label and raises the default limit when label is nil" do
      args = GitHub.list_issue_args("test/repo", label: nil)

      refute "--label" in args
      assert Enum.at(args, -1) == "1000"
    end

    test "includes --label and keeps the narrow default limit when label is set" do
      args = GitHub.list_issue_args("test/repo", label: "autopilot")

      assert args |> Enum.take(-2) == ["--label", "autopilot"]
      assert "--limit" in args
      assert Enum.at(args, Enum.find_index(args, &(&1 == "--limit")) + 1) == "25"
    end

    test "respects an explicit limit override" do
      args = GitHub.list_issue_args("test/repo", label: nil, limit: 50)
      assert Enum.at(args, Enum.find_index(args, &(&1 == "--limit")) + 1) == "50"
    end
  end

  describe "sort_eligible_issues/1" do
    test "retains underspecified issues while sorting by issue number" do
      ready_issue = %Issue{
        number: 20,
        title: "ready",
        body: "## Problem\nx\n\n## Acceptance Criteria\n- [ ] [test] y",
        url: "https://example.test/issues/20"
      }

      unready_issue = %Issue{
        number: 10,
        title: "unready",
        body: "missing sections",
        url: "https://example.test/issues/10"
      }

      assert GitHub.sort_eligible_issues([ready_issue, unready_issue]) == [
               unready_issue,
               ready_issue
             ]
    end
  end

  defp with_fake_gh(script, fun) do
    tmp_dir = Path.join(System.tmp_dir!(), "github_test_#{System.unique_integer([:positive])}")
    gh_path = Path.join(tmp_dir, "gh")
    args_path = Path.join(tmp_dir, "gh-args.log")
    prev_path = System.get_env("PATH") || ""
    prev_args_path = System.get_env("GH_ARGS_PATH")

    File.mkdir_p!(tmp_dir)
    File.write!(gh_path, "#!/usr/bin/env bash\nset -eu\n#{script}\n")
    File.chmod!(gh_path, 0o755)
    System.put_env("PATH", "#{tmp_dir}:#{prev_path}")
    System.put_env("GH_ARGS_PATH", args_path)

    try do
      fun.(tmp_dir, args_path)
    after
      System.put_env("PATH", prev_path)

      if prev_args_path,
        do: System.put_env("GH_ARGS_PATH", prev_args_path),
        else: System.delete_env("GH_ARGS_PATH")

      File.rm_rf!(tmp_dir)
    end
  end

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

  describe "find_open_pr/2 — branch prefix filtering" do
    # Test the pure filter logic extracted from find_open_pr.
    # The function finds the first PR whose headRefName starts with "factory/<issue>-".

    defp filter_open_pr(prs, issue_number) do
      prefix = "factory/#{issue_number}-"
      Enum.find(prs, fn pr -> String.starts_with?(pr["headRefName"] || "", prefix) end)
    end

    test "matches PR with correct issue prefix" do
      prs = [
        %{
          "number" => 10,
          "headRefName" => "factory/42-1234567890",
          "url" => "http://example.com/10"
        },
        %{
          "number" => 11,
          "headRefName" => "factory/99-9999999999",
          "url" => "http://example.com/11"
        }
      ]

      result = filter_open_pr(prs, 42)
      assert result["number"] == 10
    end

    test "returns nil when no PR matches the issue number" do
      prs = [
        %{
          "number" => 10,
          "headRefName" => "factory/99-1234567890",
          "url" => "http://example.com/10"
        }
      ]

      assert filter_open_pr(prs, 42) == nil
    end

    test "does not match a different issue number that shares a prefix" do
      prs = [
        %{
          "number" => 10,
          "headRefName" => "factory/420-1234567890",
          "url" => "http://example.com/10"
        }
      ]

      # issue 42 should NOT match factory/420-... (dash is the delimiter)
      assert filter_open_pr(prs, 42) == nil
    end

    test "handles nil headRefName gracefully" do
      prs = [%{"number" => 10, "headRefName" => nil, "url" => "http://example.com/10"}]
      assert filter_open_pr(prs, 42) == nil
    end

    test "returns nil for empty list" do
      assert filter_open_pr([], 42) == nil
    end
  end

  describe "list_issues/2" do
    test "omits --label when label is nil and preserves an explicit limit" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        []
        JSON
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, []} = GitHub.list_issues("misty-step/bitterblossom", label: nil, limit: 12)

          args = File.read!(args_path)
          assert String.contains?(args, "issue\nlist\n")
          assert String.contains?(args, "--limit\n12\n")
          refute String.contains?(args, "--label\n")
        end
      )
    end

    test "omits --label when label is blank" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        []
        JSON
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, []} = GitHub.list_issues("misty-step/bitterblossom", label: "", limit: 8)

          args = File.read!(args_path)
          refute String.contains?(args, "--label\n")
        end
      )
    end

    test "includes --label when a label filter is provided" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        []
        JSON
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, []} =
                   GitHub.list_issues("misty-step/bitterblossom", label: "autopilot", limit: 7)

          args = File.read!(args_path)
          assert String.contains?(args, "--label\nautopilot\n")
        end
      )
    end

    test "paginates all open issues by default and eligible_issues keeps unready issues" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        [
          {
            "data": {
              "repository": {
                "issues": {
                  "nodes": [
                    {
                      "number": 7,
                      "title": "ready issue",
                      "body": "## Problem\\nx\\n\\n## Acceptance Criteria\\n- [ ] [test] y",
                      "url": "https://example.test/issues/7",
                      "labels": {"nodes": [{"name": "autopilot"}]}
                    }
                  ],
                  "pageInfo": {"hasNextPage": true, "endCursor": "cursor-1"}
                }
              }
            }
          },
          {
            "data": {
              "repository": {
                "issues": {
                  "nodes": [
                    {
                      "number": 6,
                      "title": "unready issue",
                      "body": "draft body",
                      "url": "https://example.test/issues/6",
                      "labels": {"nodes": []}
                    }
                  ],
                  "pageInfo": {"hasNextPage": false, "endCursor": null}
                }
              }
            }
          }
        ]
        JSON
        """,
        fn _tmp_dir, args_path ->
          issues = GitHub.eligible_issues("misty-step/bitterblossom")

          assert Enum.map(issues, & &1.number) == [6, 7]
          assert Enum.find(issues, &(&1.number == 6)).body == "draft body"

          args = File.read!(args_path)
          assert String.contains?(args, "api\ngraphql\n")
          assert String.contains?(args, "--paginate\n")
          assert String.contains?(args, "--slurp\n")
          refute String.contains?(args, "--label\n")
        end
      )
    end
  end

  describe "checks_failed?/1 (unit, no CLI)" do
    test "FAILURE among checks → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Tests", "conclusion" => "FAILURE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "all SUCCESS → false" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "SUCCESS", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == false
    end

    test "pending checks with no conclusion → false (not failed yet)" do
      checks = [
        %{"name" => "CI", "conclusion" => nil, "status" => "IN_PROGRESS"},
        %{"name" => "Lint", "conclusion" => nil, "status" => "QUEUED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == false
    end

    test "empty list → false" do
      assert GitHub.evaluate_checks_failed([]) == false
    end

    test "ERROR conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "ERROR", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "CANCELLED conclusion → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "CANCELLED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "null conclusions only → false" do
      checks = [
        %{"name" => nil, "conclusion" => nil, "status" => nil}
      ]

      assert GitHub.evaluate_checks_failed(checks) == false
    end
  end
end
