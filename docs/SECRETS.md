# Secret Detection & Key Rotation

## Tooling

[gitleaks](https://github.com/gitleaks/gitleaks) scans for secrets in code and git history. Config: `.gitleaks.toml`.

CI runs gitleaks on every PR and push to master via `.github/workflows/gitleaks.yml`.

## Local Usage

```bash
brew install gitleaks

# Scan current files
gitleaks detect --source . --no-git -v

# Scan full git history
gitleaks detect --source . -v

# Pre-commit hook (optional)
gitleaks protect --staged
```

## Custom Rules

`.gitleaks.toml` extends gitleaks defaults with two patterns:

| Rule | Pattern | Matches |
|------|---------|---------|
| `moonshot-api-key` | `sk-[a-zA-Z0-9]{40,}` | Moonshot proxy API keys |
| `anthropic-api-key` | `sk-ant-[a-zA-Z0-9\-_]{40,}` | Anthropic API keys |

Update these if key formats change. Test with `gitleaks detect -v` after changes.

## When a Leak is Detected

1. **Rotate the key immediately** from the provider dashboard (Moonshot, Anthropic, etc.)
2. **Rewrite git history** if the key was committed:
   ```bash
   git filter-repo --replace-text <(echo 'THE_KEY==>***REDACTED***') --force
   git remote add origin https://github.com/misty-step/bitterblossom.git
   git push --force origin master
   ```
3. **Update sprites** — if the rotated key was deployed, re-sync:
   ```bash
   export ANTHROPIC_AUTH_TOKEN="new-key-here"
   ./scripts/sync.sh
   ```
4. **Verify** the old key no longer appears: `gitleaks detect --source . -v`

## Sprite Auth Token Flow

Sprites receive their API key at provision/sync time. The key is never stored in git — `base/settings.json` contains a placeholder (`__SET_VIA_ANTHROPIC_AUTH_TOKEN_ENV__`) that gets rendered from `$ANTHROPIC_AUTH_TOKEN` at deploy time.
