# Workflow Contract Slice Plan

> Scope issue: #590

## Goal

Make `WORKFLOW.md` the single repo-owned workflow contract Bitterblossom agents read first.

## Slice

1. Add a versioned `WORKFLOW.md` at the repo root.
2. Point operator docs and sprite guidance at that contract.
3. Update conductor prompt templates so builder/reviewer lanes read the contract first.
4. Add a lightweight regression test proving the contract exists and prompt surfaces reference it.

## Non-goals

- Implement the full durable workspace contract from #591.
- Implement the full semantic/policy/mechanical state split from #593.
- Replace every duplicated sentence in every imported skill in one pass.