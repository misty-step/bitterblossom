# Multi-Provider Support

Bitterblossom supports multiple LLM providers for sprites, allowing you to choose the best model for each sprite's specialization.

## Supported Providers

| Provider | Description | Base URL | Models |
|----------|-------------|----------|--------|
| `moonshot-anthropic` | **Moonshot Anthropic endpoint (default)** - Preferred for Claude Code | `https://api.moonshot.ai/anthropic` | `kimi-k2-thinking-turbo` |
| `moonshot` | Native Moonshot AI API (legacy) | `https://api.moonshot.ai/anthropic` | `kimi-k2.5` |
| `openrouter-kimi` | Kimi via OpenRouter | `https://openrouter.ai/api/v1` | `kimi-k2.5` |
| `openrouter-claude` | Claude via OpenRouter | `https://openrouter.ai/api/v1` | `anthropic/claude-opus-4`, `anthropic/claude-sonnet-4`, `anthropic/claude-haiku-4` |

## Configuration

### Environment Variables

```bash
# Default provider for all sprites (if not specified per-sprite)
# moonshot-anthropic is the recommended default for Claude Code
export BB_PROVIDER=moonshot-anthropic

# API authentication - uses your Moonshot API key
export ANTHROPIC_AUTH_TOKEN="your-moonshot-api-key"

# Per-sprite provider overrides
export BB_PROVIDER_HEMLOCK=openrouter-claude
export BB_MODEL_HEMLOCK=anthropic/claude-opus-4
```

### Moonshot Anthropic Endpoint (Default)

The `moonshot-anthropic` provider uses Moonshot AI's Anthropic-compatible endpoint, which is the preferred pattern for Claude Code. It sets:

- `ANTHROPIC_BASE_URL=https://api.moonshot.ai/anthropic`
- `ANTHROPIC_AUTH_TOKEN=$ANTHROPIC_AUTH_TOKEN`
- `ANTHROPIC_MODEL=kimi-k2-thinking-turbo`

### Composition YAML

#### Full Provider Block Syntax

```yaml
sprites:
  hemlock:
    definition: sprites/hemlock.md
    preference: "Security Audit"
    provider:
      name: openrouter-claude
      model: anthropic/claude-opus-4
      # Optional: additional environment variables
      API_TIMEOUT_MS: "900000"
```

#### Short Syntax (Inline)

```yaml
sprites:
  bramble:
    definition: sprites/bramble.md
    provider: moonshot-anthropic  # Uses default model (kimi-k2-thinking-turbo)
```

#### With Model Override

```yaml
sprites:
  sage:
    definition: sprites/sage.md
    provider: openrouter-claude
    model: anthropic/claude-sonnet-4
```

### Legacy Backward Compatibility

Existing compositions without provider configuration continue to work and now default to `moonshot-anthropic`:

```yaml
sprites:
  bramble:
    definition: sprites/bramble.md
    preference: "Systems & Data"
    # Uses default provider (moonshot-anthropic / kimi-k2-thinking-turbo)
```

Sprite persona files with `model: inherit` also continue to work as expected.

## Usage Examples

### Provision with Specific Provider

```bash
# Provision a single sprite with Moonshot Anthropic endpoint (default)
bb provision bramble

# Provision a single sprite with Claude via OpenRouter
bb provision hemlock --provider openrouter-claude --model anthropic/claude-opus-4

# Provision all sprites from composition (uses per-sprite provider config)
bb provision --all
```

### Sync with Provider Update

```bash
# Sync a sprite and update its provider configuration
bb sync sage --provider openrouter-claude --model anthropic/claude-sonnet-4

# Sync all sprites (uses their configured providers)
bb sync
```

## Multi-Provider Composition Example

See `compositions/v3-multi-provider.yaml` for a complete example:

```yaml
version: 3
name: "Multi-Provider Fae Court"

sprites:
  # Core team uses Moonshot Anthropic endpoint (default)
  bramble:
    definition: sprites/bramble.md
    preference: "Systems & Data"

  # Security specialist uses Claude Opus
  hemlock:
    definition: sprites/hemlock.md
    preference: "Security Audit"
    provider:
      name: openrouter-claude
      model: anthropic/claude-opus-4

  # Documentation specialist uses Claude Sonnet
  sage:
    definition: sprites/sage.md
    preference: "Documentation"
    provider:
      name: openrouter-claude
      model: anthropic/claude-sonnet-4
```

## Provider Selection Guidelines

| Task Type | Recommended Provider | Rationale |
|-----------|---------------------|-----------|
| **Default/General coding** | `moonshot-anthropic` | Fast, reliable, optimized for Claude Code |
| Security auditing | `openrouter-claude` (Opus) | Superior reasoning for threat modeling |
| Documentation | `openrouter-claude` (Sonnet) | Excellent writing quality |
| General coding (legacy) | `moonshot` | Fast, cost-effective, good code generation |
| Architecture decisions | `openrouter-claude` (Opus) | Better at complex system design |
| Test writing | `moonshot-anthropic` | Good at pattern recognition with thinking turbo |
| DevOps/Infrastructure | `moonshot-anthropic` | Reliable for configuration tasks |

## Technical Details

### How Provider Configuration Works

1. **Composition parsing**: The `provider` field in sprite specs is parsed and validated
2. **Settings rendering**: During `provision` or `sync`, settings.json is rendered with provider-specific environment variables
3. **Environment variables**: The following are set based on provider:
   - `ANTHROPIC_BASE_URL` - API endpoint
   - `ANTHROPIC_MODEL` - Model identifier
   - `ANTHROPIC_AUTH_TOKEN` - API key
   - `OPENROUTER_API_KEY` - OpenRouter-specific key (when using OpenRouter)
   - `CLAUDE_CODE_OPENROUTER_COMPAT` - Compatibility flag for OpenRouter

### Migration from v2 to v3

Existing compositions work without changes and will now use `moonshot-anthropic` as the default provider instead of `moonshot`. To pin to a specific provider:

1. Add `provider` configuration to specific sprites
2. Ensure `ANTHROPIC_AUTH_TOKEN` is set (uses your Moonshot API key)
3. Run `bb provision --all` to re-provision with new provider settings

### Testing Provider Configuration

```bash
# Verify settings are rendered correctly
bb provision --dry-run bramble 2>&1 | grep -A5 "Provider:"

# Check rendered settings.json on a sprite
sprite exec -s bramble -- cat /home/sprite/.claude/settings.json | jq '.env.ANTHROPIC_MODEL'
```

## Troubleshooting

### "ANTHROPIC_AUTH_TOKEN is required"

Set your API token:
```bash
export ANTHROPIC_AUTH_TOKEN="your-moonshot-api-key"
```

For OpenRouter, you can also use:
```bash
export OPENROUTER_API_KEY="your-openrouter-key"
```

### "Invalid provider"

Valid providers are: `moonshot-anthropic`, `moonshot`, `openrouter-kimi`, `openrouter-claude`, `inherit`

### Model not found errors

Ensure model identifiers use the correct format:
- Moonshot Anthropic: `kimi-k2-thinking-turbo`
- Kimi: `kimi-k2.5`
- Claude via OpenRouter: `anthropic/claude-opus-4` (must include provider prefix)
