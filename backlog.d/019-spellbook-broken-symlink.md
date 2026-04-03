# Spellbook bootstrap fails on stale skill symlinks

Priority: high
Status: ready
Estimate: S

## Goal
Spellbook bootstrap should clean up broken symlinks from renamed/deleted skills instead of failing.

## Problem
When a spellbook skill is renamed (e.g. `debug` → `investigate`), the old symlink remains on the sprite filesystem: `/home/sprite/.claude/skills/debug/debug -> /home/sprite/spellbook/skills/debug`. The bootstrap script detects this as a broken symlink and fails, blocking the sprite from launching.

## Evidence
Factory audit 2026-04-01: bb-fixer stuck in infinite bootstrap-fail loop. Every recovery attempt re-triggers the same broken symlink error. Sprite is healthy and reachable but cannot launch its agent loop.

## Sequence
- [ ] Bootstrap script: detect and remove broken symlinks in skills/ before linking
- [ ] Or: re-clone spellbook on the sprite to clear stale state (`rm -rf /home/sprite/spellbook && git clone ...`)
- [ ] Test: bootstrap succeeds when stale symlinks exist from renamed skills

## Oracle
- [ ] Bootstrap succeeds even when old skill symlinks point to deleted targets
- [ ] bb-fixer can launch after bootstrap
