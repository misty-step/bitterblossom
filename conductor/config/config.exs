import Config

config :conductor,
  db_path: ".bb/conductor.db",
  event_log: ".bb/events.jsonl",
  poll_seconds: 60,
  builder_timeout_minutes: 25,
  ci_timeout_minutes: 15,
  pr_minimum_age_seconds: 300,
  max_concurrent_runs: 2

config :logger,
  level: :info
