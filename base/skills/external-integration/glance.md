The `external-integration` directory provides a standardized framework for implementing reliable, observable, and recoverable connections to third-party services. Its primary purpose is to enforce a "fail-loudly" integration philosophy that prioritizes early detection of configuration errors and robust handling of asynchronous events.

### Architecture and Key Patterns
The architecture is defined by six core reliability patterns designed to mitigate the inherent instability of external dependencies:
*   **Fail-Fast Configuration:** Environment variables are validated at the module load level rather than at runtime, preventing partial system failures due to missing or malformed credentials.
*   **Health Monitoring:** Every integration requires a dedicated health check endpoint that reports both availability (200/503 status codes) and latency metrics.
*   **Structured Observability:** Logging is standardized to include specific metadata—service name, operation type, user context, and timestamps—to facilitate rapid debugging.
*   **Asynchronous Webhook Processing:** A multi-stage pipeline is required for incoming events: signature verification, immediate logging, persistence for reconciliation, and rapid HTTP 200 responses followed by asynchronous processing.
*   **State Reconciliation:** The system implements a "safety net" architecture where periodic synchronization tasks (crons) or immediate "pull-on-success" checks supplement webhook-driven state changes.

### Key File Roles
*   **SKILL.md:** Serves as the technical specification and compliance manifest. It defines the metadata for the integration (non-user-invocable), specifies allowed system tools (Read, Grep, Glob, Bash), and provides a pre-deployment checklist to ensure idempotency, signature verification, and environment consistency.

### Dependencies and Gotchas
*   **Environment Sensitivity:** The integration logic is highly sensitive to environment variable formatting; trailing whitespace in `.env` files is treated as a fatal error.
*   **Signature Verification:** Webhook handlers are strictly prohibited from processing data before cryptographic signature verification is performed.
*   **Idempotency:** Due to the reliance on both webhooks and reconciliation syncs, the architecture assumes duplicate events will occur and requires handlers to be idempotent.
*   **Canonical Domains:** Webhook configurations must use canonical domains to avoid processing failures caused by unexpected redirects.