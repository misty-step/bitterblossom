# Reference Architecture Survey — AI Agent Orchestration for Software Development

**Date:** 2026-03-14
**Context:** Phase 2 Track A of groom session. Bitterblossom conductor is 1,703 LOC Elixir/OTP.

---

## 1. Direct Competitors (Same Problem Space)

### ComposioHQ/agent-orchestrator
- **URL:** https://github.com/ComposioHQ/agent-orchestrator
- **Stack:** TypeScript, 40K LOC, 17 plugins, 3,288 tests
- **Architecture:** Dual-layer (Planner + Executor). Each agent gets isolated git worktree + tmux session. Agent-agnostic (Claude Code, Codex, Aider), runtime-agnostic (tmux, Docker). Eight plugin slots, all replaceable.
- **Scale:** Manages 20-30 parallel coding agents. Production use at Composio.
- **What works:** Plugin architecture, CI-failure auto-fix loop, reviewer-comment auto-address. Orchestrator agent replaces human in feedback loop with full context on every active session, PR, and CI run.
- **Comparison to BB:** Much larger codebase (40K vs 1.7K LOC). TypeScript vs Elixir. Plugin-heavy vs behaviour-driven. Both use worktrees for isolation. BB is leaner but less plugin-flexible. BB's OTP supervision is more robust than tmux session management.
- **Adopt:** Plugin slot pattern for harness/worker extensibility. CI-failure remediation loop.
- **Avoid:** 40K LOC complexity. 17 plugins is framework territory.

### jayminwest/overstory
- **URL:** https://github.com/jayminwest/overstory
- **Stack:** TypeScript/Node.js
- **Architecture:** Orchestrator -> Team Lead -> Specialist Workers hierarchy. SQLite mail system (WAL mode, 1-5ms latency) for inter-agent messaging. Git worktrees via tmux. Configurable depth limit (default 2).
- **What works:** SQLite-based inter-agent messaging is simple and debuggable. Tiered conflict resolution for merging. Watchdog daemon for health monitoring.
- **Comparison to BB:** Very similar problem space. Both use SQLite for state. Overstory focuses on intra-session multi-agent coordination; BB focuses on issue-to-merge lifecycle. BB has stronger governance (lease/merge authority).
- **Adopt:** SQLite mail/messaging pattern could enable multi-run coordination. Watchdog health monitoring.
- **Avoid:** Excessive hierarchy depth. The warning is real: "compounding error rates, cost amplification, debugging complexity, and merge conflicts are the normal case."

### GitHub Copilot Coding Agent / Agentic Workflows
- **URL:** https://github.blog/changelog/2026-02-13-github-agentic-workflows-are-now-in-technical-preview/
- **Stack:** GitHub Actions, Markdown-defined workflows (not YAML)
- **Architecture:** Agent spins up in GitHub Actions runner. Reviews repo context, related issues, PR discussions, custom instructions. Built-in code scanning, secret scanning, dependency vulnerability checks.
- **Scale:** GA for all paid Copilot subscribers since Sept 2025. Massive scale.
- **What works:** Deep GitHub integration. Security-first (threat model, constrained outputs, comprehensive logging). Multi-source dispatch (Issues, Azure Boards, Jira, Linear, Raycast).
- **Comparison to BB:** GitHub's version of what BB does, but platform-native. BB adds governance (lease authority, merge gates, bakeoffs) that GitHub doesn't. BB is repo-owner operated; GitHub's is platform-operated.
- **Adopt:** Markdown workflow definitions for simple cases. Security scanning integration.
- **Avoid:** Platform lock-in. No model bakeoff capability. No cost tracking granularity.

### langchain-ai/open-swe
- **URL:** https://github.com/langchain-ai/open-swe
- **Stack:** Python, LangGraph, Daytona sandboxes
- **Architecture:** Three-agent pipeline: Manager -> Planner -> Programmer (contains Reviewer sub-agent). Each run in secure Daytona sandbox. Built on LangGraph Platform for long-running workflows with persistence.
- **What works:** Clear agent separation of concerns. Cloud sandbox isolation. Async operation with Slack/Linear invocation. Subagent spawning via `task` tool.
- **Comparison to BB:** Python/LangGraph vs Elixir/OTP. Open SWE is more framework-dependent (LangGraph Platform). BB owns its own state machine; Open SWE delegates to LangGraph. Open SWE's sandbox isolation is stronger (full container per run vs worktree).
- **Adopt:** Structured planning step before coding. Sub-agent delegation pattern.
- **Avoid:** LangGraph Platform dependency. Framework lock-in.

