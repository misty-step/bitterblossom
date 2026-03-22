defmodule Conductor.TimeTest do
  use ExUnit.Case, async: true

  test "now_utc returns an ISO8601 UTC timestamp" do
    timestamp = Conductor.Time.now_utc()

    assert {:ok, datetime, 0} = DateTime.from_iso8601(timestamp)
    assert datetime.time_zone == "Etc/UTC"
  end
end
