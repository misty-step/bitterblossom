This directory contains the GitHub Actions CI/CD workflow definitions for Bitterblossom.

### Workflows

*   **`ci.yml`**: The primary quality gate, triggered on pull requests and pushes to `master`. Runs four parallel jobs: `shellcheck` (validates all `.sh` scripts at error severity), `python-hooks` (lints and type-checks the Python safety hooks in `base/hooks/`), `go-checks` (builds the `bb` CLI and runs all Go tests), and `yaml-lint` (validates workflow YAML syntax). Uses concurrency groups to cancel stale runs on rapid pushes.

*   **`cerberus.yml`**: Runs the Cerberus AI Code Review Council on pull requests. Fans out to five independent reviewer jobs (APOLLO/correctness, ATHENA/architecture, SENTINEL/security, VULCAN/performance, ARTEMIS/maintainability), each powered by the `misty-step/cerberus@v2` action. A final "Council Verdict" job aggregates results and fails the PR if any reviewer votes FAIL. Restricted to non-fork PRs to protect secrets.

*   **`release.yml`**: Automated semantic release pipeline, triggered when CI passes on `master` or via manual `workflow_dispatch`. Delegates to `misty-step/landfall@v1` which uses an LLM to generate release notes and create GitHub releases with semantic version tags.

*   **`secret-detection.yml`**: Runs TruffleHog on every PR and master push to detect accidentally committed credentials. Uses pinned action SHAs for supply-chain integrity. The `--only-verified` flag limits reports to live, confirmed secrets — run locally without the flag to catch rotated or unverifiable ones.

### Key Constraints

*   **Secret access**: Only non-fork PRs (`github.event.pull_request.head.repo.full_name == github.repository`) receive `OPENROUTER_API_KEY` or `GH_RELEASE_TOKEN`. Fork PRs run CI without Cerberus.
*   **Concurrency**: All workflows use `cancel-in-progress: true` (except `release.yml`) to avoid queuing stale runs during rapid iteration.
*   **Landfall dependency**: The release workflow requires `GH_RELEASE_TOKEN` (a PAT with `contents: write`) and `OPENROUTER_API_KEY` to be set as repository secrets.
