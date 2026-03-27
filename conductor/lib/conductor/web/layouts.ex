defmodule Conductor.Web.Layouts do
  @moduledoc "Root and app layouts for the dashboard."

  use Phoenix.Component

  def root(assigns) do
    ~H"""
    <!DOCTYPE html>
    <html lang="en">
      <head>
        <meta charset="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <title>Bitterblossom Dashboard</title>
        <style>
          body { font-family: monospace; background: #0d1117; color: #c9d1d9; margin: 0; padding: 16px; }
          h1 { color: #58a6ff; margin: 0 0 16px; }
          .stats { display: flex; gap: 16px; margin-bottom: 24px; flex-wrap: wrap; }
          .stat { background: #161b22; border: 1px solid #30363d; border-radius: 6px; padding: 12px 20px; }
          .stat-label { color: #8b949e; font-size: 0.85em; }
          .stat-value { font-size: 1.6em; font-weight: bold; }
          table { width: 100%; border-collapse: collapse; background: #161b22; border: 1px solid #30363d; border-radius: 6px; }
          th { background: #21262d; color: #8b949e; text-align: left; padding: 8px 12px; font-size: 0.85em; border-bottom: 1px solid #30363d; }
          td { padding: 8px 12px; border-bottom: 1px solid #21262d; font-size: 0.9em; }
          tr:last-child td { border-bottom: none; }
          .phase-building { color: #f0883e; }
          .phase-reviewing { color: #a371f7; }
          .phase-merged { color: #3fb950; }
          .phase-blocked { color: #f85149; }
          .phase-pending { color: #8b949e; }
          .phase-failed { color: #f85149; }
          .refresh-note { color: #8b949e; font-size: 0.75em; margin-top: 24px; }
        </style>
        <meta name="csrf-token" content={Phoenix.Controller.get_csrf_token()} />
        <script src="/assets/phoenix/phoenix.min.js"></script>
        <script src="/assets/live_view/phoenix_live_view.min.js"></script>
        <script>
          let liveSocket = new window.LiveView.LiveSocket("/live", window.Phoenix.Socket, {params: {_csrf_token: document.querySelector("meta[name='csrf-token']")?.getAttribute("content")}});
          liveSocket.connect();
          window.liveSocket = liveSocket;
        </script>
      </head>
      <body>
        {@inner_content}
      </body>
    </html>
    """
  end

  def app(assigns), do: ~H[{@inner_content}]
end
