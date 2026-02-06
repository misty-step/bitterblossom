# Orientation Phase (Run FIRST, before any work)

Before starting any task, complete this orientation:

## 1. Context Load
- Read MEMORY.md for learnings from previous iterations
- Read CLAUDE.md for repo-specific conventions
- Check git log for recent changes (what happened since last time?)

## 2. Assumption Check
- What do I think the current state is?
- Verify it: run tests, check CI status, read error logs
- What assumptions from the prompt might be stale?

## 3. Pattern Recognition
- Have I seen this type of problem before? (Check MEMORY.md)
- What worked/failed last time?
- What's the fastest path based on accumulated knowledge?

## 4. Decision
- High confidence (clear path, seen before): Execute immediately
- Medium confidence (mostly clear, some unknowns): Research first, then execute
- Low confidence (unclear, risky): Document what you know, write BLOCKED.md

## 5. After Task Completion
Write LEARNINGS.md with:
- What patterns did you discover in this codebase?
- What was harder/easier than expected?
- What would you tell the next agent working on this repo?
- Any architectural insights or gotchas?

These learnings will be fed back to the fleet.
