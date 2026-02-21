The `/compositions` directory serves as the configuration and orchestration layer for the Bitterblossom project. It defines the "Fae Court" model—a collection of LLM-powered agents, or "sprites," organized into specific functional roles. The directory utilizes a versioned YAML architecture to manage the evolution of agent personas, deployment strategies, and multi-model provider routing.

### Architecture and Organization
The directory is structured around iterative configuration files that define the operational environment and agent hierarchies. Each composition file follows a standardized schema divided into four functional blocks:
*   **Base:** Establishes the global execution environment, including shared instructions (`CLAUDE.md`), Python-based lifecycle hooks (e.g., `destructive-command-guard.py`), core skills (testing, git mastery), and standardized command sets.
*   **Sprites:** Defines the individual agent roster. Each sprite entry maps to a specific markdown definition and includes metadata for routing, such as "preferences" (e.g., "Systems & Data"), "philosophies," and "strengths."
*   **Deployment:** Manages the operational footprint of the composition. It defines concurrency limits, default active sets, and "rotation pools" for swapping specialized sprites in and out based on project priorities (e.g., security sprints or documentation pushes).
*   **Experiment:** Contains metadata for performance tracking, including hypotheses regarding sprite specialization and metrics for evaluating task completion and routing accuracy.

### Key File Roles
*   **v1.yaml:** The baseline composition establishing the five core "generalist-specialist" sprites (Bramble, Willow, Thorn, Fern, and Moss).
*   **v2.yaml:** An expanded orchestration introducing seven "deep specialist" sprites (e.g., Hemlock for security, Sage for documentation) and the deployment rotation logic.
*   **v3-multi-provider.yaml:** The current architectural evolution supporting heterogeneous LLM backends. It enables sprite-level provider configuration, routing specific tasks to different models (e.g., Kimi via Moonshot or Claude via OpenRouter) based on the sprite's specialization.
*   **archive/:** A "cold storage" subdirectory for deprecated or superseded compositional logic. It maintains the historical record of creative and technical iterations but is decoupled from the active runtime.

### Dependencies and Technical Constraints
The compositions rely on the Bitterblossom core framework to parse YAML configurations and execute the referenced hooks and commands. Version 3 introduces a critical dependency on external LLM providers (Moonshot and OpenRouter) and requires specific API environment variables for model access. The system assumes a specific directory structure where relative paths for `sprites/`, `base/hooks/`, and `base/skills/` must remain consistent with the YAML definitions.

### Important Gotchas
*   **Bit Rot in Archive:** Files within the `/archive` directory frequently reference deprecated APIs, obsolete environment variables, or legacy versions of the Bitterblossom framework that may no longer be executable.
*   **Broken Path References:** Compositions rely heavily on relative file paths; moving definition files or hooks without updating the corresponding YAML files will cause runtime failures.
*   **Namespace and Indexing:** Archived configurations are not automatically excluded from global searches or automated indexing tools, which can lead to naming collisions or confusion between legacy and active agent definitions.
*   **Environment Incompatibility:** Python-based hooks referenced in older compositions may not be syntactically valid or compatible with the current environment's interpreter version.