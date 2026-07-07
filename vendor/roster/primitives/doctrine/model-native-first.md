# Model-Native First (AI where it wins, mechanism where it must)

Operator doctrine, hard-won twice (gazette heuristics; the bridge hint-array
correction): at a semantic seam — classification, triage, synthesis, routing,
review — the options are a declared field or a model call, NEVER keyword
heuristics. Keyword rules at judgment seams are mechanism squatting where a
model belongs.

Standing watchfulness: whenever you are about to write judgment-shaped
imperative logic (roughly: any branch a smart human would decide differently
with context), stop and ask whether a twenty-line prompt does it better. It
usually does. Conversely, policy, persistence, approval, gating, and anything
that must be provable stays deterministic — a model never guards its own
output (external gates, always: compaction erodes prompt-embedded constraints).

Routing is evidence, not faith: default to a cheap model, escalate on measured
failure, record which judge ran. Failures are routing data for the capability
ledger. The seam-judgment benchmark in crucible exists to measure exactly this
skill — knowing WHEN a model beats mechanism.

## The placement test (run it before adding mechanism)

Three questions, in order, when deciding classical vs model:

1. **Who consumes the output?** Deterministic code (parser, loader, CI,
   filesystem) → classical is correct. A model or a human → carry rich
   context; schema, enums, and keyword heuristics are the failure mode.
2. **Is it must-fire-every-time policy at a trust boundary** (secrets,
   destructive commands, permissions, persistence)? → classical on purpose.
3. **Does correctness require reading meaning?** → model, always. The tell
   is regex or string-matching aimed at natural language.

Mechanism-adding diffs — a new crate, CLI subcommand, hook, state machine,
regex-over-natural-language, or a mechanism-budget raise — summon the ai-scout
lens in review (roster-929): artifact-only, answering one question — should
any of this be a declaration or model judgment instead?
