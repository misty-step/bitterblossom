defmodule Conductor.Store do
  @moduledoc """
  Durable run and event persistence over SQLite.

  Deep module: all SQL is hidden. Callers see only Elixir maps and tuples.
  Serializes all writes through a GenServer to avoid SQLite concurrency issues.
  """

  use GenServer
  require Logger

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec create_run(map()) :: {:ok, binary()}
  def create_run(attrs) do
    GenServer.call(__MODULE__, {:create_run, attrs})
  end

  @spec update_run(binary(), map()) :: :ok
  def update_run(run_id, attrs) do
    GenServer.call(__MODULE__, {:update_run, run_id, attrs})
  end

  @spec complete_run(binary(), binary(), binary()) :: :ok
  def complete_run(run_id, phase, status) do
    GenServer.call(__MODULE__, {:complete_run, run_id, phase, status})
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

    exec(state.conn, """
      INSERT INTO runs (run_id, repo, issue_number, issue_title, phase, status,
                        builder_sprite, picked_at, heartbeat_at, updated_at)
      VALUES (?1, ?2, ?3, ?4, 'pending', 'pending', ?5, ?6, ?6, ?6)
    """, [run_id, attrs[:repo], attrs[:issue_number], attrs[:issue_title],
          attrs[:builder_sprite], now])

    {:reply, {:ok, run_id}, state}
  end

  @impl true
  def handle_call({:update_run, run_id, attrs}, _from, state) do
    sets = Enum.map_join(attrs, ", ", fn {k, _} -> "#{k} = ?" end)
    vals = Map.values(attrs) ++ [now_utc(), run_id]

    exec(state.conn,
      "UPDATE runs SET #{sets}, updated_at = ? WHERE run_id = ?",
      vals)

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:complete_run, run_id, phase, status}, _from, state) do
    now = now_utc()

    exec(state.conn, """
      UPDATE runs SET phase = ?1, status = ?2, completed_at = ?3, updated_at = ?3
      WHERE run_id = ?4
    """, [phase, status, now, run_id])

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
    rows = query_all(state.conn, "SELECT * FROM runs ORDER BY picked_at DESC LIMIT ?1", [limit])
    {:reply, rows, state}
  end

  @impl true
  def handle_call({:acquire_lease, repo, issue_number, run_id}, _from, state) do
    active =
      query_one(state.conn,
        "SELECT run_id FROM leases WHERE repo = ?1 AND issue_number = ?2 AND released_at IS NULL",
        [repo, issue_number])

    if active do
      {:reply, {:error, :already_leased}, state}
    else
      exec(state.conn,
        "INSERT INTO leases (repo, issue_number, run_id, acquired_at) VALUES (?1, ?2, ?3, ?4)",
        [repo, issue_number, run_id, now_utc()])

      {:reply, :ok, state}
    end
  end

  @impl true
  def handle_call({:release_lease, repo, issue_number}, _from, state) do
    exec(state.conn,
      "UPDATE leases SET released_at = ?1 WHERE repo = ?2 AND issue_number = ?3 AND released_at IS NULL",
      [now_utc(), repo, issue_number])

    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:leased?, repo, issue_number}, _from, state) do
    row =
      query_one(state.conn,
        "SELECT 1 FROM leases WHERE repo = ?1 AND issue_number = ?2 AND released_at IS NULL",
        [repo, issue_number])

    {:reply, row != nil, state}
  end

  @impl true
  def handle_call({:record_event, run_id, event_type, payload}, _from, state) do
    now = now_utc()
    json = Jason.encode!(payload)

    exec(state.conn,
      "INSERT INTO events (run_id, event_type, payload, created_at) VALUES (?1, ?2, ?3, ?4)",
      [run_id, event_type, json, now])

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
    rows = query_all(state.conn,
      "SELECT * FROM events WHERE run_id = ?1 ORDER BY created_at ASC",
      [run_id])

    events = Enum.map(rows, fn row ->
      Map.update(row, "payload", %{}, &Jason.decode!/1)
    end)

    {:reply, events, state}
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
        picked_at TEXT,
        completed_at TEXT,
        heartbeat_at TEXT,
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
      """
    ] do
      exec(conn, sql, [])
    end
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

  defp append_event_log(path, event) do
    File.mkdir_p!(Path.dirname(path))
    File.write!(path, Jason.encode!(event) <> "\n", [:append])
  rescue
    e ->
      Logger.warning("event log append failed: #{Exception.message(e)}")
      :ok
  end
end
