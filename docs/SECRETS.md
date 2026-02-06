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
trufflehog git file://. --since-commit $(git merge-base HEAD master) --only-verified
```

`--only-verified` means TruffleHog confirms the secret is live before reporting. Drop the flag to see unverified matches too (recommended periodically — CI only reports verified secrets, so rotated keys in history won't trigger CI failures but are still visible to anyone who clones the repo).

## When a Leak is Detected

1. **Rotate the key immediately** from the provider dashboard (Anthropic, Moonshot, GitHub, Fly.io, etc.)
2. **Check for unauthorized usage** in the provider's dashboard — look for anomalous API calls between the time of commit and rotation.
3. **Rewrite git history** if the key was committed:
   ```bash
   # Install if needed: brew install git-filter-repo
   # Save remote URL before filter-repo removes it
   REMOTE_URL=$(git remote get-url origin)
   git filter-repo --replace-text <(echo 'THE_KEY==>***REDACTED***') --force
   git remote add origin "$REMOTE_URL"
   ```
4. **Force-push the rewritten history:**
   ```bash
   git push --force origin master
   ```
   **Warning:** This rewrites shared history. Coordinate with all collaborators before force-pushing. Sprites with diverged local state will need re-provisioning. This must be done from a local workstation — the destructive-command-guard hook blocks force pushes from sprites.
5. **Update sprites** — if the rotated key was deployed, re-sync:
   ```bash
   export ANTHROPIC_AUTH_TOKEN="new-key-here"
   ./scripts/sync.sh
   ```
6. **Verify** the old key no longer appears: `trufflehog git file://. --only-verified`

## Sprite Auth Token Flow

Sprites receive their API key at provision/sync time. The key is never stored in git — `base/settings.json` contains a placeholder (`__SET_VIA_ANTHROPIC_AUTH_TOKEN_ENV__`) that gets rendered from `$ANTHROPIC_AUTH_TOKEN` at deploy time. At teardown, `teardown.sh` redacts the token from exported archives.
