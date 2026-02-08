# Multi-Provider Support

Bitterblossom now supports multiple LLM providers for sprites, allowing you to choose the best model for each sprite's specialization.

## Supported Providers

| Provider | Description | Base URL | Models |
|----------|-------------|----------|--------|
| `moonshot` | Native Moonshot AI API (default) | `https://api.moonshot.ai/anthropic` | `kimi-k2.5` |
| `openrouter-kimi` | Kimi via OpenRouter | `https://openrouter.ai/api/v1` | `kimi-k2.5` |
| `openrouter-claude` | Claude via OpenRouter | `https://openrouter.ai/api/v1` | `anthropic/claude-opus-4`, `anthropic/claude-sonnet-4`, `anthropic/claude-haiku-4` |

## Configuration

### Environment Variables

```bash
# Default provider for all sprites (if not specified per-sprite)
export BB_PROVIDER=moonshot  # or openrouter-kimi, openrouter-claude

# API authentication
export ANTHROPIC_AUTH_TOKEN="your-moonshot-or-openrouter-key"
# OR for OpenRouter specifically:
export BB_OPENROUTER_API_KEY="your-openrouter-key"

# Per-sprite provider overrides
export BB_PROVIDER_HEMLOCK=openrouter-claude
export BB_MODEL_HEMLOCK=anthropic/claude-opus-4
```

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
    provider: moonshot  # Uses default model
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

Existing compositions without provider configuration continue to work:

```yaml
sprites:
  bramble:
    definition: sprites/bramble.md
    preference: "Systems & Data"
    # Uses default provider (moonshot/kimi)
```

Sprite persona files with `model: inherit` also continue to work as expected.

## Usage Examples

### Provision with Specific Provider

```bash
# Provision a single sprite with Claude via OpenRouter
./scripts/provision.sh --provider openrouter-claude --model anthropic/claude-opus-4 hemlock

# Provision with Kimi via OpenRouter
./scripts/provision.sh --provider openrouter-kimi fern

# Provision all sprites from composition (uses per-sprite provider config)
./scripts/provision.sh --all

# Use specific composition with provider configs
./scripts/provision.sh --composition compositions/v3-multi-provider.yaml --all
```

### Sync with Provider Update

```bash
# Sync a sprite and update its provider configuration
./scripts/sync.sh --provider openrouter-claude --model anthropic/claude-sonnet-4 sage

# Sync all sprites (uses their configured providers)
./scripts/sync.sh --all
```

## Multi-Provider Composition Example

See `compositions/v3-multi-provider.yaml` for a complete example:

```yaml
version: 3
name: "Multi-Provider Fae Court"

sprites:
  # Core team uses Moonshot/Kimi (default)
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
| Security auditing | `openrouter-claude` (Opus) | Superior reasoning for threat modeling |
| Documentation | `openrouter-claude` (Sonnet) | Excellent writing quality |
| General coding | `moonshot` or `openrouter-kimi` | Fast, cost-effective, good code generation |
| Architecture decisions | `openrouter-claude` (Opus) | Better at complex system design |
| Test writing | `moonshot` or `openrouter-kimi` | Good at pattern recognition |
| DevOps/Infrastructure | `moonshot` | Reliable for configuration tasks |

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

Existing compositions work without changes. To opt-in to multi-provider:

1. Add `provider` configuration to specific sprites
2. Ensure `ANTHROPIC_AUTH_TOKEN` or `BB_OPENROUTER_API_KEY` is set
3. Run `./scripts/provision.sh --all` to re-provision with new provider settings

### Testing Provider Configuration

```bash
# Verify settings are rendered correctly
./scripts/provision.sh --dry-run bramble 2>&1 | grep -A5 "Provider:"

# Check rendered settings.json on a sprite
sprite exec -s bramble -- cat /home/sprite/.claude/settings.json | jq '.env.ANTHROPIC_MODEL'
```

## Troubleshooting

### "ANTHROPIC_AUTH_TOKEN is required"

Set your API token:
```bash
export ANTHROPIC_AUTH_TOKEN="your-token"
```

For OpenRouter, you can also use:
```bash
export BB_OPENROUTER_API_KEY="your-openrouter-key"
```

### "Invalid provider"

Valid providers are: `moonshot`, `openrouter-kimi`, `openrouter-claude`, `inherit`

### Model not found errors

Ensure model identifiers use the correct format:
- Kimi: `kimi-k2.5`
- Claude via OpenRouter: `anthropic/claude-opus-4` (must include provider prefix)
