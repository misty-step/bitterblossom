# Surface webhook response status and bounded body while deliveries are retrying

Priority: P2 | Status: ready | Estimate: S

## Goal

Canary webhook delivery inspection should expose enough bounded response detail
to debug a failing delivery before it is discarded.

## Evidence

Live drill on 2026-07-02:

- Webhook: `WHK-4t02p71mz5h6`
- Delivery: `DLV-zo93xxqrjhsd`
- During retries, the delivery was visible as `retrying` without enough detail
  to distinguish signature failure, route failure, or BB admission failure.
- After discard, it showed only `reason=http_429`.

The final reason was useful, but only after the retry budget was exhausted.

## Oracle

- [ ] Delivery list/detail exposes `last_status_code` while retrying.
- [ ] Delivery list/detail exposes a bounded, secret-scrubbed `last_response`
      snippet or an enum reason before final discard.
- [ ] The response body cap prevents large or secret-bearing payloads from
      being stored.
- [ ] A retrying delivery test covers response status/body visibility.

## Non-goals

Do not store request signing secrets, authorization headers, or full response
bodies.
