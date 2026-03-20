defmodule Conductor.RetroTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO
  import Conductor.TestSupport.ProcessHelpers

  alias Conductor.{CLI, Retro, Store}

  setup do
    db_path = Path.join(System.tmp_dir!(), "retro_test_#{System.unique_integer([:positive])}.db")

    event_log =
      Path.join(System.tmp_dir!(), "retro_test_#{System.unique_integer([:positive])}.jsonl")

    orig_db = Application.get_env(:conductor, :db_path)
    orig_log = Application.get_env(:conductor, :event_log)

    Application.stop(:conductor)
    Application.put_env(:conductor, :db_path, db_path)
    Application.put_env(:conductor, :event_log, event_log)
    Application.ensure_all_started(:conductor)

    on_exit(fn ->
      Application.stop(:conductor)
      restore_env(:db_path, orig_db)
      restore_env(:event_log, orig_log)
      Application.ensure_all_started(:conductor)
      File.rm(db_path)
      File.rm(event_log)
    end)

    :ok
  end

  describe "analyze/1" do
    test "returns :ok without crashing when retro is disabled" do
      assert Retro.analyze("run-999-0000000000") == :ok
    end
  end

  describe "retro_complete persistence" do
    test "list_events includes retro summary and action count" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 646,
          issue_title: "Persist retro analysis",
          builder_sprite: "sprite-1"
        })

      :ok =
        Retro.record_complete_event(run_id, %{
          "summary" => "builder hit review friction; backlog updated",
          "findings" => [
            %{
              "title" => "Missing reviewer guidance",
              "action" => "create_issue",
              "existing_issue" => nil
            },
            %{
              "title" => "Low-value note",
              "action" => "none",
              "existing_issue" => "#615"
            },
            %{
              "title" => "Unsupported action",
              "action" => "surprise_me",
              "existing_issue" => nil
            }
          ]
        })

      [event] = Store.list_events(run_id)

      assert event["event_type"] == "retro_complete"
      assert event["payload"]["summary"] == "builder hit review friction; backlog updated"
      assert event["payload"]["action_count"] == 1
      assert event["payload"]["skipped_count"] == 2

      assert event["payload"]["actions_taken"] == [
               %{
                 "action" => "create_issue",
                 "existing_issue" => nil,
                 "title" => "Missing reviewer guidance"
               }
             ]
    end

    test "show-events exposes retro_complete event payload" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 647,
          issue_title: "Show retro event",
          builder_sprite: "sprite-1"
        })

      :ok =
        Retro.record_complete_event(run_id, %{
          "summary" => "clean run",
          "findings" => []
        })

      output =
        capture_io(fn ->
          CLI.main(["show-events", "--run-id", run_id])
        end)

      decoded = Jason.decode!(output)
      assert decoded["run_id"] == run_id
      assert decoded["event_count"] == 1

      [event] = decoded["events"]
      assert event["event_type"] == "retro_complete"
      assert event["payload"]["summary"] == "clean run"
      assert event["payload"]["action_count"] == 0
    end

    test "persists retro_complete even when action execution raises" do
      {:ok, run_id} =
        Store.create_run(%{
          repo: "test/repo",
          issue_number: 648,
          issue_title: "Persist failed retro",
          builder_sprite: "sprite-1"
        })

      {:ok, run} = Store.get_run(run_id)

      analysis = %{
        "summary" => "retro action failed",
        "findings" => [
          %{
            "title" => "Actionable failure",
            "action" => "create_issue",
            "existing_issue" => nil
          }
        ]
      }

      assert_raise RuntimeError, "boom", fn ->
        Retro.finalize_analysis(run_id, analysis, run, fn _, _ -> raise "boom" end)
      end

      [event] = Store.list_events(run_id)
      assert event["event_type"] == "retro_complete"
      assert event["payload"]["summary"] == "retro action failed"
      assert event["payload"]["action_count"] == 1
    end
  end
end
