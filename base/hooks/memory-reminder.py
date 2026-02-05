#!/usr/bin/env python3
"""
Memory reminder for sprites.

Stop hook - prompts sprite to update MEMORY.md after completing work.
Runs on Notification events (task completion signals).

Exit 0 always (inform, don't block).
"""
import json
import sys


def main():
    try:
        data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.exit(0)

    # Only trigger on stop/notification events
    event = data.get("event")
    if event not in ("Stop", "stop"):
        sys.exit(0)

    print(
        "[memory-reminder] Session ending. Consider updating MEMORY.md with:\n"
        "  - Insights about problem constraints\n"
        "  - Strategies that worked or failed\n"
        "  - Patterns specific to this codebase\n"
        "  - Mistakes to avoid next time"
    )

    sys.exit(0)


if __name__ == "__main__":
    main()
