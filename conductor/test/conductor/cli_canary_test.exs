defmodule Conductor.CLICanaryTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO

  alias Conductor.CLI

  @conductor_dir Path.expand("../..", __DIR__)

  defmodule MockCanaryClient do
    def incidents(opts) do
      notify({:incidents_called, opts})

      {:ok,
       %{
         "incidents" => [
           %{
             "id" => "INC-123",
             "service" => "volume",
             "severity" => "high",
             "state" => "investigating",
             "title" => "Volume is failing health checks"
           }
         ]
       }}
    end

    def report(opts) do
      notify({:report_called, opts})

      {:ok,
       %{
         "status" => "degraded",
         "summary" => "1 service degraded.",
         "incidents" => [%{"id" => "INC-123"}],
         "error_groups" => [%{"group_hash" => "grp-1"}],
         "targets" => [%{"id" => "tgt-1"}]
       }}
    end

    def timeline(opts) do
      notify({:timeline_called, opts})

      {:ok,
       %{
         "summary" => "2 recent events.",
         "events" => [
           %{
             "created_at" => "2026-04-08T12:00:00Z",
             "service" => "volume",
             "event" => "incident.opened",
             "severity" => "high",
             "summary" => "Incident opened for volume."
           }
         ]
       }}
    end

    def incident_annotations(incident_id) do
      notify({:incident_annotations_called, incident_id})

      {:ok,
       %{
         "annotations" => [
           %{
             "created_at" => "2026-04-08T12:01:00Z",
             "agent" => "tansy",
             "action" => "bitterblossom.claimed"
           }
         ]
       }}
    end

    def annotate_incident(incident_id, attrs) do
      notify({:annotate_incident_called, incident_id, attrs})

      {:ok,
       %{
         "created_at" => "2026-04-08T12:02:00Z",
         "agent" => attrs.agent,
         "action" => attrs.action
       }}
    end

    defp notify(message) do
      if pid = Application.get_env(:conductor, :canary_test_pid) do
        send(pid, message)
      end
    end
  end

  setup do
    path =
      Path.join(
        System.tmp_dir!(),
        "canary-services-cli-test-#{System.unique_integer([:positive])}.toml"
      )

    File.write!(path, """
    [[service]]
    name = "volume"
    repo = "misty-step/volume"
    default_branch = "main"
    test_cmd = ["make", "test"]
    auto_merge = true
    """)

    original_client = Application.get_env(:conductor, :canary_client_module)
    original_test_pid = Application.get_env(:conductor, :canary_test_pid)

    Application.put_env(:conductor, :canary_client_module, MockCanaryClient)
    Application.put_env(:conductor, :canary_test_pid, self())

    on_exit(fn ->
      File.rm(path)

      if original_client,
        do: Application.put_env(:conductor, :canary_client_module, original_client),
        else: Application.delete_env(:conductor, :canary_client_module)

      if original_test_pid,
        do: Application.put_env(:conductor, :canary_test_pid, original_test_pid),
        else: Application.delete_env(:conductor, :canary_test_pid)
    end)

    %{path: path}
  end

  test "prints a resolved service as json", %{path: path} do
    output =
      capture_io(fn ->
        CLI.main(["canary", "service", "volume", "--catalog", path, "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert decoded["name"] == "volume"
    assert decoded["repo"] == "misty-step/volume"
    assert decoded["default_branch"] == "main"
    assert decoded["test_cmd"] == ["make", "test"]
    assert decoded["auto_merge"] == true
    assert decoded["auto_deploy"] == false
  end

  test "prints a readable service summary without --json", %{path: path} do
    output =
      capture_io(fn ->
        CLI.main(["canary", "service", "volume", "--catalog", path])
      end)

    assert output =~ "service: volume"
    assert output =~ "repo: misty-step/volume"
    assert output =~ "default_branch: main"
    assert output =~ "test_cmd: make test"
  end

  test "prints incidents and forwards annotation filters" do
    output =
      capture_io(fn ->
        CLI.main([
          "canary",
          "incidents",
          "--without-annotation",
          "bitterblossom.claimed"
        ])
      end)

    assert_received {:incidents_called, opts}
    assert opts[:without_annotation] == "bitterblossom.claimed"
    assert output =~ "INC-123 volume high investigating Volume is failing health checks"
  end

  test "prints report summary and forwards query options" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "report", "--window", "24h", "--limit", "5"])
      end)

    assert_received {:report_called, opts}
    assert opts[:window] == "24h"
    assert opts[:limit] == 5
    assert output =~ "status: degraded"
    assert output =~ "summary: 1 service degraded."
  end

  test "prints timeline summary and events" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "timeline", "--service", "volume", "--window", "24h"])
      end)

    assert_received {:timeline_called, opts}
    assert opts[:service] == "volume"
    assert opts[:window] == "24h"
    assert output =~ "summary: 2 recent events."
    assert output =~ "incident.opened"
  end

  test "lists incident annotations" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "annotations", "incident", "INC-123"])
      end)

    assert_received {:incident_annotations_called, "INC-123"}
    assert output =~ "tansy bitterblossom.claimed"
  end

  test "creates incident annotations from json metadata" do
    output =
      capture_io(fn ->
        CLI.main([
          "canary",
          "annotate",
          "incident",
          "INC-123",
          "--agent",
          "tansy",
          "--action",
          "bitterblossom.claimed",
          "--metadata",
          ~s({"service":"volume"})
        ])
      end)

    assert_received {:annotate_incident_called, "INC-123", attrs}
    assert attrs.agent == "tansy"
    assert attrs.action == "bitterblossom.claimed"
    assert attrs.metadata == %{"service" => "volume"}
    assert output =~ "tansy bitterblossom.claimed"
  end

  test "prints usage on missing canary subcommands" do
    {output, status} =
      System.cmd("mix", ["conductor", "canary"],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1

    assert output =~
             "usage: bitterblossom canary <service|incidents|report|timeline|annotations|annotate> ..."
  end

  test "fails clearly for unknown services", %{path: path} do
    {output, status} =
      System.cmd("mix", ["conductor", "canary", "service", "missing", "--catalog", path],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "unknown Canary service: missing"
  end
end
