# xAI Search Reference

Use this file as the canonical source for the xAI request shape used by the
`research` skill.

## Model

Export `XAI_MODEL_ID` before calling the xAI Responses API and use the variable
in the request payload instead of hardcoding a model string in `SKILL.md`.

Recommended shell setup:

```bash
export XAI_MODEL_ID="${XAI_MODEL_ID:-grok-4.20-beta-latest-non-reasoning}"
```

Why this default:
- xAI's tool-usage and search-tool guides currently show
  `grok-4.20-beta-latest-non-reasoning` in their Responses API examples.
- xAI's models page separately notes that Grok 4 is reasoning-only, so treat
  the alias as vendor-controlled and re-check the official docs or your
  `/v1/models` output if it stops working for your account.

## Request Notes

- Endpoint: `https://api.x.ai/v1/responses`
- Auth header: `Authorization: Bearer $XAI_API_KEY`
- Prefer built-in search tools for social pulse and grounded web lookups.
- Capture citations from the response and surface them in the final report.

Minimal example:

```bash
curl -s https://api.x.ai/v1/responses \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $XAI_API_KEY" \
  -d "{
    \"model\": \"${XAI_MODEL_ID}\",
    \"input\": [
      {
        \"role\": \"user\",
        \"content\": \"What are people saying about <topic> right now?\"
      }
    ]
  }"
```
