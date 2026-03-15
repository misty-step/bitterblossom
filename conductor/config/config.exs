import Config

config :conductor,
  db_path: ".bb/conductor.db",
  event_log: ".bb/events.jsonl",
  poll_seconds: 60,
  builder_timeout_minutes: 25,
  ci_timeout_minutes: 15,
  pr_minimum_age_seconds: 300,
  max_concurrent_runs: 2,
  fleet: [
    %{name: "bb-builder", role: :builder, org: "misty-step"},
    %{name: "bb-fixer", role: :fixer, org: "misty-step"},
    %{name: "bb-polisher", role: :polisher, org: "misty-step"}
  ]

config :conductor, Conductor.Web.Endpoint,
  adapter: Bandit.PhoenixAdapter,
  http: [ip: {127, 0, 0, 1}, port: 4000],
  secret_key_base:
    System.get_env("DASHBOARD_SECRET_KEY_BASE") ||
      "bitterblossom-dashboard-dev-key-must-be-at-least-64-chars-long-x",
  live_view: [signing_salt: "bb_lv_salt"],
  server: false

config :logger,
  level: :info
