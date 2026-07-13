export const SPECS = [
  {
    id: "editorial-register",
    label: "The Workflow Register",
    philosophy: "Editorial Calm",
    move:
      "Make the configured workflow roster a ruled editorial register, then preserve that register as the quiet index beside one configuration desk; the sole primary action opens the current run as a temporary second plane before its scoped evidence.",
    layout: "roster-detail",
    roster: "rows",
    detail: "configuration-first",
    navigation: "rail",
    density: "quiet",
    mobile:
      "At 390px the operator sees the workflow register first, opens one workflow into a full-screen configuration task, and invokes its current run or evidence only as the next contextual view.",
    accent: "rare",
  },
  {
    id: "editorial-focus",
    label: "The Working Folio",
    philosophy: "Editorial Calm",
    move:
      "Invert the persistent master-detail assumption: selection replaces the roster with a single topology-first folio whose configured graph is the fixed front matter, whose current run is a deliberate overlay, and whose evidence is an appendix reached from that run.",
    layout: "focus-stack",
    roster: "index",
    detail: "topology-first",
    navigation: "bottom-contextual",
    density: "compact",
    mobile:
      "At 390px the operator moves in strict order from workflow index to configured topology to an explicitly selected run and finally to its evidence, with exactly one of those tasks occupying the viewport.",
    accent: "active-only",
  },
];
