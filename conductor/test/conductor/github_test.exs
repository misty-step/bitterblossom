defmodule Conductor.GitHubTest do
  use ExUnit.Case, async: false
  import ExUnit.CaptureLog

  alias Conductor.{GitHub, Issue}

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

  describe "find_open_pr/2" do
    test "matches open PRs on non-factory branches when the branch embeds the issue number" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        [
          {"number":10,"title":"fix","body":"","headRefName":"fix/42-cerberus-permissions","url":"http://example.com/10"},
          {"number":11,"title":"other","body":"","headRefName":"factory/99-1234567890","url":"http://example.com/11"}
        ]
        JSON
        """,
        fn _tmp_dir, _args_path ->
          assert {:ok, %{"number" => 10}} = GitHub.find_open_pr("misty-step/bitterblossom", 42)
        end
      )
    end

    test "matches open PRs on manual branches when the body closes the issue" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        [
          {"number":12,"title":"fix","body":"Closes #42","headRefName":"fix/cerberus-permissions","url":"http://example.com/12"},
          {"number":13,"title":"other","body":"Closes #99","headRefName":"fix/other","url":"http://example.com/13"}
        ]
        JSON
        """,
        fn _tmp_dir, _args_path ->
          assert {:ok, %{"number" => 12}} = GitHub.find_open_pr("misty-step/bitterblossom", 42)
        end
      )
    end

    test "does not match a different issue number that shares a numeric prefix" do
      with_fake_gh(
        """
        printf '%s\n' "$@" > "$GH_ARGS_PATH"
        cat <<'JSON'
        [
          {"number":10,"title":"fix","body":"Closes #420","headRefName":"factory/420-1234567890","url":"http://example.com/10"}
        ]
        JSON
        """,
        fn _tmp_dir, _args_path ->
          assert {:error, :not_found} = GitHub.find_open_pr("misty-step/bitterblossom", 42)
        end
      )
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
        printf '%s\n' "$@" >> "$GH_ARGS_PATH"
        if [[ "$*" == *"&page=1"* ]]; then
          cat <<'JSON'
        [
          {
            "number": 7,
            "title": "ready issue",
            "body": "## Problem\\nx\\n\\n## Acceptance Criteria\\n- [ ] [test] y",
            "url": "https://example.test/issues/7",
            "labels": [{"name": "autopilot"}]
          }
        ]
        JSON
        elif [[ "$*" == *"&page=2"* ]]; then
          cat <<'JSON'
        [
          {
            "number": 6,
            "title": "unready issue",
            "body": "draft body",
            "url": "https://example.test/issues/6",
            "labels": []
          }
        ]
        JSON
        else
          echo '[]'
        fi
        """,
        fn _tmp_dir, args_path ->
          issues = GitHub.eligible_issues("misty-step/bitterblossom")

          assert Enum.map(issues, & &1.number) == [6, 7]
          assert Enum.find(issues, &(&1.number == 6)).body == "draft body"

          args = File.read!(args_path)

          assert String.contains?(
                   args,
                   "api\nrepos/misty-step/bitterblossom/issues?state=open&per_page=100&page=1\n"
                 )

          assert String.contains?(
                   args,
                   "api\nrepos/misty-step/bitterblossom/issues?state=open&per_page=100&page=2\n"
                 )
        end
      )
    end

    test "caps unfiltered pagination at the default issue limit" do
      with_fake_gh(
        """
        printf '%s\n' "$@" >> "$GH_ARGS_PATH"
        page="$(printf '%s' "$*" | sed -n 's/.*page=\\([0-9][0-9]*\\).*/\\1/p')"

        if [[ -z "$page" ]]; then
          echo '[]'
        elif (( page <= 11 )); then
          start=$(( (page - 1) * 100 + 1 ))
          finish=$(( start + 99 ))
          printf '[\n'

          for number in $(seq "$start" "$finish"); do
            comma=","

            if (( number == finish )); then
              comma=""
            fi

            printf '{"number":%s,"title":"issue %s","body":"draft body","url":"https://example.test/issues/%s","labels":[]}%s\n' \
              "$number" "$number" "$number" "$comma"
          done

          printf ']\n'
        else
          echo '[]'
        fi
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, issues} = GitHub.list_issues("misty-step/bitterblossom")
          assert length(issues) == 1000
          assert Enum.at(issues, 0).number == 1
          assert Enum.at(issues, -1).number == 1000

          args = File.read!(args_path)
          refute String.contains?(args, "page=11")
        end
      )
    end

    test "continues past the initial page budget when mixed issue pages still retain backlog items" do
      with_fake_gh(
        """
        printf '%s\n' "$@" >> "$GH_ARGS_PATH"
        page="$(printf '%s' "$*" | sed -n 's/.*page=\\([0-9][0-9]*\\).*/\\1/p')"

        if [[ -z "$page" ]]; then
          echo '[]'
        elif (( page <= 10 )); then
          start=$(( (page - 1) * 90 + 1 ))
          finish=$(( start + 89 ))
          printf '[\n'

          first=1

          for number in $(seq "$start" "$finish"); do
            if (( first == 0 )); then
              printf ',\n'
            fi

            first=0

            printf '{"number":%s,"title":"issue %s","body":"draft body","url":"https://example.test/issues/%s","labels":[]}' \
              "$number" "$number" "$number"
          done

          for pr_number in $(seq 1 10); do
            printf ',\n{"number":%s,"title":"not an issue","body":"","url":"https://example.test/pull/%s","labels":[],"pull_request":{"url":"https://example.test/pull/%s"}}' \
              "$(( page * 1000 + pr_number ))" "$(( page * 1000 + pr_number ))" "$(( page * 1000 + pr_number ))"
          done

          printf '\n]\n'
        elif (( page == 11 )); then
          start=901
          finish=1000
          printf '[\n'

          first=1

          for number in $(seq "$start" "$finish"); do
            if (( first == 0 )); then
              printf ',\n'
            fi

            first=0

            printf '{"number":%s,"title":"issue %s","body":"draft body","url":"https://example.test/issues/%s","labels":[]}' \
              "$number" "$number" "$number"
          done

          printf '\n]\n'
        else
          echo '[]'
        fi
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, issues} = GitHub.list_issues("misty-step/bitterblossom")
          assert length(issues) == 1000
          assert Enum.at(issues, 0).number == 1
          assert Enum.at(issues, -1).number == 1000

          args = File.read!(args_path)
          assert String.contains?(args, "page=11")
          refute String.contains?(args, "page=12")
        end
      )
    end

    test "resets the PR-only page budget after a retained issue page" do
      with_fake_gh(
        """
        printf '%s\n' "$@" >> "$GH_ARGS_PATH"
        page="$(printf '%s' "$*" | sed -n 's/.*page=\\([0-9][0-9]*\\).*/\\1/p')"

        if [[ -z "$page" ]]; then
          echo '[]'
        elif (( page == 1 )); then
          cat <<'JSON'
        [
          {
            "number": 1,
            "title": "first issue",
            "body": "draft body",
            "url": "https://example.test/issues/1",
            "labels": []
          }
        ]
        JSON
        elif (( page == 11 )); then
          cat <<'JSON'
        [
          {
            "number": 2,
            "title": "middle issue",
            "body": "draft body",
            "url": "https://example.test/issues/2",
            "labels": []
          }
        ]
        JSON
        elif (( page == 21 )); then
          cat <<'JSON'
        [
          {
            "number": 3,
            "title": "late issue",
            "body": "draft body",
            "url": "https://example.test/issues/3",
            "labels": []
          }
        ]
        JSON
        elif (( page < 21 )); then
          cat <<'JSON'
        [
          {
            "number": 9999,
            "title": "not an issue",
            "body": "",
            "url": "https://example.test/pull/9999",
            "labels": [],
            "pull_request": {"url": "https://example.test/pull/9999"}
          }
        ]
        JSON
        else
          echo '[]'
        fi
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, issues} = GitHub.list_issues("misty-step/bitterblossom")
          assert Enum.map(issues, & &1.number) == [1, 2, 3]

          args = File.read!(args_path)
          assert String.contains?(args, "page=11")
          assert String.contains?(args, "page=21")
          assert String.contains?(args, "page=22")
          refute String.contains?(args, "page=23")
        end
      )
    end

    test "stops unfiltered pagination after the default page budget even when pages only contain pull requests" do
      with_fake_gh(
        """
        printf '%s\n' "$@" >> "$GH_ARGS_PATH"
        page="$(printf '%s' "$*" | sed -n 's/.*page=\\([0-9][0-9]*\\).*/\\1/p')"

        if [[ -z "$page" ]]; then
          echo '[]'
        elif (( page <= 10 )); then
          cat <<JSON
        [
          {
            "number": $(( page + 100 )),
            "title": "not an issue",
            "body": "",
            "url": "https://example.test/pull/$(( page + 100 ))",
            "labels": [],
            "pull_request": {"url": "https://example.test/pull/$(( page + 100 ))"}
          }
        ]
        JSON
        else
          echo '[]'
        fi
        """,
        fn _tmp_dir, args_path ->
          assert {:ok, []} = GitHub.list_issues("misty-step/bitterblossom")

          args = File.read!(args_path)
          assert String.contains?(args, "page=1")
          assert String.contains?(args, "page=10")
          refute String.contains?(args, "page=11")
        end
      )
    end

    test "eligible_issues returns both ready and unready issues sorted by number" do
      with_fake_gh(
        """
        cat <<'JSON'
        [
          {
            "number": 20,
            "title": "ready issue",
            "body": "## Problem\\nx\\n\\n## Acceptance Criteria\\n- [ ] [test] y",
            "url": "https://example.test/issues/20",
            "labels": []
          },
          {
            "number": 10,
            "title": "unready issue",
            "body": "draft body",
            "url": "https://example.test/issues/10",
            "labels": []
          }
        ]
        JSON
        """,
        fn _tmp_dir, _args_path ->
          issues =
            GitHub.eligible_issues(
              "misty-step/bitterblossom",
              label: "autopilot",
              limit: 25
            )

          assert Enum.map(issues, & &1.number) == [10, 20]
          assert Enum.find(issues, &(&1.number == 10)).body == "draft body"
        end
      )
    end

    test "filters pull requests from paginated issue fetches" do
      with_fake_gh(
        """
        if [[ "$*" == *"&page=1"* ]]; then
          cat <<'JSON'
        [
          {
            "number": 8,
            "title": "real issue",
            "body": "draft body",
            "url": "https://example.test/issues/8",
            "labels": []
          },
          {
            "number": 99,
            "title": "not an issue",
            "body": "",
            "url": "https://example.test/pull/99",
            "labels": [],
            "pull_request": {"url": "https://example.test/pull/99"}
          }
        ]
        JSON
        else
          echo '[]'
        fi
        """,
        fn _tmp_dir, _args_path ->
          assert {:ok, [%Issue{number: 8}]} = GitHub.list_issues("misty-step/bitterblossom")
        end
      )
    end

    test "returns an error when a paginated issue page is malformed" do
      with_fake_gh(
        """
        echo '{"not":"a list"}'
        """,
        fn _tmp_dir, _args_path ->
          assert {:error, message} = GitHub.list_issues("misty-step/bitterblossom")
          assert message =~ "invalid JSON from gh"
        end
      )
    end

    test "returns an error instead of raising when the repo is malformed" do
      assert {:error, message} = GitHub.list_issues("not-a-repo")
      assert message =~ "expected repo in owner/name format"
    end

    test "rejects a repo with extra path segments" do
      assert {:error, message} = GitHub.list_issues("owner/name/extra")
      assert message =~ "expected repo in owner/name format"
    end

    test "rejects malformed repo strings that rely on trimmed segments" do
      for repo <- ["/owner/name/", "owner//name"] do
        assert {:error, message} = GitHub.list_issues(repo)
        assert message =~ "expected repo in owner/name format"
      end
    end

    test "eligible_issues logs and returns an empty list when the repo is malformed" do
      assert capture_log(fn ->
               assert GitHub.eligible_issues("still-not-a-repo") == []
             end) =~ "failed to list issues"
    end
  end

  describe "operator directive normalization" do
    test "label_present?/2 treats nil labels as empty" do
      refute GitHub.label_present?(%{"labels" => nil}, "hold")
    end

    test "label_present?/2 finds matching label names" do
      assert GitHub.label_present?(%{"labels" => [%{"name" => "hold"}]}, "hold")
    end

    test "normalize_issue_comments/1 treats nil comments as empty" do
      assert GitHub.normalize_issue_comments(nil) == []
    end

    test "normalize_issue_comments/1 keeps binary comment bodies" do
      assert GitHub.normalize_issue_comments([%{"body" => "bb: cancel"}]) == [
               %{"body" => "bb: cancel"}
             ]
    end

    test "normalize_issue_comments/1 extracts nested text bodies" do
      assert GitHub.normalize_issue_comments([%{"body" => %{"text" => "bb: cancel"}}]) == [
               %{"body" => "bb: cancel"}
             ]
    end

    test "normalize_issue_comments/1 extracts nested body bodies" do
      assert GitHub.normalize_issue_comments([%{"body" => %{"body" => "bb: cancel"}}]) == [
               %{"body" => "bb: cancel"}
             ]
    end
  end

  describe "pr_state/2" do
    test "closed + merged returns MERGED" do
      with_fake_gh(
        ~S|echo '{"number":1,"title":"t","state":"closed","merged":true,"mergeable":"","headRefName":"b","url":"u"}'|,
        fn _tmp_dir, _args_path ->
          assert {:ok, "MERGED"} = GitHub.pr_state("owner/repo", 1)
        end
      )
    end

    test "closed + not merged returns CLOSED" do
      with_fake_gh(
        ~S|echo '{"number":1,"title":"t","state":"closed","merged":false,"mergeable":"","headRefName":"b","url":"u"}'|,
        fn _tmp_dir, _args_path ->
          assert {:ok, "CLOSED"} = GitHub.pr_state("owner/repo", 1)
        end
      )
    end

    test "open returns OPEN (upcased from lowercase)" do
      with_fake_gh(
        ~S|echo '{"number":1,"title":"t","state":"open","merged":false,"mergeable":"","headRefName":"b","url":"u"}'|,
        fn _tmp_dir, _args_path ->
          assert {:ok, "OPEN"} = GitHub.pr_state("owner/repo", 1)
        end
      )
    end

    test "error passthrough" do
      with_fake_gh(
        "exit 1",
        fn _tmp_dir, _args_path ->
          assert {:error, _} = GitHub.pr_state("owner/repo", 1)
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

    test "STARTUP_FAILURE among passing checks → true (Cerberus bootstrap failure)" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{
          "name" => "review / Cerberus · preflight",
          "conclusion" => "STARTUP_FAILURE",
          "status" => "COMPLETED"
        }
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "TIMED_OUT among passing checks → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => "TIMED_OUT", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "STALE among passing checks → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "STALE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end

    test "ACTION_REQUIRED among passing checks → true" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Review", "conclusion" => "ACTION_REQUIRED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks_failed(checks) == true
    end
  end

  describe "evaluate_checks/1 — workflow bootstrap failure modes" do
    test "STARTUP_FAILURE blocks merge (non-green)" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{
          "name" => "review / Cerberus · preflight",
          "conclusion" => "STARTUP_FAILURE",
          "status" => "COMPLETED"
        }
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "TIMED_OUT blocks merge" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Deploy", "conclusion" => "TIMED_OUT", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "STALE blocks merge among passing checks" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Lint", "conclusion" => "STALE", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end

    test "ACTION_REQUIRED blocks merge among passing checks" do
      checks = [
        %{"name" => "CI", "conclusion" => "SUCCESS", "status" => "COMPLETED"},
        %{"name" => "Review", "conclusion" => "ACTION_REQUIRED", "status" => "COMPLETED"}
      ]

      assert GitHub.evaluate_checks(checks) == false
    end
  end
end
