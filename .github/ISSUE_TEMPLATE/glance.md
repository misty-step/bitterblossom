This directory contains GitHub issue templates for Bitterblossom.

### Templates

*   **`ralph-ready-spec.md`**: The primary issue template for work intended to be dispatched to a sprite via the Ralph loop. Issues created from this template are auto-labeled `ralph-ready` and structured to be self-contained: they include a summary, background, functional and non-functional requirements, a file modification table, and explicit acceptance criteria. The format is designed so that the issue body can be embedded verbatim in a dispatch prompt — sprites cannot access GitHub directly, so the issue must carry all necessary context.

### Usage Convention

Issues created from this template are the canonical input to `bb dispatch`. When dispatching, fetch the full issue body with `gh issue view <NUMBER>` and embed it in the prompt string. Do not rely on `--issue <NUMBER>` flags — sprites have no GitHub API access.
