The `/sprites` directory serves as the configuration hub for a multi-agent orchestrated engineering system referred to as the "fae engineering court." This directory contains a collection of Markdown files, each defining a specialized AI persona (a "sprite") with distinct domain expertise, operational philosophies, and interaction protocols. The repo-root `WORKFLOW.md` file is the phase contract these personas are expected to follow.

### Architecture and Design Patterns
The architecture is modular and role-based, designed for a delegated workflow where tasks are routed based on specific "Routing Signals." The system follows a collaborative "Team Context" model, where individual agents are aware of their peers' specialties to facilitate handoffs (e.g., Foxglove identifying a bug and handing it to Clover for regression testing). 

Each sprite configuration adheres to a standardized schema:
*   **Metadata (Frontmatter):** Defines the agent's name, skill sets (for example imported autonomy skills such as `shape`, `build`, `debug`, `pr-fix`, or `autopilot`), and execution parameters such as `permissionMode: bypassPermissions` and `model: inherit`.
*   **Operational Philosophy:** Establishes the high-level principles guiding the agent's logic (e.g., "Infrastructure as Code" for Fern or "Reproduce First" for Foxglove).
*   **Working Patterns:** Provides specific algorithmic or procedural instructions for task execution.
*   **Routing Signals:** Defines the triggers and keywords used by the orchestrator (OpenClaw) to dispatch tasks to the correct agent.
*   **Memory Management:** Each agent is required to maintain a `MEMORY.md` file in its working directory to persist state, architectural decisions, and domain-specific gotchas.

### Key File Roles
The directory is partitioned into several functional domains:
*   **Infrastructure & Operations:** `fern.md` (CI/CD, Docker, Deployment, skill-backed remediation).
*   **Core Development:** `bramble.md` (Backend/Data with `shape`/`build`/`pr`), `willow.md` (Frontend/UI with build/polish skills), and `moss.md` (System Architecture with autopilot/shaping/review skills).
*   **Quality & Security:** `thorn.md` (General Quality with debug/fix/polish), `hemlock.md` (Security Auditing), `clover.md` (Test Writing), and `foxglove.md` (Bug Investigation).
*   **Maintenance & Evolution:** `rowan.md` (Refactoring), `nettle.md` (Technical Debt), and `hazel.md` (Issue Grooming).
*   **Support & Research:** `sage.md` (Documentation) and `beaker.md` (Scientific Experimentation/Adversarial Testing).

### Dependencies and Technical Constraints
*   **Orchestration Layer:** The files are dependent on the **OpenClaw** routing engine to interpret the Markdown-based instructions and "Routing Signals," and on repo `WORKFLOW.md` to define the canonical phase order.
*   **Persistence:** The agents rely on a **local memory pattern**, specifically requiring the existence of a `MEMORY.md` file within their execution context to ensure continuity across sessions.
*   **Environment Integration:** Several sprites (Fern, Bramble, Beaker) assume access to external tools and libraries including Docker, Fly.io, GitHub Actions, and Python-based data science stacks (scipy, pandas).
*   **Permissions:** Most agents are configured with `bypassPermissions`, indicating they operate with elevated execution privileges within the development environment.
*   **Contextual Awareness:** There is a high degree of inter-dependency mentioned in the "Team Context" sections; modifying the role or scope of one sprite may necessitate updates to the routing logic of its peers to prevent coordination failures.