# SDLC reflex-agent plane design

Date: 2026-06-15

## Goal

Design the next Bitterblossom step toward an Amjad/Replit-style loop: short
operator outcomes trigger a durable plane of specialized agents, verifier
feedback, and stage-specific fix prompts. The answer should preserve the
current event-plane spine instead of rebuilding a semantic SDLC engine in Rust.

## Answer

Bitterblossom already has the important primitives: file-owned agents, tasks,
manual/webhook/cron triggers, submissions, verdict storms, gates, run evidence,
and a manual builder dispatch lane. The missing product layer is a **lifecycle
reflex pack**: predefined triggers and task cards for SDLC events such as PR
ready, submission opened, CI failed, gate blocked, deploy smoke failed, and
production incident observed.

The spine should stay mechanical. Lifecycle intelligence belongs in task cards,
agent launch contracts, and validated templates. The plane records and routes;
agents decide and explain.

## Research Notes

Replit's public Agent 3 material validates the loop shape. Agent 3 periodically
tests apps with a browser, reports what it tried, and fixes detected issues.
Replit also describes longer autonomous sessions with self-supervision and
agent/automation generation as first-class product behaviors:
https://replit.com/blog/introducing-agent-3-our-most-autonomous-agent-yet.

Replit's verification writeup names the core failure mode for autonomous
builders: outputs that look correct but are not wired up. Their remedy is
shift-left verification using code execution plus browser automation, because
autonomy makes mistakes compound:
https://replit.com/blog/automated-self-testing.

Replit's decision-time guidance is especially relevant to Bitterblossom. It
argues that long trajectories are not controlled well by static prompt piles;
the execution environment should inject feedback at the decision point:
https://replit.com/blog/decision-time-guidance.

Microsoft's Conductor post is the best architectural comparator. For known
multi-agent workflows, it argues for deterministic, version-controlled routing
instead of an LLM deciding the workflow graph every time:
https://opensource.microsoft.com/blog/2026/05/14/conductor-deterministic-orchestration-for-multi-agent-ai-workflows/.

Addy Osmani's orchestration notes line up with the operator ergonomics:
parallel agents work when there are separate contexts, scoped responsibility,
quality gates, and lifecycle hooks such as task-completed review:
https://addyosmani.com/blog/code-agent-orchestra/.

The cautionary side is real. Augment's survey emphasizes that multi-agent
systems add coordination overhead, cost, and distributed-system observability
problems; they fit best when work is parallelizable or domain-specialized, and
cross-cutting review benefits more than sequential debugging:
https://www.augmentcode.com/guides/single-agent-vs-multi-agent-ai.

OpenRouter model facts were refreshed from `https://openrouter.ai/api/v1/models`
on 2026-06-15. Current useful facts:

- `moonshotai/kimi-k2.7-code`: 262k context, coding-focused, $0.75/M input
  and $3.50/M output in the API catalog, tools and structured output support.
- `deepseek/deepseek-v4-pro`: 1M context, $0.435/M input and $0.87/M output,
  tools and structured output support.
- `deepseek/deepseek-v4-flash`: 1M context, $0.098/M input and $0.196/M
  output, tools and structured output support.
- `z-ai/glm-5.1`: 203k context, $0.98/M input and $3.08/M output, tool and
  reasoning parameters.
- `z-ai/glm-5.2`: visible on OpenRouter's Z.ai page on 2026-06-15 as a
  released model, with API access releasing 2026-06-16; not present in the
  API catalog on 2026-06-15.
- `openrouter/fusion`: suitable as an architecture/research council, not a
  drop-in coding model. OpenRouter says coding models should call it
  selectively for questions worth multi-model deliberation.

## Proposed Shape

Add a lifecycle reflex pack after the current branch:

1. **Lifecycle triggers as data**
   - `pull_request` ready/synchronize: run review or open/refresh a submission.
   - `check_suite` failed: run a CI-diagnose task with logs as evidence and a
     deterministic builder-packet recommendation.
   - `submission.opened` or explicit manual command: run verdict storm members.
   - `gate.blocked`: run a fix-prompt generator that writes a bounded packet for
     the builder; it does not edit code.
   - `deployment_status` or scheduled smoke failure: run production verifier and
     incident/diagnose task.

2. **Orchestrator as task, not spine**
   - A `lifecycle-orchestrator` task reads event payload plus plane state and
     emits a plan/report or opens follow-up runs with deterministic commands.
   - It has no magic Rust privileges. If it needs to launch follow-up runs, the
     first slice can make it report exact `bb run` commands instead of executing
     them. Later slices can grant a narrow plane token for run creation.

3. **Focused reflex agents**
   - `review-coordinator`: PR diff review, already present.
   - `verifier`: deterministic local gate, already present.
   - `correctness`, `security`, `simplification`, `product`: independent
     verdict domains, already present.
   - `ci-diagnoser`: consumes failed check logs and produces a fix packet.
   - `prod-verifier`: browser/API smoke against deployed surface.
   - `fix-prompt-generator`: converts gate findings into a builder packet.
   - `gardener`: mines ledger and opens falsifiable backlog tickets, already
     present.

4. **Feedback packet contract**
   - All non-builder reflex agents write durable JSON reports with:
     `event`, `task`, `repo`, `rev`, `claim`, `evidence`, `suggested_next_run`,
     `cost`, `artifact_paths`, and `residual_risk`.
   - Fix prompts are artifacts, not hidden chat context.

6. **First runnable slice**
   - Implement `ci-diagnose` as a task/agent/card packet with a manual trigger
     for dogfood and a `check_suite.completed` webhook filtered to failed
     GitHub Actions suites for `misty-step/bitterblossom`.
   - The agent emits `REPORT.json` and may recommend an exact `bb run build`
     command, but the first slice does not grant run-creation, comment, merge,
     deploy, or task-parking authority.

5. **Model composition**
   - Default reflex review lanes stay cheap OpenRouter/Pi.
   - Use DeepSeek V4 Flash for high-volume triage and simplification.
   - Use DeepSeek V4 Pro for long-context correctness/security.
   - Use Kimi K2.7 Code for coding-aware coordinator/build support where a
     current local smoke exists.
   - Treat GLM 5.2 as page-visible/API-pending until the API catalog and local
     smoke prove it.
   - Use Fusion only for architecture/research decisions where its extra
     multi-model cost is justified.

## Boundaries

Do not add a Rust SDLC state machine such as
`plan -> implement -> review -> fix -> land`.

Do not add workload-specific branches to dispatch, queue, substrate, recovery,
or gate arithmetic.

Do not make an LLM the hidden router for known lifecycle events. The event
graph should be deterministic, visible, and versioned in files.

Do not grant reflex agents merge authority. Builder and fix agents can push
branches; gates and humans decide landing.

Do not treat more agents as automatically better. Use parallel agents for
independent domains, read-heavy research, and cross-cutting review; keep
sequential debugging and tightly coupled implementation single-lane.

## Backlog Decision

This is not already fully shaped. Existing backlog `053` owns the stable
agent-facing contract; `055` owns template portfolios; `056` owns telemetry.
The lifecycle reflex layer deserves its own ticket because it connects SDLC
events to the existing plane primitives without changing those lower-level
contracts.

New ticket: `061-sdlc-lifecycle-reflex-pack`.
