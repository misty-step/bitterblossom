The `/base/commands` directory serves as a centralized repository for standardized operational procedures and development workflows. Its architecture relies on structured Markdown documentation to define executable-like playbooks that guide developers through complex or sensitive version control and lifecycle tasks.

### Key File Roles
*   **`commit.md`**: Acts as the primary protocol for workspace management and code integration. It defines a six-phase lifecycle—Analyze, Tidy, Group, Create, Quality Check, and Push—to ensure repository hygiene. The file enforces the **Conventional Commits** specification and provides strict templates for commit messages, including co-authorship metadata and imperative-mood subject lines.

### Architecture and Workflow Integration
The directory functions as a "manual-as-code" layer, bridging local development actions with project-specific standards. The workflows are designed to be tool-agnostic yet specific in execution, utilizing standard shell commands and git primitives to maintain a linear and legible project history.

### Dependencies
*   **Version Control**: Heavily dependent on **Git CLI** for status analysis, diffing, and branch management.
*   **Runtime/Build Tools**: Relies on the presence of **pnpm** or **npm** to execute quality gates.
*   **Validation Scripts**: Specifically looks for `lint`, `typecheck`, and `test` scripts defined within the project's `package.json`.

### Gotchas
*   **Branch Protection**: The workflow explicitly prohibits direct pushes to `main` or `master` branches, requiring a feature-branch approach.
*   **History Management**: Force pushing is strictly forbidden; the protocol mandates a `fetch` and `rebase` strategy during the push phase to prevent merge commits and resolve conflicts locally.
*   **Silent Failures**: Quality checks (linting/testing) use `|| true` or redirection to `/dev/null` in some contexts, potentially masking non-critical warnings unless the developer manually inspects the output.
*   **Metadata Requirements**: Commit messages require a specific `Co-Authored-By` trailer, which is necessary for internal attribution and compliance.