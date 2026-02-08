# 1.0.0 (2026-02-08)


### Bug Fixes

* allow git push --tags in destructive command guard ([#59](https://github.com/misty-step/bitterblossom/issues/59)) ([6ae9534](https://github.com/misty-step/bitterblossom/commit/6ae95346f7d4f61b908986b2378aefbc28eb3222))
* **ci:** support Go 1.25 linting and deflake supervisor test ([#65](https://github.com/misty-step/bitterblossom/issues/65)) ([1128321](https://github.com/misty-step/bitterblossom/commit/1128321a6708c15501c879b7c3dd22bccc445b62))
* **dispatch:** decouple from removed contracts types ([43650bf](https://github.com/misty-step/bitterblossom/commit/43650bf8127987e679d8c9ec823ff4e76084a38e))
* **gitignore:** ignore python cache artifacts ([#61](https://github.com/misty-step/bitterblossom/issues/61)) ([03e7438](https://github.com/misty-step/bitterblossom/commit/03e74388ac0767583b341f67cb385ff4468e03c0)), closes [#5](https://github.com/misty-step/bitterblossom/issues/5)
* parse dispatch times as UTC in watchdog ([058600a](https://github.com/misty-step/bitterblossom/commit/058600a11f310b0938f3a378a4fafa911a104c2f))
* Ralph Loop v2 must check merge conflicts before completing ([8a69f3a](https://github.com/misty-step/bitterblossom/commit/8a69f3a269e4a6ff2f3513d57673aff49f5ceeb1)), closes [#32](https://github.com/misty-step/bitterblossom/issues/32)
* **release:** add semantic-release config for non-Node project ([#72](https://github.com/misty-step/bitterblossom/issues/72)) ([e0759f7](https://github.com/misty-step/bitterblossom/commit/e0759f71f695c9328375b4e61e21b477320a4cd6))
* replace script -q -c with piped claude invocation in dispatch.sh ([e91043c](https://github.com/misty-step/bitterblossom/commit/e91043c11ef9a817d52d358348ab3fc7b5d5342c))
* **scripts:** centralize remote workspace constant ([#47](https://github.com/misty-step/bitterblossom/issues/47)) ([5d374c4](https://github.com/misty-step/bitterblossom/commit/5d374c4ce45661c2bbef2abccaa2f0dd926f8c29))
* **scripts:** line-buffer Ralph loop output for real-time tailing ([#33](https://github.com/misty-step/bitterblossom/issues/33)) ([bb9e159](https://github.com/misty-step/bitterblossom/commit/bb9e159f635a8795edd895ce16451cf050125d32)), closes [#7](https://github.com/misty-step/bitterblossom/issues/7)
* **security:** close guard bypass via subshells and refs/heads/ ([#31](https://github.com/misty-step/bitterblossom/issues/31)) ([30fe14a](https://github.com/misty-step/bitterblossom/commit/30fe14ad070e07703a910394821a514de5888354)), closes [#24](https://github.com/misty-step/bitterblossom/issues/24) [#25](https://github.com/misty-step/bitterblossom/issues/25) [subshell/#24](https://github.com/misty-step/bitterblossom/issues/24) [refs/heads/#25](https://github.com/refs/heads//issues/25) [#35](https://github.com/misty-step/bitterblossom/issues/35) [#36](https://github.com/misty-step/bitterblossom/issues/36)
* **security:** scrub leaked API key + add secret detection ([#28](https://github.com/misty-step/bitterblossom/issues/28)) ([9bc07e9](https://github.com/misty-step/bitterblossom/commit/9bc07e92ce26ccfc9db31251e4c72ac4197e1bea)), closes [#23](https://github.com/misty-step/bitterblossom/issues/23)


### Features

* add 7 specialist sprite personas and v2 composition ([#48](https://github.com/misty-step/bitterblossom/issues/48)) ([6cb22c3](https://github.com/misty-step/bitterblossom/commit/6cb22c32bf8f84424675736e8799c18d3a5e9aac))
* add beaker science sprite persona ([01fa5fe](https://github.com/misty-step/bitterblossom/commit/01fa5fef24edc83a0263e60531a679f7496a904f))
* add tail-logs.sh for real-time sprite log visibility ([9131fa3](https://github.com/misty-step/bitterblossom/commit/9131fa314c18de560508fd7a4ccb231ab8e5c14c))
* **base:** add planning phase + lessons loop to sprite workflow ([aae9534](https://github.com/misty-step/bitterblossom/commit/aae953431d9ae9301fcc7359e7ce985617bcb7e1))
* **bb:** add go run-task and check-fleet commands ([83816ce](https://github.com/misty-step/bitterblossom/commit/83816ce23055b1bfc462776bb1b3a7d39f5c3734))
* bot-account auth foundation for sprite GitHub isolation ([#68](https://github.com/misty-step/bitterblossom/issues/68)) ([2caf5e4](https://github.com/misty-step/bitterblossom/commit/2caf5e409cae22f47ac07ad33205eee9e62ffe1b))
* **contracts:** versioned JSON/NDJSON contracts and machine error semantics ([#63](https://github.com/misty-step/bitterblossom/issues/63)) ([24fb2ce](https://github.com/misty-step/bitterblossom/commit/24fb2ce9f9e5e9c44751475d9a4eb3f7f6b2fab7)), closes [#41](https://github.com/misty-step/bitterblossom/issues/41)
* declarative sprite factory for Claude Code agent fleet ([a6b10d4](https://github.com/misty-step/bitterblossom/commit/a6b10d4e29d27e51f71abe07b2a2f78d866edff5))
* integrate Landfall release pipeline ([#64](https://github.com/misty-step/bitterblossom/issues/64)) ([0304dfb](https://github.com/misty-step/bitterblossom/commit/0304dfbad4c524900a5f274af486f806c4994c75))
* **ooda:** orientation phase + learnings feedback for sprites ([16a9728](https://github.com/misty-step/bitterblossom/commit/16a97287085ca3c1932bb5595755271028186041))
* Ousterhout hardening — define errors out of existence ([dc55081](https://github.com/misty-step/bitterblossom/commit/dc55081fe045043ebf6d78068471dc9e7e58bff0))
* Phase 2 core abstractions — sprite state machine, fleet reconciler, JSONL events ([#56](https://github.com/misty-step/bitterblossom/issues/56)) ([53af092](https://github.com/misty-step/bitterblossom/commit/53af092da9cd42a06d9300e3e30a0efaae1c07b3)), closes [#52](https://github.com/misty-step/bitterblossom/issues/52)
* Phase 2 integration — fleet reconciler, event protocol, agent supervisor ([#62](https://github.com/misty-step/bitterblossom/issues/62)) ([4bb4f70](https://github.com/misty-step/bitterblossom/commit/4bb4f70a88e17e5c737007e994a512b08a53ebbf))
* port dispatch and watchdog to Go state machines ([#43](https://github.com/misty-step/bitterblossom/issues/43)) ([#67](https://github.com/misty-step/bitterblossom/issues/67)) ([063e30e](https://github.com/misty-step/bitterblossom/commit/063e30e0dd327b4ade9f32ea8b6271269c34e7c6))
* scaffold Go control plane foundation ([#51](https://github.com/misty-step/bitterblossom/issues/51)) ([995e17f](https://github.com/misty-step/bitterblossom/commit/995e17f878ec37a4139c83dd0aa1fc106b84ea5d))
* **scripts:** drive sprite enumeration from composition YAML ([#34](https://github.com/misty-step/bitterblossom/issues/34)) ([2c1d041](https://github.com/misty-step/bitterblossom/commit/2c1d041f966d42f59b34e4657c31dae9911467af)), closes [#18](https://github.com/misty-step/bitterblossom/issues/18)
* **scripts:** sprite fleet watchdog — auto-detect and recover dead agents ([1d08d4e](https://github.com/misty-step/bitterblossom/commit/1d08d4e5946b0ede4d7923983abd3b709022fe09))
* watchdog-v2 — auto-action on sprite signals ([c49b2dd](https://github.com/misty-step/bitterblossom/commit/c49b2dd879052476e8b3ba88807f12c6d3746cd1))