### OpenHands (formerly OpenDevin)
- **URL:** https://github.com/All-Hands-AI/OpenHands
- **Stack:** Python, Docker sandboxes, event-sourced architecture
- **Architecture:** Stateless, event-sourced SDK spanning 4 packages (SDK, Tools, Workspace, Server). All interactions flow as typed events through central hub. Docker container per session. REST/WebSocket server for remote execution. AgentDelegateAction for multi-agent delegation.
- **Scale:** 2.1K+ contributions, 188+ contributors. ICLR 2025 paper. Most popular open-source AI coding agent.
- **What works:** Event-sourced architecture is clean and auditable. Model-agnostic (100+ providers). Built-in security analyzer. QA instrumentation. History restore/pause/resume.
- **Comparison to BB:** OpenHands is more general-purpose (editor, terminal, browser). BB is focused on issue-to-merge lifecycle. OpenHands' event-sourcing aligns with BB's EventBus design. OpenHands has stronger sandbox isolation. BB has stronger governance/authority model.
- **Adopt:** Event-sourcing pattern (BB already does this via EventBus). Security analyzer for agent actions. Pause/resume capability.
- **Avoid:** General-purpose scope creep. 4-package SDK complexity.

---

## 2. Elixir/OTP Agent Frameworks

### agentjido/jido
- **URL:** https://github.com/agentjido/jido
- **Stack:** Elixir, OTP, pure functional core
- **Architecture:** Agents are immutable data structures with single `cmd/2` function. State changes are pure data transformations. Side effects described as directives, executed by OTP runtime. Agent module is pure/stateless; AgentServer is GenServer wrapper.
- **What works:** Clean separation of pure logic and side effects. Deterministic agent logic. Testable without processes. Ecosystem of opt-in packages (jido_ai, jido_workbench).
- **Comparison to BB:** Jido is a general agent framework; BB is a purpose-built orchestrator. Jido's pure-functional core is more composable. BB's RunServer is similar to AgentServer but domain-specific. BB could benefit from Jido's immutable-data-in/directives-out pattern.
- **Adopt:** Pure functional core pattern — separate state transitions from side effects. `cmd/2` -> directives pattern for testability.
- **Avoid:** Premature abstraction. BB's domain-specific approach is correct; don't generalize into a framework.

### jessedrelick/agens
- **URL:** https://github.com/jessedrelick/agens
- **Stack:** Elixir, OTP, Nx.Serving integration
- **Architecture:** Multi-agent workflows via Jobs (sequences of Steps). Each Step employs an Agent. Results passed to next step. Agents have identities for LLM refinement. Tool modules for function calling. Serving can be Nx.Serving or GenServer for API calls.
- **Comparison to BB:** Agens is LangChain-for-Elixir. BB doesn't need this — it orchestrates external agents, not internal LLM chains. Different problem.

### Alloy
- **URL:** https://alloylabs.dev/
- **Stack:** Elixir, 3 dependencies (jason, req, telemetry)
- **Architecture:** Minimal agent engine — completion -> tool-call loop and nothing else. GenServer agents with parallel tool execution. Multi-agent teams. "Engine, not framework."
- **What works:** Radical minimalism. 3 dependencies. Owns nothing you don't need.
- **Comparison to BB:** Alloy handles the LLM loop; BB handles the orchestration lifecycle. Complementary, not competitive. BB's approach of shelling out to `claude-code` is simpler than running its own LLM loop.
- **Adopt:** Philosophy of minimalism. "Engine, not framework."

