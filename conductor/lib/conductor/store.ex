defmodule Conductor.Store do
  @moduledoc """
  Durable run and event persistence over SQLite.

  Deep module: all SQL is hidden. Callers see only Elixir maps and tuples.
  Serializes all writes through a GenServer to avoid SQLite concurrency issues.
  """

  use GenServer
  require Logger

  @valid_columns ~w(phase status branch pr_number pr_url turn_count worktree_path
                    replay_count builder_sprite heartbeat_at completed_at
                    ci_wait_started_at ci_last_reported_at blocked_reason
                    dispatch_attempt_count builder_failure_class builder_failure_reason)

  @doc "Validate that all map keys are in the column allowlist."
  @spec validate_columns(map()) :: :ok | {:error, :invalid_column}
  def validate_columns(attrs) do
    keys = Enum.map(Map.keys(attrs), &to_string/1)

    if Enum.all?(keys, &(&1 in @valid_columns)) do
      :ok
    else
      {:error, :invalid_column}
    end
  end

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec create_run(map()) :: {:ok, binary()}
  def create_run(attrs) do
    GenServer.call(__MODULE__, {:create_run, attrs})
  end

  @spec update_run(binary(), map()) :: :ok | {:error, :invalid_column | :empty_attrs}
  def update_run(run_id, attrs) do
    GenServer.call(__MODULE__, {:update_run, run_id, attrs})
  end

  @doc "Find a run by repo and PR number."
  @spec find_run_by_pr(binary(), pos_integer()) :: {:ok, map()} | {:error, term()}
  def find_run_by_pr(repo, pr_number) do
    GenServer.call(__MODULE__, {:find_run_by_pr, repo, pr_number})
  end

  @spec complete_run(binary(), binary(), binary()) :: :ok
  def complete_run(run_id, phase, status) do
    GenServer.call(__MODULE__, {:complete_run, run_id, phase, status})
  end

  @doc "Atomically complete a run and release its lease. Prevents the class of bug where one is called without the other."
  @spec terminate_run(binary(), binary(), binary(), binary(), pos_integer()) :: :ok
  def terminate_run(run_id, phase, status, repo, issue_number) do
    GenServer.call(__MODULE__, {:terminate_run, run_id, phase, status, repo, issue_number})
  end

  @spec heartbeat_run(binary()) :: :ok
  def heartbeat_run(run_id) do
    GenServer.call(__MODULE__, {:heartbeat_run, run_id})
  end

  @spec get_run(binary()) :: {:ok, map()} | {:error, :not_found}
  def get_run(run_id) do
    GenServer.call(__MODULE__, {:get_run, run_id})
  end

  @spec list_runs(keyword()) :: [map()]
  def list_runs(opts \\ []) do
    GenServer.call(__MODULE__, {:list_runs, opts})
  end

  @spec acquire_lease(binary(), pos_integer(), binary()) :: :ok | {:error, :already_leased}
  def acquire_lease(repo, issue_number, run_id) do
    GenServer.call(__MODULE__, {:acquire_lease, repo, issue_number, run_id})
  end

  @spec release_lease(binary(), pos_integer()) :: :ok
  def release_lease(repo, issue_number) do
    GenServer.call(__MODULE__, {:release_lease, repo, issue_number})
  end

  @spec leased?(binary(), pos_integer()) :: boolean()
  def leased?(repo, issue_number) do
    GenServer.call(__MODULE__, {:leased?, repo, issue_number})
  end

  @spec record_event(binary(), binary(), map()) :: :ok
  def record_event(run_id, event_type, payload \\ %{}) do
    GenServer.call(__MODULE__, {:record_event, run_id, event_type, payload})
  end

  @spec list_events(binary()) :: [map()]
  def list_events(run_id) do
    GenServer.call(__MODULE__, {:list_events, run_id})
  end

  @spec record_incident(binary(), map()) :: :ok
  def record_incident(run_id, attrs) do
    GenServer.call(__MODULE__, {:record_incident, run_id, attrs})
  end

  @spec list_incidents(binary()) :: [map()]
  def list_incidents(run_id) do
    GenServer.call(__MODULE__, {:list_incidents, run_id})
  end

  @spec record_waiver(binary(), map()) :: :ok
  def record_waiver(run_id, attrs) do
    GenServer.call(__MODULE__, {:record_waiver, run_id, attrs})
  end

  @spec list_waivers(binary()) :: [map()]
  def list_waivers(run_id) do
    GenServer.call(__MODULE__, {:list_waivers, run_id})
  end

  @spec mark_semantic_ready(binary()) :: :ok
  def mark_semantic_ready(run_id) do
    GenServer.call(__MODULE__, {:mark_semantic_ready, run_id})
  end

  @doc "Persist whether new run dispatch is paused."
  @spec set_dispatch_paused(boolean()) :: :ok
  def set_dispatch_paused(paused?) do
    GenServer.call(__MODULE__, {:set_dispatch_paused, paused?})
  end

  @doc "Return true when new run dispatch is paused."
  @spec dispatch_paused?() :: boolean()
  def dispatch_paused? do
    GenServer.call(__MODULE__, :dispatch_paused?)
  end

  @doc "List non-terminal runs for a repo (completed_at IS NULL)."
  @spec list_active_runs(binary()) :: [map()]
  def list_active_runs(repo) do
    GenServer.call(__MODULE__, {:list_active_runs, repo})
  end

  @doc "Leases held past run completion: process exited, lease persists, awaiting resolution."
  @spec list_held_leases(binary()) :: [map()]
  def list_held_leases(repo) do
    GenServer.call(__MODULE__, {:list_held_leases, repo})
  end

  @doc """
  Atomically expire a stale run: record event, complete the run as failed,
  and release its lease. Encapsulates the domain transition so callers
  only express intent.
  """
  @spec expire_stale_run(binary(), binary(), pos_integer(), binary() | nil) :: :ok
  def expire_stale_run(repo, run_id, issue_number, heartbeat_at) do
    GenServer.call(__MODULE__, {:expire_stale_run, repo, run_id, issue_number, heartbeat_at})
  end

  # --- GenServer Callbacks ---

  @impl true
  def init(opts) do
    db_path = Keyword.get(opts, :db_path, Conductor.Config.db_path())
    event_log = Keyword.get(opts, :event_log, Conductor.Config.event_log_path())

    File.mkdir_p!(Path.dirname(db_path))

    {:ok, conn} = Exqlite.Sqlite3.open(db_path)
    create_tables(conn)

    {:ok, %{conn: conn, event_log: event_log}}
  end

  @impl true
  def handle_call({:create_run, attrs}, _from, state) do
    run_id = attrs[:run_id] || generate_run_id(attrs[:issue_number])
    now = now_utc()

    exec(
      state.conn,
      """
        INSERT INTO runs (run_id, repo, issue_number, issue_title, phase, status,
                          builder_sprite, picked_at, heartbeat_at, updated_at)
        VALUES (?1, ?2, ?3, ?4, 'pending', 'pending', ?5, ?6, ?6, ?6)
      """,
      [
        run_id,
        attrs[:repo],
        attrs[:issue_number],
        attrs[:issue_title],
        attrs[:builder_sprite],
        now
      ]
    )

    broadcast_update()
    {:reply, {:ok, run_id}, state}
  end

  @impl true
  def handle_call({:update_run, run_id, attrs}, _from, state) do
    case {map_size(attrs), validate_columns(attrs)} do
      {0, _} ->
        {:reply, {:error, :empty_attrs}, state}

      {_, :ok} ->
        sets = Enum.map_join(attrs, ", ", fn {k, _} -> "#{k} = ?" end)
        vals = Map.values(attrs) ++ [now_utc(), run_id]

        exec(
          state.conn,
          "UPDATE runs SET #{sets}, updated_at = ? WHERE run_id = ?",
          vals
        )

        broadcast_update()
        {:reply, :ok, state}

      {_, {:error, :invalid_column}} ->
        Logger.error("update_run rejected: invalid column in #{inspect(Map.keys(attrs))}")
        {:reply, {:error, :invalid_column}, state}
    end
  end

  @impl true
  def handle_call({:find_run_by_pr, repo, pr_number}, _from, state) do
    result =
      try do
        rows =
          query_all(
            state.conn,
            "SELECT * FROM runs WHERE repo = ?1 AND pr_number = ?2 ORDER BY picked_at DESC LIMIT 1",
            [repo, pr_number]
          )

        case rows do
          [run | _] -> {:ok, run}
          [] -> {:error, :not_found}
        end
      rescue
        error ->
          {:error, {:db_error, Exception.message(error)}}
      catch
        :exit, reason ->
          {:error, reason}
      end

    {:reply, result, state}
  end

  @impl true
  def handle_call({:complete_run, run_id, phase, status}, _from, state) do
    now = now_utc()

    exec(
      state.conn,
      """
        UPDATE runs SET phase = ?1, status = ?2, completed_at = ?3, updated_at = ?3
        WHERE run_id = ?4
      """,
      [phase, status, now, run_id]
    )

    broadcast_update()
    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:terminate_run, run_id, phase, status, repo, issue_number}, _from, state) do
    now = now_utc()

    exec(state.conn, "BEGIN IMMEDIATE", [])

    exec(
      state.conn,
      "UPDATE runs SET phase = ?1, status = ?2, completed_at = ?3, updated_at = ?3 WHERE run_id = ?4",
      [phase, status, now, run_id]
    )

    exec(
      state.conn,
      "UPDATE leases SET released_at = ?1 WHERE repo = ?2 AND issue_number = ?3 AND released_at IS NULL",
      [now, repo, issue_number]
    )

    exec(state.conn, "COMMIT", [])

    broadcast_update()
    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:heartbeat_run, run_id}, _from, state) do
    exec(state.conn, "UPDATE runs SET heartbeat_at = ?1 WHERE run_id = ?2", [now_utc(), run_id])
    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:get_run, run_id}, _from, state) do
    case query_one(state.conn, "SELECT * FROM runs WHERE run_id = ?1", [run_id]) do
      nil -> {:reply, {:error, :not_found}, state}
      row -> {:reply, {:ok, row}, state}
    end
  end

  @impl true
  def handle_call({:list_runs, opts}, _from, state) do
    limit = Keyword.get(opts, :limit, 20)
    repo = Keyword.get(opts, :repo)
    issue_number = Keyword.get(opts, :issue_number)

    {sql, params} =
      case {repo, issue_number} do
        {r, i} when is_binary(r) and is_integer(i) ->
          {"SELECT * FROM runs WHERE repo = ?1 AND issue_number = ?2 ORDER BY picked_at DESC LIMIT ?3",
           [r, i, limit]}

        _ ->
          {"SELECT * FROM runs ORDER BY picked_at DESC LIMIT ?1", [limit]}
      end

    {:reply, query_all(state.conn, sql, params), state}
  end

  @impl true
  def handle_call({:acquire_lease, repo, issue_number, run_id}, _from, state) do
    active =
      query_one(
        state.conn,
        "SELECT run_id FROM leases WHERE repo = ?1 AND issue_number = ?2 AND released_at IS NULL",
        [repo, issue_number]
      )

    if active do
      {:reply, {:error, :already_leased}, state}
    else
      exec(
        state.conn,
        "INSERT INTO leases (repo, issue_number, run_id, acquired_at) VALUES (?1, ?2, ?3, ?4)",
        [repo, issue_number, run_id, now_utc()]
      )

      {:reply, :ok, state}
    end
  end

  @impl true
  def handle_call({:release_lease, repo, issue_number}, _from, state) do
    exec(
      state.conn,
      "UPDATE leases SET released_at = ?1 WHERE repo = ?2 AND issue_number = ?3 AND released_at IS NULL",
      [now_utc(), repo, issue_number]
    )

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:leased?, repo, issue_number}, _from, state) do
    row =
      query_one(
        state.conn,
        "SELECT 1 FROM leases WHERE repo = ?1 AND issue_number = ?2 AND released_at IS NULL",
        [repo, issue_number]
      )

    {:reply, row != nil, state}
  end

  @impl true
  def handle_call({:record_event, run_id, event_type, payload}, _from, state) do
    now = now_utc()
    json = Jason.encode!(payload)

    exec(
      state.conn,
      "INSERT INTO events (run_id, event_type, payload, created_at) VALUES (?1, ?2, ?3, ?4)",
      [run_id, event_type, json, now]
    )

    append_event_log(state.event_log, %{
      run_id: run_id,
      event_type: event_type,
      payload: payload,
      created_at: now
    })

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:list_events, run_id}, _from, state) do
    rows =
      query_all(
        state.conn,
        "SELECT * FROM events WHERE run_id = ?1 ORDER BY created_at ASC",
        [run_id]
      )

    events =
      Enum.map(rows, fn row ->
        Map.update(row, "payload", %{}, &Jason.decode!/1)
      end)

    {:reply, events, state}
  end

  @impl true
  def handle_call({:record_incident, run_id, attrs}, _from, state) do
    now = now_utc()

    exec(
      state.conn,
      """
        INSERT INTO incidents (run_id, check_name, failure_class, signature, created_at)
        VALUES (?1, ?2, ?3, ?4, ?5)
      """,
      [
        run_id,
        attrs[:check_name] || attrs["check_name"],
        attrs[:failure_class] || attrs["failure_class"],
        attrs[:signature] || attrs["signature"],
        now
      ]
    )

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:list_incidents, run_id}, _from, state) do
    rows =
      query_all(
        state.conn,
        "SELECT * FROM incidents WHERE run_id = ?1 ORDER BY created_at ASC",
        [run_id]
      )

    {:reply, rows, state}
  end

  @impl true
  def handle_call({:record_waiver, run_id, attrs}, _from, state) do
    now = now_utc()

    exec(
      state.conn,
      """
        INSERT INTO waivers (run_id, check_name, rationale, waived_at)
        VALUES (?1, ?2, ?3, ?4)
      """,
      [
        run_id,
        attrs[:check_name] || attrs["check_name"],
        attrs[:rationale] || attrs["rationale"],
        now
      ]
    )

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:list_waivers, run_id}, _from, state) do
    rows =
      query_all(
        state.conn,
        "SELECT * FROM waivers WHERE run_id = ?1 ORDER BY waived_at ASC",
        [run_id]
      )

    {:reply, rows, state}
  end

  @impl true
  def handle_call({:mark_semantic_ready, run_id}, _from, state) do
    exec(
      state.conn,
      "UPDATE runs SET semantic_ready = 1, updated_at = ?1 WHERE run_id = ?2",
      [now_utc(), run_id]
    )

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:set_dispatch_paused, paused?}, _from, state) do
    now = now_utc()
    value = if paused?, do: "true", else: "false"

    exec(
      state.conn,
      """
      INSERT INTO control (key, value, updated_at)
      VALUES ('dispatch_paused', ?1, ?2)
      ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
      """,
      [value, now]
    )

    {:reply, :ok, state}
  end

  @impl true
  def handle_call(:dispatch_paused?, _from, state) do
    row = query_one(state.conn, "SELECT value FROM control WHERE key = 'dispatch_paused'", [])
    {:reply, row != nil and row["value"] == "true", state}
  end

  @impl true
  def handle_call({:list_active_runs, repo}, _from, state) do
    rows =
      query_all(
        state.conn,
        "SELECT * FROM runs WHERE repo = ?1 AND completed_at IS NULL ORDER BY picked_at ASC",
        [repo]
      )

    {:reply, rows, state}
  end

  @impl true
  def handle_call({:list_held_leases, repo}, _from, state) do
    rows =
      query_all(
        state.conn,
        """
        SELECT l.repo, l.issue_number, l.run_id, l.acquired_at, r.pr_number, r.completed_at
        FROM leases l
        JOIN runs r ON l.run_id = r.run_id
        WHERE l.repo = ?1 AND l.released_at IS NULL AND r.completed_at IS NOT NULL
        ORDER BY l.acquired_at ASC
        """,
        [repo]
      )

    {:reply, rows, state}
  end

  @impl true
  def handle_call({:expire_stale_run, repo, run_id, issue_number, heartbeat_at}, _from, state) do
    now = now_utc()

    # Record event
    event_json = Jason.encode!(%{heartbeat_at: heartbeat_at})

    exec(
      state.conn,
      "INSERT INTO events (run_id, event_type, payload, created_at) VALUES (?1, ?2, ?3, ?4)",
      [run_id, "stale_run_detected", event_json, now]
    )

    # Mark run failed
    exec(
      state.conn,
      "UPDATE runs SET phase = 'failed', status = 'failed', completed_at = ?1, updated_at = ?1 WHERE run_id = ?2",
      [now, run_id]
    )

    # Release lease
    exec(
      state.conn,
      "UPDATE leases SET released_at = ?1 WHERE repo = ?2 AND issue_number = ?3 AND released_at IS NULL",
      [now, repo, issue_number]
    )

    append_event_log(state.event_log, %{
      run_id: run_id,
      event_type: "stale_run_detected",
      payload: %{heartbeat_at: heartbeat_at},
      created_at: now
    })

    broadcast_update()
    {:reply, :ok, state}
  end

  # --- Private ---

  defp create_tables(conn) do
    for sql <- [
          """
          CREATE TABLE IF NOT EXISTS runs (
            run_id TEXT PRIMARY KEY,
            repo TEXT NOT NULL,
            issue_number INTEGER NOT NULL,
            issue_title TEXT,
            phase TEXT NOT NULL DEFAULT 'pending',
            status TEXT NOT NULL DEFAULT 'pending',
            builder_sprite TEXT,
            branch TEXT,
            pr_number INTEGER,
            pr_url TEXT,
            worktree_path TEXT,
            turn_count INTEGER DEFAULT 0,
            semantic_ready INTEGER DEFAULT NULL,
            replay_count INTEGER DEFAULT 0,
            dispatch_attempt_count INTEGER DEFAULT 0,
            builder_failure_class TEXT,
            builder_failure_reason TEXT,
            picked_at TEXT,
            completed_at TEXT,
            heartbeat_at TEXT,
            ci_wait_started_at TEXT,
            ci_last_reported_at TEXT,
            blocked_reason TEXT,
            updated_at TEXT
          )
          """,
          """
          CREATE TABLE IF NOT EXISTS leases (
            repo TEXT NOT NULL,
            issue_number INTEGER NOT NULL,
            run_id TEXT NOT NULL,
            acquired_at TEXT NOT NULL,
            released_at TEXT,
            blocked_at TEXT,
            PRIMARY KEY (repo, issue_number, run_id)
          )
          """,
          """
          CREATE TABLE IF NOT EXISTS events (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            run_id TEXT NOT NULL,
            event_type TEXT NOT NULL,
            payload TEXT NOT NULL DEFAULT '{}',
            created_at TEXT NOT NULL
          )
          """,
          """
          CREATE TABLE IF NOT EXISTS incidents (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            run_id TEXT NOT NULL,
            check_name TEXT,
            failure_class TEXT NOT NULL,
            signature TEXT,
            created_at TEXT NOT NULL
          )
          """,
          """
          CREATE TABLE IF NOT EXISTS waivers (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            run_id TEXT NOT NULL,
            check_name TEXT,
            rationale TEXT NOT NULL,
            waived_at TEXT NOT NULL
          )
          """,
          """
          CREATE TABLE IF NOT EXISTS control (
            key TEXT PRIMARY KEY,
            value TEXT NOT NULL,
            updated_at TEXT NOT NULL
          )
          """,
          """
          CREATE INDEX IF NOT EXISTS idx_runs_repo_active
          ON runs(repo, completed_at, picked_at)
          """,
          """
          CREATE INDEX IF NOT EXISTS idx_runs_repo_pr
          ON runs(repo, pr_number)
          """
        ] do
      exec(conn, sql, [])
    end

    ensure_columns(conn, "runs", [
      {"ci_wait_started_at", "TEXT"},
      {"ci_last_reported_at", "TEXT"},
      {"blocked_reason", "TEXT"},
      {"dispatch_attempt_count", "INTEGER DEFAULT 0"},
      {"builder_failure_class", "TEXT"},
      {"builder_failure_reason", "TEXT"}
    ])
  end

  defp ensure_columns(conn, table, columns) do
    existing =
      conn
      |> query_all("PRAGMA table_info(#{table})", [])
      |> Enum.map(& &1["name"])
      |> MapSet.new()

    Enum.each(columns, fn {name, type} ->
      unless MapSet.member?(existing, name) do
        exec(conn, "ALTER TABLE #{table} ADD COLUMN #{name} #{type}", [])
      end
    end)
  end

  defp exec(conn, sql, params) do
    {:ok, stmt} = Exqlite.Sqlite3.prepare(conn, sql)
    :ok = Exqlite.Sqlite3.bind(stmt, params)
    :done = Exqlite.Sqlite3.step(conn, stmt)
    :ok = Exqlite.Sqlite3.release(conn, stmt)
  end

  defp query_all(conn, sql, params) do
    {:ok, stmt} = Exqlite.Sqlite3.prepare(conn, sql)
    :ok = Exqlite.Sqlite3.bind(stmt, params)
    {:ok, columns} = Exqlite.Sqlite3.columns(conn, stmt)
    rows = collect_rows(conn, stmt, columns, [])
    :ok = Exqlite.Sqlite3.release(conn, stmt)
    rows
  end

  defp query_one(conn, sql, params) do
    case query_all(conn, sql, params) do
      [row | _] -> row
      [] -> nil
    end
  end

  defp collect_rows(conn, stmt, columns, acc) do
    case Exqlite.Sqlite3.step(conn, stmt) do
      {:row, values} ->
        row = Enum.zip(columns, values) |> Map.new()
        collect_rows(conn, stmt, columns, [row | acc])

      :done ->
        Enum.reverse(acc)
    end
  end

  defp generate_run_id(issue_number) do
    ts = System.system_time(:second)
    "run-#{issue_number}-#{ts}"
  end

  defp now_utc do
    DateTime.utc_now() |> DateTime.to_iso8601()
  end

  defp broadcast_update do
    if Process.whereis(Conductor.PubSub) do
      Phoenix.PubSub.broadcast(Conductor.PubSub, "dashboard", :runs_updated)
    end

    :ok
  end

  defp append_event_log(path, event) do
    File.mkdir_p!(Path.dirname(path))
    File.write!(path, Jason.encode!(event) <> "\n", [:append])
  rescue
    e ->
      Logger.warning("event log append failed: #{Exception.message(e)}")
      :ok
  end
end
