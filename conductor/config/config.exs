import Config

config :conductor,
  db_path: ".bb/conductor.db",
  event_log: ".bb/events.jsonl",
  poll_seconds: 60,
  builder_timeout_minutes: 25,
  ci_timeout_minutes: 30,
  pr_minimum_age_seconds: 300,
  max_concurrent_runs: 2,
  start_dashboard: true,
  persona_source_root: Path.expand("../../sprites", __DIR__)

config :conductor, Conductor.Web.Endpoint,
  adapter: Bandit.PhoenixAdapter,
  http: [ip: {127, 0, 0, 1}, port: 4000],
  secret_key_base:
    System.get_env("DASHBOARD_SECRET_KEY_BASE") ||
      "bitterblossom-dashboard-dev-key-must-be-at-least-64-chars-long-x",
  live_view: [signing_salt: "bb_lv_salt"],
  server: true

config :logger,
  level: :info

if File.exists?(Path.expand("#{config_env()}.exs", __DIR__)) do
  import_config "#{config_env()}.exs"
end
