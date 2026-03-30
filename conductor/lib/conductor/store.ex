defmodule Conductor.Store do
  @moduledoc """
  Durable event and control persistence over SQLite.

  Deep module: all SQL is hidden. Callers see only Elixir maps and tuples.
  Serializes all writes through a GenServer to avoid SQLite concurrency issues.
  """

  use GenServer
  require Logger

  # --- Public API ---

  def start_link(opts \\ []) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  @spec record_event(binary(), binary(), map()) :: :ok
  def record_event(source, event_type, payload \\ %{}) do
    GenServer.call(__MODULE__, {:record_event, source, event_type, payload})
  end

  @spec list_events(binary()) :: [map()]
  def list_events(source) do
    GenServer.call(__MODULE__, {:list_events, source})
  end

  @doc "List recent events across all sources, newest first."
  @spec list_all_events(keyword()) :: [map()]
  def list_all_events(opts \\ []) do
    GenServer.call(__MODULE__, {:list_all_events, opts})
  end

  @doc "Persist whether new dispatch is paused."
  @spec set_dispatch_paused(boolean()) :: :ok
  def set_dispatch_paused(paused?) do
    GenServer.call(__MODULE__, {:set_dispatch_paused, paused?})
  end

  @doc "Return true when dispatch is paused."
  @spec dispatch_paused?() :: boolean()
  def dispatch_paused? do
    GenServer.call(__MODULE__, :dispatch_paused?)
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
  def handle_call({:record_event, source, event_type, payload}, _from, state) do
    now = now_utc()
    json = Jason.encode!(payload)

    exec(
      state.conn,
      "INSERT INTO events (run_id, event_type, payload, created_at) VALUES (?1, ?2, ?3, ?4)",
      [source, event_type, json, now]
    )

    append_event_log(state.event_log, %{
      run_id: source,
      event_type: event_type,
      payload: payload,
      created_at: now
    })

    broadcast_update()
    {:reply, :ok, state}
  end

  @impl true
  def handle_call({:list_events, source}, _from, state) do
    rows =
      query_all(
        state.conn,
        "SELECT * FROM events WHERE run_id = ?1 ORDER BY created_at ASC",
        [source]
      )

    events =
      Enum.map(rows, fn row ->
        Map.update(row, "payload", %{}, &Jason.decode!/1)
      end)

    {:reply, events, state}
  end

  @impl true
  def handle_call({:list_all_events, opts}, _from, state) do
    limit = Keyword.get(opts, :limit, 100)

    rows =
      query_all(
        state.conn,
        "SELECT * FROM events ORDER BY created_at DESC LIMIT ?1",
        [limit]
      )

    events =
      Enum.map(rows, fn row ->
        Map.update(row, "payload", %{}, fn
          val when is_binary(val) -> Jason.decode!(val)
          val -> val
        end)
      end)

    {:reply, events, state}
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

  # --- Private ---

  defp create_tables(conn) do
    for sql <- [
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
          CREATE TABLE IF NOT EXISTS control (
            key TEXT PRIMARY KEY,
            value TEXT NOT NULL,
            updated_at TEXT NOT NULL
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

  defp now_utc do
    DateTime.utc_now() |> DateTime.to_iso8601()
  end

  defp broadcast_update do
    if Process.whereis(Conductor.PubSub) do
      Phoenix.PubSub.broadcast(Conductor.PubSub, "dashboard", :store_updated)
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
