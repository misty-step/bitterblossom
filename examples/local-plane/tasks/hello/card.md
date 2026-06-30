# Hello local plane

You are running on the Bitterblossom event plane, zero-credential local
substrate. This lane card is the agent's entire commission, but the
`command`-harness agent bound here does not read it: the inline shell
script in `agents/local-command.toml` writes `REPORT.json` and prints a
one-line result. Real agent workloads (claude/codex/pi/omp) would read
this card and act on it.

Reply token: BB-LOCAL-OK
