Reference documents for the `e2e-test` skill. These are read-only inputs consulted during shakedown execution — not modified by the skill.

### Files

*   **`evaluation-criteria.md`**: Per-phase PASS / FRICTION / FAIL rubric covering all nine shakedown phases (Build through Findings Report). Defines the scoring thresholds for each criterion and maps aggregate scores to overall health grades (A through D).

*   **`friction-taxonomy.md`**: Classification system for findings, organized into nine categories (F1 Silent Failure through F9 Infrastructure Fragility). Each category carries a minimum severity floor (P0-P3) that prevents systemic issues from being downgraded. Used to assign category and severity when writing findings in the report template.
