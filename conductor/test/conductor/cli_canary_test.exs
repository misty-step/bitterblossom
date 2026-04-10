defmodule Conductor.CLICanaryTest do
  use ExUnit.Case, async: false

  import ExUnit.CaptureIO

  alias Conductor.CLI

  @conductor_dir Path.expand("../..", __DIR__)

  defmodule MockCanaryClient do
    def incidents(opts) do
      notify({:incidents_called, opts})

      if error = mock_error(:incidents) do
        {:error, error}
      else
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
    end

    def report(opts) do
      notify({:report_called, opts})

      if error = mock_error(:report) do
        {:error, error}
      else
        {:ok,
         %{
           "status" => "degraded",
           "summary" => "1 service degraded.",
           "incidents" => [%{"id" => "INC-123"}],
           "error_groups" => [%{"group_hash" => "grp-1"}],
           "targets" => [%{"id" => "tgt-1"}]
         }}
      end
    end

    def timeline(opts) do
      notify({:timeline_called, opts})

      if error = mock_error(:timeline) do
        {:error, error}
      else
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
    end

    def incident_annotations(incident_id) do
      notify({:incident_annotations_called, incident_id})

      if error = mock_error(:incident_annotations) do
        {:error, error}
      else
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
    end

    def annotate_incident(incident_id, attrs) do
      notify({:annotate_incident_called, incident_id, attrs})

      if error = mock_error(:annotate_incident) do
        {:error, error}
      else
        {:ok,
         %{
           "created_at" => "2026-04-08T12:02:00Z",
           "agent" => attrs.agent,
           "action" => attrs.action
         }}
      end
    end

    defp notify(message) do
      if pid = Application.get_env(:conductor, :canary_test_pid) do
        send(pid, message)
      end
    end

    defp mock_error(key) do
      Application.get_env(:conductor, :canary_mock_errors, %{}) |> Map.get(key)
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
    aliases = ["volume-web"]
    repo = "misty-step/volume"
    clone_url = "https://git.example.com/misty-step/volume.git"
    default_branch = "main"
    test_cmd = ["make", "test"]
    auto_merge = true
    """)

    original_client = Application.get_env(:conductor, :canary_client_module)
    original_errors = Application.get_env(:conductor, :canary_mock_errors)
    original_test_pid = Application.get_env(:conductor, :canary_test_pid)

    Application.put_env(:conductor, :canary_client_module, MockCanaryClient)
    Application.put_env(:conductor, :canary_mock_errors, %{})
    Application.put_env(:conductor, :canary_test_pid, self())

    on_exit(fn ->
      File.rm(path)

      if original_client,
        do: Application.put_env(:conductor, :canary_client_module, original_client),
        else: Application.delete_env(:conductor, :canary_client_module)

      if original_errors,
        do: Application.put_env(:conductor, :canary_mock_errors, original_errors),
        else: Application.delete_env(:conductor, :canary_mock_errors)

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
    assert decoded["aliases"] == ["volume-web"]
    assert decoded["repo"] == "misty-step/volume"
    assert decoded["clone_url"] == "https://git.example.com/misty-step/volume.git"
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
    assert output =~ "aliases: volume-web"
    assert output =~ "repo: misty-step/volume"
    assert output =~ "clone_url: https://git.example.com/misty-step/volume.git"
    assert output =~ "default_branch: main"
    assert output =~ "test_cmd: make test"
  end

  test "uses the configured default catalog path", %{path: path} do
    original_path = Application.get_env(:conductor, :canary_services_path)

    on_exit(fn ->
      if original_path,
        do: Application.put_env(:conductor, :canary_services_path, original_path),
        else: Application.delete_env(:conductor, :canary_services_path)
    end)

    Application.put_env(:conductor, :canary_services_path, path)

    output =
      capture_io(fn ->
        CLI.main(["canary", "service", "volume", "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert decoded["name"] == "volume"
  end

  test "resolves aliases through the CLI", %{path: path} do
    output =
      capture_io(fn ->
        CLI.main(["canary", "service", "volume-web", "--catalog", path, "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert decoded["name"] == "volume"
    assert decoded["aliases"] == ["volume-web"]
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

  test "prints incidents as json" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "incidents", "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert get_in(decoded, ["incidents", Access.at(0), "id"]) == "INC-123"
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

  test "prints report as json" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "report", "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert decoded["status"] == "degraded"
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

  test "prints timeline as json" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "timeline", "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert get_in(decoded, ["events", Access.at(0), "event"]) == "incident.opened"
  end

  test "forwards timeline cursor pagination options" do
    capture_io(fn ->
      CLI.main(["canary", "timeline", "--cursor", "next-page", "--limit", "200", "--json"])
    end)

    assert_received {:timeline_called, opts}
    assert opts[:cursor] == "next-page"
    assert opts[:limit] == 200
  end

  test "lists incident annotations" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "annotations", "incident", "INC-123"])
      end)

    assert_received {:incident_annotations_called, "INC-123"}
    assert output =~ "tansy bitterblossom.claimed"
  end

  test "lists incident annotations as json" do
    output =
      capture_io(fn ->
        CLI.main(["canary", "annotations", "incident", "INC-123", "--json"])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert get_in(decoded, ["annotations", Access.at(0), "action"]) == "bitterblossom.claimed"
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

  test "creates incident annotations as json" do
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
          "--json"
        ])
      end)

    assert {:ok, decoded} = Jason.decode(output)
    assert decoded["agent"] == "tansy"
    assert decoded["action"] == "bitterblossom.claimed"
  end

  test "fails clearly when Canary credentials are missing for incidents" do
    {output, status} =
      System.cmd("mix", ["conductor", "canary", "incidents"],
        cd: @conductor_dir,
        env: [
          {"MIX_ENV", "test"},
          {"CANARY_ENDPOINT", ""},
          {"CANARY_API_KEY", ""}
        ],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "CANARY_ENDPOINT and CANARY_API_KEY must be set"
  end

  test "validates missing annotation arguments" do
    {output, status} =
      System.cmd(
        "mix",
        ["conductor", "canary", "annotate", "incident", "INC-123", "--agent", "tansy"],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "missing required --action"
  end

  test "validates missing annotation agent" do
    {output, status} =
      System.cmd(
        "mix",
        [
          "conductor",
          "canary",
          "annotate",
          "incident",
          "INC-123",
          "--action",
          "bitterblossom.claimed"
        ],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "missing required --agent"
  end

  test "validates annotation metadata json" do
    {output, status} =
      System.cmd(
        "mix",
        [
          "conductor",
          "canary",
          "annotate",
          "incident",
          "INC-123",
          "--agent",
          "tansy",
          "--action",
          "bitterblossom.claimed",
          "--metadata",
          "[]"
        ],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "--metadata must be a JSON object"
  end

  test "rejects empty incident ids for annotations" do
    {output, status} =
      System.cmd(
        "mix",
        ["conductor", "canary", "annotations", "incident", ""],
        cd: @conductor_dir,
        env: [{"MIX_ENV", "test"}],
        stderr_to_stdout: true
      )

    assert status == 1
    assert output =~ "incident id must not be empty"
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
