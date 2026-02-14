# Transport Fallback Rationales

This document explicitly documents the rationale for each operation where CLI fallback remains required, as per issue #262.

## Design Principle

API-first with CLI fallback for unsupported features. All operations attempt API first where equivalent behavior exists; CLI is used only where API limitations make it necessary.

## Fallback Rationales by Operation

### List

**Primary:** CLI  
**Rationale:** The registry is the source of truth for sprite names. The Fly Machines API returns machine IDs and raw VM names, not the logical sprite names used by the system. While we could map machine metadata back to sprite names, this requires registry lookup anyway. Using CLI which already interfaces with the registry is simpler and avoids inconsistencies.

**Future API Path:** A dedicated Sprites API endpoint that returns logical sprite names from the registry would allow API-first here.

---

### Exec (with environment variables)

**Primary:** CLI  
**Rationale:** The current Fly Machines exec API does not support passing environment variables to the executed command. Environment variables are required for:

- `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` for agent authentication
- `OPENROUTER_API_KEY` for proxy routing
- `SPRITE_NAME`, `SPRITE_WEBHOOK_URL` for agent context
- `MAX_ITERATIONS`, `MAX_TOKENS`, `MAX_TIME_SEC` for Ralph loop limits

Without API support for environment variables, CLI is required.

**Future API Path:** Fly Machines exec endpoint would need to accept an `env` map in the request body.

---

### Exec (large stdin > 64KB)

**Primary:** CLI  
**Rationale:** Large stdin uploads via API exec are inefficient due to request size limits and buffering. CLI uses direct SSH streaming which handles large payloads better.

**Future API Path:** Direct file upload API followed by exec referencing the uploaded file.

---

### Upload / UploadFile

**Primary:** CLI  
**Rationale:** The Fly Machines API does not provide a direct file upload endpoint. File transfers currently require:

1. Encoding file content in exec request (inefficient for binary/large files)
2. Multi-step process: upload to temporary location, then move

CLI uses direct SSH/SCP via the sprite binary which handles this efficiently.

**Future API Path:** Native file upload endpoint in the Machines API or Sprites API.

---

### Create

**Primary:** CLI (current implementation)  
**Rationale:** While the Fly Machines API supports machine creation, several factors require CLI:

1. **Registry integration:** New sprites must be registered in `~/.config/bb/registry.toml`
2. **Persona/config mapping:** Persona selection and config version hints from `compositions.yaml`
3. **Bootstrap scripts:** Post-creation bootstrap relies on CLI-provided behavior

**Future API Path:** Full Sprites API integration with registry management would allow API-first creation.

---

### Destroy

**Primary:** CLI (current implementation)  
**Rationale:** Similar to Create:

1. **Registry cleanup:** Sprite entry must be removed from registry
2. **State synchronization:** Local state files need cleanup

**Future API Path:** Sprites API with registry integration.

---

### CheckpointCreate / CheckpointList

**Primary:** CLI  
**Rationale:** Checkpoint operations are not exposed via the Fly Machines API. These are custom operations implemented by the sprite CLI.

**Future API Path:** Sprites-native API endpoints for checkpoint management.

---

### API / APISprite

**Primary:** CLI  
**Rationale:** These methods are passthroughs to the sprite CLI's `api` subcommand, which provides raw access to machine metadata. There's no equivalent Machines API endpoint for arbitrary metadata queries.

**Future API Path:** Structured metadata API in Sprites API.

---

## Metrics and Observability

The `FallbackTransport` records telemetry for debugging and optimization:

```go
type TransportMetricsSnapshot struct {
    APICalls   int64           // Total API calls attempted
    CLICalls   int64           // Total CLI calls executed
    APIErrors  int64           // API call failures
    CLIErrors  int64           // CLI call failures
    APILatency time.Duration   // Cumulative API latency
    CLILatency time.Duration   // Cumulative CLI latency
    Fallbacks  int64           // How many times CLI was used as fallback
}
```

Access via:

```go
transport, _ := NewFallbackTransport(cli, org)
// ... operations ...
metrics := transport.Metrics()
```

## TransportMethod Tracking

Each operation sets the `Method()` indicator:

- `TransportAPI` - Operation used API transport
- `TransportCLI` - Operation used CLI transport

This allows callers to understand the actual transport mechanism used.

## Future Work

1. **API Capability Detection:** Query Sprites API capabilities at initialization to dynamically determine which operations can use API vs CLI
2. **Progressive Migration:** As Fly/Sprites API adds features, migrate individual operations from CLI to API
3. **Observability Integration:** Export transport metrics to Prometheus/OpenTelemetry
4. **Circuit Breaker:** Automatically mark API unavailable after repeated failures to reduce latency

---

Related: #262, #133, #256, #257
