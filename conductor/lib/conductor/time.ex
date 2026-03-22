defmodule Conductor.Time do
  @moduledoc false

  @spec now_utc() :: binary()
  def now_utc do
    DateTime.utc_now() |> DateTime.to_iso8601()
  end
end
