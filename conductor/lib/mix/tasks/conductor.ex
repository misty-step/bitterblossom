defmodule Mix.Tasks.Conductor do
  @moduledoc "Run conductor commands via Mix."
  @shortdoc "Bitterblossom conductor"

  use Mix.Task

  @impl Mix.Task
  def run(args) do
    Mix.Task.run("app.start")
    Conductor.CLI.main(args)
  end
end
