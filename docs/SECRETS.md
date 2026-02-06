# Secret Detection & Key Rotation

## Tooling

[TruffleHog](https://github.com/trufflesecurity/trufflehog) scans for secrets in code and git history. It detects 800+ secret types with verification — it actually checks whether detected credentials are live.

CI runs TruffleHog on every PR and push to master via `.github/workflows/secret-detection.yml`.

## Local Usage

```bash
brew install trufflehog

# Scan current directory
trufflehog filesystem --directory . --only-verified

# Scan full git history
trufflehog git file://. --only-verified

# Scan only since a branch point
trufflehog git file://. --since-commit HEAD~10 --only-verified
```

`--only-verified` means TruffleHog confirms the secret is live before reporting. Drop the flag to see unverified matches too.

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
4. **Verify** the old key no longer appears: `trufflehog git file://. --only-verified`

## Sprite Auth Token Flow

Sprites receive their API key at provision/sync time. The key is never stored in git — `base/settings.json` contains a placeholder (`__SET_VIA_ANTHROPIC_AUTH_TOKEN_ENV__`) that gets rendered from `$ANTHROPIC_AUTH_TOKEN` at deploy time.
