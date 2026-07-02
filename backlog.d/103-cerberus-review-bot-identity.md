# Cerberus review comments post as the operator's personal GitHub identity

Priority: P3 | Status: ready | Estimate: S

## Goal

Cerberus review comments dispatched through BB's `review` task should post
under a dedicated bot/app identity, not the operator's personal account.

## Context

Live 2026-07-02: the first production Cerberus reviews
(misty-step/bitterblossom#842, #936; misty-step/powder#31) all posted under
`phrazzld` (the operator's personal GitHub login), because `GH_TOKEN` on the
plane is a personal access token. Cerberus's own README already names the
better shape: "If the token is a GitHub App installation token, Cerberus
posts as that app identity." Every other bot on these repos
(`coderabbitai[bot]`, `cursor[bot]`, `gemini-code-assist[bot]`) posts under a
distinct bot identity, which makes advisory-vs-human comments easy to
triage at a glance; Cerberus comments currently read as if the operator
wrote them by hand.

## Oracle

- [ ] `review` task's `GH_TOKEN` secret is a GitHub App installation token (or
      equivalent machine identity), not a personal PAT.
- [ ] A real review posts under the bot/app identity, verified via
      `gh api repos/<r>/issues/<n>/comments --jq '.[].user.login'`.
- [ ] Token minting/rotation for the app identity is documented in
      `docs/operations/README.md` alongside the existing secret-rotation
      runbook.

## Notes

Not urgent — advisory-only, no authority implications — but worth doing
before Cerberus is the default reviewer across more fleet repos, since the
identity confusion gets worse with scale.
