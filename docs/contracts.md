# JSON/NDJSON Contracts

## Versioning Policy

- All machine JSON responses include `version`.
- Current version: `v1`.
- Contract changes follow semver intent:
  - Breaking shape changes increment major (`v1` -> `v2`).
  - Backward-compatible additions stay in the same major line.

## Response Envelope

Every command-level JSON response is wrapped:

```json
{
  "version": "v1",
  "command": "compose.status",
  "data": {}
}
```

`error` replaces `data` on failures:

```json
{
  "version": "v1",
  "command": "agent.status",
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "--state-file is required"
  }
}
```

Fields:

- `version`: schema version string (`v1`)
- `command`: stable command identifier (for example `compose.status`)
- `data`: success payload (shape is command-specific)
- `error`: machine error object

## Error Object

Schema:

```json
{
  "code": "VALIDATION_ERROR",
  "message": "human readable",
  "details": {},
  "remediation": "suggested fix",
  "trace_id": "optional-correlation-id"
}
```

Error codes:

- `VALIDATION_ERROR`: bad input, invalid/missing flags
- `AUTH_ERROR`: missing or invalid credentials
- `NETWORK_ERROR`: connectivity failures
- `REMOTE_STATE_ERROR`: missing/conflicting remote resources
- `INTERNAL_ERROR`: unexpected internal failures

## Exit Codes

| Exit code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Internal error |
| `2` | Validation error |
| `3` | Auth error |
| `4` | Network error |
| `5` | Remote state error |
| `130` | Interrupted (`SIGINT`) |

## NDJSON Streaming

- Streaming commands emit NDJSON: one JSON object per line.
- NDJSON lines are not wrapped in the command response envelope.
- This keeps streams incremental and parse-friendly.

## Compatibility Promise

- `internal/contracts/contracts_test.go` includes golden tests for envelope stability.
- Any schema drift changes golden snapshots and is caught in CI unless intentionally updated.
