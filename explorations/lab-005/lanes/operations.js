export const SPECS = [
  {
    id: "operations-command-ledger",
    label: "The Context Command Ledger",
    philosophy: "Operations Ledger",
    move:
      "Use the configured-workflow ledger as the entry register, then replace global navigation with a contextual command strip after selection: Configuration, Current run, and Evidence are explicit transitions into separate task surfaces, so runtime and immutable proof never masquerade as configured state.",
    layout: "command-strip",
    roster: "ledger",
    detail: "run-overlay",
    navigation: "bottom-contextual",
    density: "compact",
    mobile:
      "At 390px the operator selects a workflow from the ledger, lands on configuration, and uses the pinned contextual strip to switch the entire viewport to Current run or Evidence.",
    accent: "active-only",
  },
  {
    id: "operations-topology-register",
    label: "The Topology Register",
    philosophy: "Operations Ledger",
    move:
      "Pair a quiet row roster with one stable topology-first configuration register under top navigation; the selected workflow remains available as restrained index context while the configured route owns the work surface, and runtime or evidence replaces that surface only on request.",
    layout: "split-register",
    roster: "rows",
    detail: "topology-first",
    navigation: "top",
    density: "quiet",
    mobile:
      "At 390px the row roster is the first task, selection opens only the configured topology, and Current run then Evidence each replace it as their own full-viewport task.",
    accent: "rare",
  },
];
