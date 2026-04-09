defmodule Conductor.MixProject do
  use Mix.Project

  def project do
    [
      app: :conductor,
      version: "0.1.0",
      elixir: "~> 1.16",
      start_permanent: Mix.env() == :prod,
      escript: [main_module: Conductor.CLI],
      deps: deps()
    ]
  end

  def application do
    [
      extra_applications: [:logger],
      mod: {Conductor.Application, []}
    ]
  end

  defp deps do
    [
      {:exqlite, "~> 0.27"},
      {:jason, "~> 1.4"},
      {:req, "~> 0.5"},
      {:phoenix, "~> 1.7"},
      {:phoenix_live_view, "~> 1.0"},
      {:phoenix_html, "~> 4.1"},
      {:phoenix_pubsub, "~> 2.1"},
      {:bandit, "~> 1.5"},
      {:plug_crypto, "~> 2.1"},
      {:toml, "~> 0.7"},
      {:canary_sdk, github: "misty-step/canary", sparse: "canary_sdk", ref: "91a430c"},
      {:lazy_html, ">= 0.1.0", only: :test}
    ]
  end
end
