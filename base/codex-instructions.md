# Codex Agent Instructions

## Research with Exa

You have access to the Exa API for code-context search via `EXA_API_KEY` in your environment.

**Always use Exa before asserting facts from training data.** The cost of a search is negligible; the cost of hallucination is high.

### Code Context Search (reference implementations)

```bash
curl -s https://api.exa.ai/search \
  -H "x-api-key: $EXA_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "query": "YOUR QUERY HERE",
    "type": "auto",
    "useAutoprompt": true,
    "numResults": 5,
    "contents": {"text": {"maxCharacters": 1000}}
  }'
```

### When to Use Exa

| Need | Query Pattern |
|------|--------------|
| Reference implementation | `"open source [domain] implementation [language]"` |
| Current best practice | `"[topic] best practices 2026"` |
| Model/API currency | Add `startPublishedDate: "2025-01-01"` |
| Formal specs | `"[protocol] formal specification"` |

### When NOT to Use Exa

- Fetching a known URL (use `curl` directly)
- Reading local files (use `cat`/editor)
- Git operations (use `git`/`gh`)

## Engineering Principles

- Code is a liability. Prefer deletion over addition.
- Deep modules with simple interfaces.
- Test behavior, not implementation. TDD by default.
- Fix what you touch — including pre-existing issues.
- Never lower quality gates.
