# BB Dashboard Local Service

This runbook makes the review dashboard durable on Serenity:

- local service: `http://127.0.0.1:7091/`
- tailnet URL: `https://serenity.tail5f5eb4.ts.net:7443/`
- launchd label: `com.misty-step.bb-dashboard`
- plane state: `~/.local/share/bitterblossom/bb-dashboard-plane`
- logs: `~/.local/state/bitterblossom/bb-dashboard.{out,err}.log`

The service intentionally uses a local dev/demo plane, not `plane/`. `bb serve`
starts cron and dispatch loops; pointing this reviewer dashboard at production
state would make viewing the UI an execution surface.

## Install Or Refresh

From the checkout:

```sh
./scripts/install-bb-dashboard-service.sh
```

The installer:

1. Builds `target/debug/bb` if needed.
2. Creates the persistent local dashboard plane if it does not already exist.
3. Writes `~/Library/LaunchAgents/com.misty-step.bb-dashboard.plist`.
4. Boots the LaunchAgent with `BB_INGRESS_BIND=127.0.0.1:7091`.
5. Reasserts the Tailscale Serve mapping from HTTPS `:7443` to the local port.

Override knobs:

```sh
BB_DASHBOARD_PLANE=~/.local/share/bitterblossom/bb-dashboard-plane \
BB_DASHBOARD_PORT=7091 \
BB_DASHBOARD_TAILSCALE_HTTPS_PORT=7443 \
./scripts/install-bb-dashboard-service.sh
```

## Verify

```sh
launchctl print gui/$(id -u)/com.misty-step.bb-dashboard | sed -n '1,80p'
curl -fsS http://127.0.0.1:7091/ >/dev/null
curl -fsS https://serenity.tail5f5eb4.ts.net:7443/ >/dev/null
tailscale serve status
```

The expected Tailscale stanza is:

```text
https://serenity.tail5f5eb4.ts.net:7443 (tailnet only)
|-- / proxy http://127.0.0.1:7091
```

## Stop

```sh
launchctl bootout gui/$(id -u) ~/Library/LaunchAgents/com.misty-step.bb-dashboard.plist
tailscale serve clear serenity.tail5f5eb4.ts.net:7443
```

`bootout` stops the local process. `tailscale serve clear` should only be used
after `tailscale serve status` confirms `:7443` still belongs to this dashboard;
do not reset the full Serve config because Serenity hosts other local tools.

## Bastion Option

If the dashboard graduates from local review to shared internal service,
coordinate with the bastion/subnet-router lane instead of exposing the dev
plane directly. The desired shape is `bb-dashboard.internal` routing to a
declared service endpoint, while this local runbook remains the safe reviewer
path.
