---
name: testing-philosophy
user-invocable: false
description: "Apply testing philosophy: test behavior not implementation, minimize mocks, AAA structure, coverage for confidence not percentage."
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# Testing Philosophy

Universal principles for writing effective tests. Language-agnostic.

## Core Principle

**Test behavior, not implementation.** Tests verify what code does, not how.

## Test Thinking

- **What is the ONE behavior this test must verify?**
- **Behavior or implementation?** If testing how, not what, you're coupling to implementation.
- **What failure would make you distrust this code?** Test that first.

## Test-First Workflow (Canon TDD)

1. Write test list (happy, edge, error scenarios)
2. Turn one into failing test (focus on interface design)
3. Make it pass (minimal implementation)
4. Refactor (improve design while green)
5. Repeat until list empty

**NEVER test:** Private internals, mock call counts (unless count IS behavior), framework code.

## What to Test

**Always:** Public API, business logic, error handling, edge cases.
**Never:** Private implementation, third-party libraries, simple getters/setters, framework features.

### Testing Boundaries

- **Unit:** Pure functions, isolated modules, business logic
- **Integration:** Module interactions, API contracts, workflows
- **E2E:** Critical user journeys, happy path + critical errors only

## Mocking: Minimize

**ALWAYS mock:** External APIs, network calls, non-deterministic behavior.
**NEVER mock:** Your own domain logic, internal collaborators, simple data structures.

**Red flag:** >3 mocks = coupling to implementation.

**Mock boundary:** If the import path is your own codebase, don't mock it.

## Test Structure

**AAA:** Arrange, Act, Assert. Visual separation between phases. One behavior per test.

**Naming:** "should [expected behavior] when [condition]"

## Test Isolation

No shared mutable state. No execution order dependencies. Each test owns its context.

## Coverage

Meaningful > percentage. Critical paths + edge cases = confidence. 100% coverage doesn't guarantee quality.

## Test Smells

- >3 mocks → coupling to implementation
- Brittle assertions → over-specifying
- Unclear intent → vague names, hidden logic
- Testing private methods → test through public API
- Flaky tests → timing dependencies, shared state
- One giant test → split by behavior

## Quick Reference

| Question | Answer |
|----------|--------|
| Write a test? | Public API, business logic, error handling → yes |
| Use TDD? | Production code → yes. Exploration → skip |
| Mock this? | External → yes. Own code → no. >3 mocks → refactor |
| Skip TDD? | Only if exploration, UI layout, or generated code |

**"Tests are a safety net, not a security blanket."**
