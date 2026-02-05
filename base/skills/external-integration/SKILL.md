---
name: external-integration
user-invocable: false
description: "Patterns for reliable external service integration: env validation, health checks, error handling, observability."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# External Integration Patterns

> External services fail. Your integration must be observable, recoverable, and fail loudly.

## Required Patterns

### 1. Fail-Fast Env Validation

Validate environment variables at module load, not at runtime.

```typescript
const REQUIRED = ['SERVICE_API_KEY', 'SERVICE_WEBHOOK_SECRET'];
for (const key of REQUIRED) {
  const value = process.env[key];
  if (!value) throw new Error(`Missing required env var: ${key}`);
  if (value !== value.trim()) throw new Error(`${key} has trailing whitespace`);
}
```

### 2. Health Check Endpoint

Every external service needs a health check. Return `200` when healthy, `503` when degraded. Include latency measurements.

### 3. Structured Error Logging

Log every external failure with full context:
- `service`: Which external service
- `operation`: What you were trying to do
- `userId`: Who this affects
- `error`: The error message
- `timestamp`: When it happened

### 4. Webhook Reliability

1. Verify signature FIRST (before any processing)
2. Log event received BEFORE processing
3. Store event for reconciliation
4. Return 200 quickly, process async if slow

### 5. Reconciliation (Safety Net)

Don't rely 100% on webhooks. Periodically sync state as backup.

### 6. Pull-on-Success

Don't wait for webhook to grant access. Verify payment immediately after redirect.

## Pre-Deploy Checklist

- [ ] All required env vars in `.env.example`
- [ ] Vars set on both dev and prod
- [ ] No trailing whitespace in env values
- [ ] Webhook URL uses canonical domain (no redirects)
- [ ] Signature verification in webhook handler
- [ ] Events logged before processing
- [ ] Health check endpoint exists
- [ ] Reconciliation cron or pull-on-success pattern
- [ ] Idempotency for duplicate events

## Anti-Patterns

- Silent failure on missing config (`process.env.KEY || ''`)
- No context in error logs
- Trusting webhook without signature verification
- 100% reliance on webhooks
- No logging of received events
