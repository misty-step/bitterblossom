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

## 5. Plan the Work
For non-trivial tasks (3+ steps):
- Write `PLAN.md` with checkable items before touching code
- Verify the plan makes sense before executing
- Check items off as you complete them

## 6. After Task Completion
Write `LEARNINGS.md` with concrete rules, not vague principles:
- What patterns did you discover in this codebase?
- What was harder/easier than expected? Why?
- What would you tell the next agent working on this repo?
- Any gotchas or traps to avoid?
- Write each learning as an actionable instruction.

These learnings get harvested back to the fleet — your experience compounds across all sprites.
