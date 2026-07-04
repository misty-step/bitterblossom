# roster

Roster is the agent declaration repository for Misty Step Factory. It keeps
agent identities, prompts, model policy, primitive references, and materializers
in one plain-file tree.

P0 provides:

- `agents/<name>/role.yaml` and `instructions.md` declarations.
- `roster list`, `roster show <agent>`, `roster materialize <agent> --harness <target>`, and `roster brief <agent>`.
- Reference-only primitive indexes for skills and MCP servers.

```sh
cargo run -p roster-cli -- list
cargo run -p roster-cli -- show cerberus
cargo run -p roster-cli -- materialize cerberus --harness codex
cargo run -p roster-cli -- brief cerberus
```