### nshkrdotcom/synapse
- **URL:** https://github.com/nshkrdotcom/synapse
- **Stack:** Elixir, OTP, Postgres
- **Architecture:** Headless, declarative. Domain-agnostic signal bus. Workflow engine with Postgres persistence. Configurable agent runtime. Ships code review domain as reference. OTP-only (no Phoenix required).
- **Comparison to BB:** Synapse is more general (domain-agnostic signal bus). BB is purpose-built. Synapse uses Postgres; BB uses SQLite. Signal bus pattern is interesting but may be over-engineered for BB's use case.
- **Adopt:** Signal bus concept for decoupled event routing (BB's EventBus already does this simpler).

---

## 3. Key Blog Posts & Thought Leadership

### "Your Agent Framework Is Just a Bad Clone of Elixir" — George Guimaraes
- **URL:** https://georgeguimaraes.com/your-agent-orchestrator-is-just-a-bad-clone-of-elixir/
- **Core thesis:** The actor model Erlang introduced in 1986 IS the agent model AI is rediscovering in 2026. Every pattern Python frameworks build — isolated state, message passing, supervision hierarchies, fault recovery — already exists in BEAM as the runtime itself, not a library.
- **Key insight:** Teams prototype in Python, rewrite in TypeScript for production. Significant share of recent YC agent startups chose TypeScript over Python. But Elixir/BEAM has all of this built in.
- **Validation for BB:** Strong. BB's architecture is vindicated by this analysis. OTP supervision, GenServer state machines, PubSub event broadcasting — these aren't reinventions, they're native capabilities.

### "Why Elixir/OTP Doesn't Need an Agent Framework" (Parts 1 & 2) — goto-code.com
- **URL:** https://goto-code.com/why-elixir-otp-doesnt-need-agent-framework-part-1/
- **Core thesis:** OTP provides all building blocks for LangChain-style behavior. Pattern matching, `with` statements, composing small focused functions. Reach for Tasks, GenServers, Agents only when benefits outweigh complexity.
- **Validation for BB:** BB correctly uses raw OTP primitives rather than adopting an Elixir agent framework. The 1,703 LOC count reflects this.

### "Conductors to Orchestrators" — Addy Osmani (O'Reilly)
- **URL:** https://addyosmani.com/blog/future-agentic-coding/
- **Key framework:** Conductors (human engaged 100% during AI work, interactive) vs Orchestrators (front-loaded + back-loaded human effort, parallel throughput). Engineers evolve from implementer to manager.
- **Relevance to BB:** BB is literally named "conductor" and operates in the orchestrator mode — front-loaded (issue grooming, readiness checks) and back-loaded (governance, merge gates), with the middle delegated to agents. This naming/framing aligns perfectly.

### Production Lessons (Aggregated from Multiple Sources)
- **Cursor's failed approaches:** Equal-status agents with locking (agents held locks too long, 20 agents -> throughput of 2-3). Optimistic concurrency (agents became risk-averse). **Solution:** Planner/Worker/Judge hierarchy.
- **Google DORA 2025:** 90% AI adoption increase correlates with 9% bug rate increase, 91% code review time increase, 154% PR size increase. 67.3% AI-generated PR rejection rate vs 15.6% manual.
- **LangGraph vs CrewAI:** LangGraph 2.2x faster. 8-9x token efficiency differences between frameworks.
- **UC San Diego/Cornell study:** Professional developers retain agency in design, insist on quality attributes, deploy explicit control strategies.

---

## 4. Tech Stack Comparison

| Project | Language | LOC | Persistence | Isolation | Agent Runtime |
|---------|----------|-----|-------------|-----------|---------------|
| **Bitterblossom** | Elixir/OTP | 1,703 | SQLite | Worktrees on sprites | Shell out to claude-code |
| **agent-orchestrator** | TypeScript | 40,000 | — | Worktrees + tmux | Plugin (Claude/Codex/Aider) |
| **Overstory** | TypeScript | ~5K | SQLite | Worktrees + tmux | Claude Code adapters |
| **Open SWE** | Python | ~10K | LangGraph Platform | Daytona containers | LangGraph agents |
| **OpenHands** | Python | ~50K+ | Event store | Docker containers | Custom SDK |
| **GitHub Agentic** | Markdown/YAML | N/A | GitHub | Actions runners | Copilot models |
| **Jido** | Elixir | ~5K | — | OTP processes | GenServer + directives |
| **Synapse** | Elixir | ~3K | Postgres | OTP processes | Signal bus |

---

## 5. Architectural Validation & Recommendations

### BB Got Right
1. **Elixir/OTP for orchestration.** Multiple independent analyses confirm BEAM's actor model IS the agent orchestration model. Every Python/TS framework is reinventing what OTP provides natively.
2. **Minimal LOC.** 1,703 LOC vs 40K (agent-orchestrator) or 50K+ (OpenHands). The "agent trust" principle — letting the builder agent own implementation judgment — eliminates thousands of lines of micro-orchestration.
3. **SQLite persistence.** Simple, embedded, no infrastructure. Overstory independently chose the same.
4. **Event-driven architecture.** EventBus + PubSub aligns with OpenHands' event-sourced pattern. Industry-validated.
5. **Behaviour-driven extensibility.** Worker/Tracker/Harness/CodeHost behaviours are cleaner than plugin architectures.
6. **Single-repo focus.** Avoids the complexity trap of multi-repo orchestration.

### BB Should Consider Adopting
1. **CI-failure remediation loop.** Agent-orchestrator's auto-fix on CI failure is high-value. BB currently handles this in the builder prompt but could make it a first-class conductor capability.
2. **Pause/resume for runs.** OpenHands supports this. Long-running agent runs benefit from operator intervention points beyond the current phase gates.
3. **Security scanning gate.** GitHub's approach of running code scanning, secret scanning, and dependency checks before PR creation is a governance improvement.
4. **Structured planning step.** Open SWE's explicit Planner agent before coding could improve success rates. BB could add a "planning" sub-phase in the building state.
5. **Health monitoring/watchdog.** Overstory's watchdog daemon pattern. BB's sprites need liveness probes beyond the current status checks.
6. **Pure functional core.** Jido's pattern of immutable state + directives improves testability. BB's RunServer could benefit from separating pure state transitions from side effects.

### BB Should Avoid
1. **Framework-ification.** Jido, Agens, Synapse all generalize. BB's domain-specific design is correct. Don't build a "framework for building agent orchestrators."
2. **Excessive hierarchy.** Overstory warns against it. BB's flat Orchestrator -> RunServer -> Worker is the right depth.
3. **Platform dependency.** Open SWE on LangGraph Platform, GitHub Agentic on Actions. BB's self-hosted model is a strength.
4. **Plugin proliferation.** 17 plugins (agent-orchestrator) is framework territory. BB's 4 behaviours are sufficient.
5. **Container-per-run isolation.** Docker/Daytona sandboxes are stronger isolation but massive overhead for a single-repo factory. Worktrees on trusted sprites are the right tradeoff.

### Open Questions
1. **Is the TypeScript rewrite trend relevant?** Multiple sources note Python->TypeScript for production. BB skipped both and went to Elixir. This seems like the right call per George Guimaraes' analysis.
2. **Multi-model routing.** GitHub's model picker (GPT-5, Claude, Gemini) per task. BB's bakeoff feature covers this but as experimentation, not production routing. Worth promoting?
3. **MCP integration.** Anthropic's Model Context Protocol is becoming standard for tool integration. BB currently shells out to claude-code which handles MCP internally. No action needed unless BB runs its own LLM loop.

---

## Sources

- [ComposioHQ/agent-orchestrator](https://github.com/ComposioHQ/agent-orchestrator)
- [Open-sourcing Agent Orchestrator: 30 Parallel Agents](https://pkarnal.com/blog/open-sourcing-agent-orchestrator)
- [jayminwest/overstory](https://github.com/jayminwest/overstory)
- [GitHub Agentic Workflows Technical Preview](https://github.blog/changelog/2026-02-13-github-agentic-workflows-are-now-in-technical-preview/)
- [GitHub Copilot Coding Agent 101](https://github.blog/ai-and-ml/github-copilot/github-copilot-coding-agent-101-getting-started-with-agentic-workflows-on-github/)
- [langchain-ai/open-swe](https://github.com/langchain-ai/open-swe)
- [Introducing Open SWE (LangChain blog)](https://blog.langchain.com/introducing-open-swe-an-open-source-asynchronous-coding-agent/)
- [OpenHands SDK Paper](https://arxiv.org/abs/2511.03690)
- [OpenHands Platform (ICLR 2025)](https://arxiv.org/abs/2407.16741)
- [agentjido/jido](https://github.com/agentjido/jido)
- [jessedrelick/agens](https://github.com/jessedrelick/agens)
- [Alloy — Agent Engine for Elixir](https://alloylabs.dev/)
- [nshkrdotcom/synapse](https://github.com/nshkrdotcom/synapse)
- [Your Agent Framework Is Just a Bad Clone of Elixir](https://georgeguimaraes.com/your-agent-orchestrator-is-just-a-bad-clone-of-elixir/)
- [Why Elixir/OTP Doesn't Need an Agent Framework (Part 1)](https://goto-code.com/why-elixir-otp-doesnt-need-agent-framework-part-1/)
- [Why Elixir/OTP Doesn't Need an Agent Framework (Part 2)](https://goto-code.com/why-elixir-otp-doesnt-need-an-agent-framework-part-2/)
- [Conductors to Orchestrators — Addy Osmani](https://addyosmani.com/blog/future-agentic-coding/)
- [AI Coding Agents 2026: Coherence Through Orchestration](https://mikemason.ca/writing/ai-coding-agents-jan-2026/)
- [Orchestrating Multi-Step Agents: Temporal/Dagster/LangGraph](https://www.kinde.com/learn/ai-for-software-engineering/ai-devops/orchestrating-multi-step-agents-temporal-dagster-langgraph-patterns-for-long-running-work/)
- [Temporal for AI Agents](https://temporal.io/solutions/ai)
- [Modal: Open-source AI Agents](https://modal.com/blog/open-ai-agents)
- [awesome-agent-orchestrators](https://github.com/andyrewlee/awesome-agent-orchestrators)
- [awesome-cli-coding-agents](https://github.com/bradAGI/awesome-cli-coding-agents)
- [SWE-bench](https://github.com/SWE-bench/SWE-bench)
- [Anthropic 2026 Agentic Coding Trends Report](https://resources.anthropic.com/hubfs/2026%20Agentic%20Coding%20Trends%20Report.pdf)
