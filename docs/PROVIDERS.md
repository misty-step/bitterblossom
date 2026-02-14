# Provider Profile

Bitterblossom has one canonical runtime profile for sprite operation.

## Canonical Profile (Default)

| Field | Value |
|------|-------|
| Provider | `proxy` |
| Model | `minimax/minimax-m2.5` |
| Base URL | `https://openrouter.ai/api` |
| Required auth env | `OPENROUTER_API_KEY` |

This profile is used when sprite provider settings are omitted or set to `inherit`.

## Required Auth

`bb provision` and `bb sync` fail fast unless a provider auth token is present.

Preferred:

```bash
export OPENROUTER_API_KEY="your-openrouter-key"
```

Legacy fallback (supported for compatibility, not recommended):

```bash
export ANTHROPIC_AUTH_TOKEN="legacy-token"
```

## Runtime Environment Rendered to Sprites

Canonical rendering injects:

- `ANTHROPIC_BASE_URL=https://openrouter.ai/api`
- `ANTHROPIC_MODEL=minimax/minimax-m2.5`
- `ANTHROPIC_SMALL_FAST_MODEL=minimax/minimax-m2.5`
- `ANTHROPIC_AUTH_TOKEN=<token>`
- `OPENROUTER_API_KEY=<token>`
- `CLAUDE_CODE_OPENROUTER_COMPAT=1`

## Compatibility Modes (Non-Default)

Legacy/advanced provider identifiers remain parseable so existing compositions do not break immediately:

- `moonshot-anthropic`
- `moonshot`
- `openrouter-kimi`
- `openrouter-claude`
- `inherit`

Use these only when explicitly needed. They are not equivalent defaults.

## Canonical Usage

```bash
export OPENROUTER_API_KEY="your-openrouter-key"

bb provision --all
bb sync
```

## Troubleshooting

### "OPENROUTER_API_KEY is required"

Set the canonical auth token before provisioning/syncing:

```bash
export OPENROUTER_API_KEY="your-openrouter-key"
```

If you are migrating old setup scripts, `ANTHROPIC_AUTH_TOKEN` is still accepted as temporary fallback.
