// Real production-plane snapshots captured 2026-07-03, inlined so the
// viewer works over plain http.server without fetch/CORS.
// Source: data/*.json (do not hand-edit; regenerate from those files).
window.BB_DATA = {
  "status": {
    "backup": {
      "enabled": false,
      "status": "disabled"
    },
    "freshness_contracts": [
      {
        "notification_severity": "none",
        "owner": "dispatcher",
        "safe_next_action": "bb runs list --state pending --json",
        "subject": "run.pending",
        "threshold_seconds": null
      },
      {
        "notification_severity": "warning",
        "owner": "operator",
        "safe_next_action": "bb recover --json before resolving or killing",
        "subject": "run.running",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "critical",
        "owner": "operator",
        "safe_next_action": "bb runs show <id> --json, inspect side effects, then bb runs resolve",
        "subject": "run.awaiting_recovery",
        "threshold_seconds": 3600
      },
      {
        "notification_severity": "warning",
        "owner": "operator",
        "safe_next_action": "bb runs release <id> or bb runs retire <id> --reason TEXT",
        "subject": "run.blocked_budget",
        "threshold_seconds": null
      },
      {
        "notification_severity": "warning",
        "owner": "dispatcher",
        "safe_next_action": "bb runs show <id> --json; bb recover --json if stale",
        "subject": "attempt.acquired",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "warning",
        "owner": "dispatcher",
        "safe_next_action": "bb runs show <id> --json; bb recover --json if stale",
        "subject": "attempt.prepared",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "critical",
        "owner": "operator",
        "safe_next_action": "bb recover --json; never auto-replay without side-effect inspection",
        "subject": "attempt.executing",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "warning",
        "owner": "dispatcher",
        "safe_next_action": "bb runs show <id> --json; bb recover --json if stale",
        "subject": "attempt.collecting",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "warning",
        "owner": "dispatcher",
        "safe_next_action": "bb runs show <id> --json; bb recover --json if stale",
        "subject": "attempt.finalizing",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "warning",
        "owner": "dispatcher",
        "safe_next_action": "bb runs show <id> --json; bb recover --json if stale",
        "subject": "attempt.released",
        "threshold_seconds": 1800
      },
      {
        "notification_severity": "warning",
        "owner": "operator",
        "safe_next_action": "bb notify retry --json or bb notify ack <id> --reason TEXT --json",
        "subject": "notification.pending_or_failed",
        "threshold_seconds": null
      }
    ],
    "generated_at": "2026-07-03T20:10:19.440853738Z",
    "guards": {
      "attention_debt": {
        "awaiting_recovery": 0,
        "blocking": true,
        "notification_failed": 0,
        "notification_pending": 0,
        "open_dlq": 1,
        "parked_tasks": 1,
        "reason": "open_dlq=1 parked_tasks=1",
        "stale_runs": 0
      },
      "cron": {
        "max_catchup_fires": 1,
        "skipped_catchup_fires": 0
      },
      "gate": {
        "arbiter": "arbiter",
        "arm_timeout_seconds": 3600,
        "max_rounds": 3,
        "quorum": 5,
        "required": [
          "verify",
          "correctness",
          "security",
          "simplification",
          "product"
        ]
      },
      "guard_event_counts": [
        {
          "kind": "attention_debt_brake",
          "total": 20
        }
      ],
      "in_flight": {
        "cost_usd": 0.0,
        "enforcement_mode": "streaming harness usage updates attempt cost while running; max_cost_per_run_usd breaches follow agent.policy.side_effect_policy (default kill)",
        "policy": "reserved = sum(max_cost_per_run_usd) over running runs; observed in-flight cost is metered from streaming harness usage and can kill/quarantine/log on max_cost_per_run_usd breach; the global daily ceiling (max_cost_per_day_usd) is still enforced by budget::pre_dispatch_check on every dispatch.",
        "reserved_usd": 0.0,
        "runs": 0,
        "spent_today_usd": 0.0
      },
      "ingress": {
        "max_body_bytes": 1048576,
        "oversized_rejections": 0
      },
      "notify": {
        "failed": 0,
        "outbox": {
          "acknowledged": 0,
          "delivered": 0,
          "failed": 0,
          "pending": 0
        },
        "recent_outbox": []
      },
      "paused_at": null,
      "paused_reason": null,
      "plane_paused": false,
      "recent_guard_events": [
        {
          "at": "2026-07-03T17:28:48.59948399Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 20,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-03T16:26:45.794862998Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 19,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-03T14:00:00.045794436Z",
          "count": 1,
          "detail": "source=cron open_dlq=1 parked_tasks=1",
          "id": 18,
          "kind": "attention_debt_brake",
          "task": "model-catalog-watch"
        },
        {
          "at": "2026-07-03T09:58:27.65991829Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 17,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-03T09:58:27.607437995Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 16,
          "kind": "attention_debt_brake",
          "task": "canary-triage"
        },
        {
          "at": "2026-07-03T09:57:57.547454591Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 15,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-03T09:57:57.496415096Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 14,
          "kind": "attention_debt_brake",
          "task": "canary-triage"
        },
        {
          "at": "2026-07-03T09:57:52.431702136Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 13,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-03T09:57:52.381656672Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 12,
          "kind": "attention_debt_brake",
          "task": "canary-triage"
        },
        {
          "at": "2026-07-03T09:57:47.325152151Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 11,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-03T09:57:47.272176385Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 10,
          "kind": "attention_debt_brake",
          "task": "canary-triage"
        },
        {
          "at": "2026-07-02T21:52:34.587205673Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 9,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-02T21:40:21.466954407Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 8,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-02T20:42:36.700926529Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 7,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-02T20:33:39.31298284Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 6,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-02T20:23:42.936060284Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 5,
          "kind": "attention_debt_brake",
          "task": "review"
        },
        {
          "at": "2026-07-02T20:12:26.965904618Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 4,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-02T20:11:56.905857305Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 3,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-02T20:11:51.815784643Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 2,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        },
        {
          "at": "2026-07-02T20:11:46.696347352Z",
          "count": 1,
          "detail": "source=webhook open_dlq=1 parked_tasks=1",
          "id": 1,
          "kind": "attention_debt_brake",
          "task": "incident-triage"
        }
      ]
    },
    "ingress": {
      "recent": [
        {
          "dedupe_key": null,
          "duplicate": false,
          "id": 149,
          "payload_hash": "e254bb2856de6adb3fcb0c8ff2592551d0cb14bb6090ca54c7550c447c6a7249",
          "received_at": "2026-07-03T17:34:40.725988545Z",
          "run_id": "831e11c3654e",
          "source_event_id": null,
          "task": "dispatch-demo",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "manual-drill:INC-ay76lctwao3z:after-89082a1",
          "duplicate": false,
          "id": 148,
          "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
          "received_at": "2026-07-02T20:35:27.593091525Z",
          "run_id": "3f2e52af59e5",
          "source_event_id": null,
          "task": "incident-triage",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "manual-drill:INC-ay76lctwao3z:after-08862ca",
          "duplicate": false,
          "id": 147,
          "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
          "received_at": "2026-07-02T20:27:03.79042321Z",
          "run_id": "ab5d92dbe4f7",
          "source_event_id": null,
          "task": "incident-triage",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "manual-drill:INC-ay76lctwao3z:DLV-zo93xxqrjhsd",
          "duplicate": false,
          "id": 146,
          "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
          "received_at": "2026-07-02T20:15:47.807137741Z",
          "run_id": "1141ed106fea",
          "source_event_id": null,
          "task": "incident-triage",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/942|29b9887d675ca6e52baecfcdaa3f9fe0f9f5b981",
          "duplicate": false,
          "id": 145,
          "payload_hash": "64c711a02327887539ff7fe3e3284808a27aeb87a60dfb187b0fe641ddc6be29",
          "received_at": "2026-07-02T19:42:25.479165513Z",
          "run_id": "b7c8dbe5b444",
          "source_event_id": "260f0040-764e-11f1-8223-fbaf3c8987f9",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/171|7b80ee9373d67ca3427974440d25e396eff1d35e",
          "duplicate": false,
          "id": 144,
          "payload_hash": "09a896a2701408c53d7f8bcece525db9f03f0cca6ac9d59d9151cc7ae6c3ca84",
          "received_at": "2026-07-02T19:39:20.004846437Z",
          "run_id": "6cd91be583d3",
          "source_event_id": "b78126d0-764d-11f1-84f7-8cb912650f39",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/941|1b19364954f5b260a4329a15579f919b4cda6f89",
          "duplicate": false,
          "id": 143,
          "payload_hash": "3ac37cd6b8dc23fad6180db76b73e4f5077baec4ef34e2c404f6b36592f70cca",
          "received_at": "2026-07-02T19:38:13.751999126Z",
          "run_id": "22ecfaaa51fb",
          "source_event_id": "90096450-764d-11f1-80e1-061b4e076951",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/171|669f51cd04e76463989f0dc45676025d20bed3ab",
          "duplicate": false,
          "id": 142,
          "payload_hash": "fc56268256fbe4963428c8bfc5c68f9d0a405c967ab8ff0a6933e351e68689c0",
          "received_at": "2026-07-02T19:37:30.296729887Z",
          "run_id": "e2a1e8c6344d",
          "source_event_id": "762a5080-764d-11f1-9180-8b894ee5b4cc",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/190|179ace024f80b2f0edf9090c158c833fcbcc660a",
          "duplicate": false,
          "id": 141,
          "payload_hash": "eee980168014ea259f7b8daea3cbb7bb55e40ac7adff00b2408198603fa8418e",
          "received_at": "2026-07-02T19:36:27.275408649Z",
          "run_id": "1f9952f858e3",
          "source_event_id": "509b2420-764d-11f1-88f5-56940a863d7c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/940|e9f8f6e194caa095b1d362b90d87b612fb67ab50",
          "duplicate": false,
          "id": 140,
          "payload_hash": "fde22d5556888b770a0b57c32dccd7c7833e0062c1eb1f3427b3ea4c3bd3867e",
          "received_at": "2026-07-02T19:32:45.318472447Z",
          "run_id": "d35af67fd726",
          "source_event_id": "cc408ad0-764c-11f1-9d1a-316e5a62000e",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/35|3e2886073fd2e13ac039a3a99b72dc5a4c4a39ea",
          "duplicate": false,
          "id": 139,
          "payload_hash": "86bfa6a38ca89ed1287d75605a70f46d507fe4fb56726aba999f4fea826d3b87",
          "received_at": "2026-07-02T19:03:59.519403435Z",
          "run_id": "0a2fce61171b",
          "source_event_id": "c7ab9630-7648-11f1-981b-f2ff76054af8",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/34|5fa7567e6813f5929f6512408541678e2b1bec66",
          "duplicate": false,
          "id": 138,
          "payload_hash": "04e0becfb004e58826aa617af3218b337066519b0275bbfedc7286b5ba881647",
          "received_at": "2026-07-02T18:57:58.710795265Z",
          "run_id": "a77a70573ac2",
          "source_event_id": "f09aabe0-7647-11f1-91f4-017a01505841",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/34|d799138d76f630109d6f1bfc30d1eff00ab0c09d",
          "duplicate": false,
          "id": 137,
          "payload_hash": "b1641242b9fb96ec6051233905cae547f97af7fb859ff55fd89c34b1e3042a40",
          "received_at": "2026-07-02T18:41:04.760868693Z",
          "run_id": "3e8f5463cc94",
          "source_event_id": "9444e4c0-7645-11f1-9e03-6892221e866d",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/939|d0c50fbd28f1a8ccaab9cfa2a32a59447f647a56",
          "duplicate": false,
          "id": 136,
          "payload_hash": "7e42ce5d8357e7e4afd5672970ab177d8b69424b57951fd37208d81e290a3e4b",
          "received_at": "2026-07-02T18:32:49.750424879Z",
          "run_id": "358a56e0c4ac",
          "source_event_id": "6d2e7190-7644-11f1-8519-703357b09fc7",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/33|a56d7abdd398a1c51db62c9112ded211982b855d",
          "duplicate": false,
          "id": 135,
          "payload_hash": "1a60607edbc53d185306119bfc659b9b8cd8af2a5992bfb20886594c7619cbf6",
          "received_at": "2026-07-02T18:16:39.174581201Z",
          "run_id": "48b934b545ed",
          "source_event_id": "2ab37880-7642-11f1-8b94-59350993c937",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/33|992aff3d6b44686dac66c5f99a1edbafa13d365b",
          "duplicate": false,
          "id": 134,
          "payload_hash": "417f2efc1442b817ccfd850d642b1b0ecf28cf8547b28022545c1d1a7b6bd417",
          "received_at": "2026-07-02T18:13:04.673743914Z",
          "run_id": "1b18f92bfe9a",
          "source_event_id": "aaa72ab0-7641-11f1-8032-ba928da890a3",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/weave/pull/20|d4436c79c7b0e6389c5a34cecea799c672c105bf",
          "duplicate": false,
          "id": 133,
          "payload_hash": "badf6c53d4be9c29a46e80a2e282750cfa22780c23eb10541f14ed53d4e2d37b",
          "received_at": "2026-07-02T17:59:17.351285066Z",
          "run_id": "eff45ee21f95",
          "source_event_id": "bdc493f0-763f-11f1-8964-02eec59e9692",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/938|7d552c59d3469e13fecdb87b8e244475304135ff",
          "duplicate": false,
          "id": 132,
          "payload_hash": "66cf9122bcb1a6dfd27dd595d164a8fa1dee261242279cf410e577fb95448679",
          "received_at": "2026-07-02T17:33:26.205128251Z",
          "run_id": "987e960ec0f5",
          "source_event_id": "210f6b50-763c-11f1-97d5-212a09c4c883",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/31|2c87fa76564bdf5ab8d88c4ce9162e178b3aacac",
          "duplicate": false,
          "id": 131,
          "payload_hash": "d5069aef8a31e90d0a39be3383585de0e4b53c54ae9bd575cdb93360039e5985",
          "received_at": "2026-07-02T17:31:01.676671007Z",
          "run_id": "343a2c1bf51b",
          "source_event_id": "6a9d5580-763b-11f1-952f-454f358bbc7d",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": null,
          "duplicate": false,
          "id": 130,
          "payload_hash": "2a819cde6cb1b83739462d8ce2cfa85cb6441e95fff3e9c31f4d3be88182c19f",
          "received_at": "2026-07-02T17:25:29.505888899Z",
          "run_id": "12eb41792f97",
          "source_event_id": null,
          "task": "review",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/937|6e37aa41aa28f4f3dbaab62ec2bba7055996ae09",
          "duplicate": false,
          "id": 129,
          "payload_hash": "6fada402a924ec51ca4f7fc07864a074c40228e4b66353bcc9c919b1dd5f726a",
          "received_at": "2026-07-02T17:24:06.918703975Z",
          "run_id": "7e771eed0896",
          "source_event_id": "d3d33110-763a-11f1-89ef-ce0abbba2ce3",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/31|4161046c2c31f23fbdf16ebe0171d9789470edee",
          "duplicate": false,
          "id": 128,
          "payload_hash": "c4d238f47249795d4137353bc98ec60488f1800aeb3a8ecdca770040ab643a36",
          "received_at": "2026-07-02T17:13:05.520666046Z",
          "run_id": "b9e0734d7343",
          "source_event_id": "499326f0-7639-11f1-9ac1-9fac2ce53f89",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/936|6bd80262f6f047a5135805d3ec21c307d58422f3",
          "duplicate": false,
          "id": 127,
          "payload_hash": "520e63eefacd13b58e9b6a52ded26ad704e6c282d278438a43b4c17e6bdfa073",
          "received_at": "2026-07-02T17:04:13.655054442Z",
          "run_id": "a218d8f1d928",
          "source_event_id": "0c93e060-7638-11f1-8d59-521541892295",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "cron:2026-07-02T14:00:00+00:00",
          "duplicate": false,
          "id": 126,
          "payload_hash": null,
          "received_at": "2026-07-02T14:00:00.92315996Z",
          "run_id": "0c803861cc73",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/935|44844f4f63b7b7fa711b4a29ab696786f389dbef",
          "duplicate": false,
          "id": 125,
          "payload_hash": "c59bc299e815a08f42422ab23857757f41493a42b677e7ed07554e99fcc836f3",
          "received_at": "2026-07-02T09:37:44.107378069Z",
          "run_id": "2e230792e695",
          "source_event_id": "ac9d1c00-75f9-11f1-82b6-161c23ecbd30",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/934|1eec414c0afd00bc6144a8abceb53c30b7f0e2d4",
          "duplicate": false,
          "id": 124,
          "payload_hash": "93d64a60550749319880bda2e7e0d19ea6725c3c7fceff753e49a90b054d3072",
          "received_at": "2026-07-02T09:26:31.687748284Z",
          "run_id": "252f5e9c7b21",
          "source_event_id": "1b1c3a50-75f8-11f1-86e8-d587cd9743ce",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/933|56352021f5a4998ac74f8b89c60a3e5c59749534",
          "duplicate": false,
          "id": 123,
          "payload_hash": "45bb0992a80367b28ebb48d4285fae99f0582de2f8db441bbae8c9f23a799725",
          "received_at": "2026-07-02T09:00:56.408178329Z",
          "run_id": "d5c899a19ce3",
          "source_event_id": "88d5a990-75f4-11f1-970b-59d8cd9474a0",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/932|a446a5fe51ea755cce5ba648d1d24dc3150dcd95",
          "duplicate": false,
          "id": 122,
          "payload_hash": "58a55cf69f2e9668117c47a9c63ee12b5906a760d5d3f8a2b55b58e74763e8b1",
          "received_at": "2026-07-02T08:22:59.775047084Z",
          "run_id": "fb4a5e37c5aa",
          "source_event_id": "3bc58530-75ef-11f1-8bce-5bb95b6e92f2",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/931|a7851a85e64204635e0ef0963f2f72fbad3d8b95",
          "duplicate": false,
          "id": 121,
          "payload_hash": "43146fd6f1480e7fad4e1b6712af51f79230fc689acbaff151f52e01f073895f",
          "received_at": "2026-07-02T08:15:58.464814673Z",
          "run_id": "e9126dc52ef7",
          "source_event_id": "40c19250-75ee-11f1-8b8a-42c657e4ece9",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/930|587045158254de6693f5e0b753462adc6148268f",
          "duplicate": false,
          "id": 120,
          "payload_hash": "a93f1198ae563a0c984a99cc6ef8665168fc19bc2da2e77fa70f668f8ec23dc3",
          "received_at": "2026-07-02T08:04:09.446822383Z",
          "run_id": "1f4d6d1c5f17",
          "source_event_id": "9a1009b0-75ec-11f1-9fbc-6778bb9ba1da",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/929|344880fbe6f3b6ee452dd84642b65e773e5689f7",
          "duplicate": false,
          "id": 119,
          "payload_hash": "406a87384e90a08de4c8e608233f4a77a44f25eaf87531a9cfebe0556e211961",
          "received_at": "2026-07-02T07:57:29.486077584Z",
          "run_id": "f2b59076b4a9",
          "source_event_id": "aba72ba0-75eb-11f1-8804-15abe1dccefc",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/928|051e85b88e6a1513c703ec94ed821350a334217d",
          "duplicate": false,
          "id": 118,
          "payload_hash": "0a99b0ef5d6e596f6b714c34644c80a9fcd5a380490455a19895ffed894f57bb",
          "received_at": "2026-07-02T07:44:02.834354963Z",
          "run_id": "0a42ed01208b",
          "source_event_id": "cae82fc0-75e9-11f1-9e9a-28ce4f0999f8",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/927|041fef9f380b0ad3a0a4dc6826c7db7931999716",
          "duplicate": false,
          "id": 117,
          "payload_hash": "344afd906b94ba88ae9422718224aeb3fd2e414df5174b9fd6d8902ebee27546",
          "received_at": "2026-07-02T07:36:10.173902573Z",
          "run_id": "a3ffcb8e37e0",
          "source_event_id": "ada3c650-75e8-11f1-856c-4872581499c7",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/926|3efde42ba50313b63724a1b60bd5590d0de4e29c",
          "duplicate": false,
          "id": 116,
          "payload_hash": "17d1478fc79d6c58a138f544db86e0a40789416788bc639429aa247cdcbf982d",
          "received_at": "2026-07-02T07:28:39.304040322Z",
          "run_id": "c8f5cd440828",
          "source_event_id": "a475ba30-75e7-11f1-966e-2bdb878f5357",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/925|e45b839cde3d2b116e67f2d4c205bac3843181f5",
          "duplicate": false,
          "id": 115,
          "payload_hash": "c152c0d17bdb0c415eac06a1014f7ff36e0d97e46c3665362758960adb65ab66",
          "received_at": "2026-07-02T07:04:57.04289324Z",
          "run_id": "9451667fdd60",
          "source_event_id": "54b3f870-75e4-11f1-9abb-8eb36536b562",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/924|186341b101771455b0dccb82a95a4a4f9e2bbcef",
          "duplicate": false,
          "id": 114,
          "payload_hash": "e80bc7418f9eb37961914113f337fbe0043e435fbf9cdd74cfeb19ee3b519869",
          "received_at": "2026-07-02T06:54:20.171193125Z",
          "run_id": "9402e80ac51d",
          "source_event_id": "d922a0e0-75e2-11f1-8050-d8e712059a97",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/923|c6c9268df42b025afd216ec85b8d6107b3bda508",
          "duplicate": false,
          "id": 113,
          "payload_hash": "d8dae5883e676e0df0da5dfb76b194f56be462b45877e72c6258916a14ddd696",
          "received_at": "2026-07-02T06:35:53.497143696Z",
          "run_id": "c813d43a33ce",
          "source_event_id": "4577a8b0-75e0-11f1-98e3-b87d20dff969",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/922|ec9a438e1a78c16807cca7a0c0661e94ae6126a4",
          "duplicate": false,
          "id": 112,
          "payload_hash": "1faf5d6d0e3fded1176a845092e1a878744beac21a536b9fffcb7e3cf3b609e3",
          "received_at": "2026-07-02T06:30:45.117348158Z",
          "run_id": "5959d446e1e8",
          "source_event_id": "8db32240-75df-11f1-9098-d05ee8055064",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/921|cb632c5254e11564f94d768ca108fbdc5e5a5e27",
          "duplicate": false,
          "id": 111,
          "payload_hash": "33d44a7d7ff864c624960efac6f3bf7ee793e862712b13fbf24b3c3ac21a49f3",
          "received_at": "2026-07-02T06:15:40.548544338Z",
          "run_id": "82d9c1e37404",
          "source_event_id": "72936300-75dd-11f1-8d85-4f59255b62d6",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/920|c6f7bcd04fd3c722d2cc0e90d16460813f7b2bca",
          "duplicate": false,
          "id": 110,
          "payload_hash": "199dd6c0cb870a37714696f1b52fd3b2f9337e46faa56f92fffeaaada47c9404",
          "received_at": "2026-07-02T06:04:49.869342448Z",
          "run_id": "dbc6c8f14b18",
          "source_event_id": "eeb409a0-75db-11f1-93e4-89f66d9f25c2",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/919|c6592466ba3e4fb0066545c3b3cb3078c6c8a42e",
          "duplicate": false,
          "id": 109,
          "payload_hash": "338e14ffc41672607bcdeecc923d7ce5d1dd6f3c5c5a8f7aa415bf39bf9b76fa",
          "received_at": "2026-07-02T05:59:47.950027783Z",
          "run_id": "54b42938cd02",
          "source_event_id": "3aac6420-75db-11f1-95ac-7ada801e782d",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/918|7da2c470b2bbb24b053af69108e78048f528cdc8",
          "duplicate": false,
          "id": 108,
          "payload_hash": "ddc14598282afdf85159e5c67a13e8e6ce67c3a1807a0c0958d12fe701e5f7d1",
          "received_at": "2026-07-02T05:52:55.442945707Z",
          "run_id": "49111d3a2618",
          "source_event_id": "44e1e010-75da-11f1-83f5-8a8fd9694a2a",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/917|6b8e3c6f214af13cdc89f35fb187d838b18bb4eb",
          "duplicate": false,
          "id": 107,
          "payload_hash": "ee11a4b244e2c653ea0c43871002dafea169652d4cb5ba162b4a13aa6cb8e4f2",
          "received_at": "2026-07-02T05:50:13.875893562Z",
          "run_id": "076dced2e2bd",
          "source_event_id": "e4896a80-75d9-11f1-8ae8-b3c57d87bd5f",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/916|df05510348e721e49ef844c9e8fcb4d79bbb5ab0",
          "duplicate": false,
          "id": 106,
          "payload_hash": "31cf86ec9254fbe7240bd112c4c6001582b1eee188c1a44edd22e7b4286e8ced",
          "received_at": "2026-07-02T05:44:12.84345609Z",
          "run_id": "2de44089c5ba",
          "source_event_id": "0d69da30-75d9-11f1-9be8-8f5d85e88a4c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:canary-triage:DLV-zj5royvznl01",
          "duplicate": false,
          "id": 105,
          "payload_hash": "cde2fbdb20d5803690f4d9f24f5d306686863e6d969218286daa0221e737444a",
          "received_at": "2026-07-02T05:31:51.303375135Z",
          "run_id": "2e3013116b4c",
          "source_event_id": "DLV-zj5royvznl01",
          "task": "canary-triage",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/915|0d2dc0130004e22cdc80a7db0aa9c702e6d24944",
          "duplicate": false,
          "id": 104,
          "payload_hash": "b6614e9a846c08ed8bf96d2fd9a45e4bbeb5378dbc20b1e770bf29c03d21cdc5",
          "received_at": "2026-07-02T05:31:30.076285861Z",
          "run_id": "8a991eba91eb",
          "source_event_id": "46bc9ae0-75d7-11f1-83c7-084ec5df33ca",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/914|9d5618270f5daecf4917609e01f99b2ecba0bc63",
          "duplicate": false,
          "id": 103,
          "payload_hash": "4814102eaa8820f47a9a0c61f47cfd083b3efebdeccc76c613d7f73822be1072",
          "received_at": "2026-07-02T05:25:47.368394096Z",
          "run_id": "bb02acf5ddca",
          "source_event_id": "7a714210-75d6-11f1-8551-cfef5ca87329",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/913|d0bb0a84e568f09a7328262ff3b27bc4ace31347",
          "duplicate": false,
          "id": 102,
          "payload_hash": "b112d3b6633ee160b877054ef23a19665baf34af496698c7ab0c46eee0850880",
          "received_at": "2026-07-02T05:22:10.762554803Z",
          "run_id": "dd5c98cdcbdf",
          "source_event_id": "f951fda0-75d5-11f1-8469-63421ee56f6b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/913|55dde26eb1cac8adc949830ea9eec25b37083fcb",
          "duplicate": false,
          "id": 101,
          "payload_hash": "f6bc73baf7c99cde9caa8b6ad49b9293ce6881e0dc686657e9b2fb866436ab6d",
          "received_at": "2026-07-02T05:20:23.538514234Z",
          "run_id": "3168c75c2b6f",
          "source_event_id": "b98808e0-75d5-11f1-9e2f-4b17e376005b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/912|4f44c27fabc0fce64b968d58e8e13d7bce47f698",
          "duplicate": false,
          "id": 100,
          "payload_hash": "d9e0c84870c0698d32cfb9a028992e67df285fd9bf074e39447ed021e4bed81c",
          "received_at": "2026-07-02T05:16:27.014581161Z",
          "run_id": "e389b7fdaba1",
          "source_event_id": "2c703130-75d5-11f1-95f3-5911824b310c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/911|b8f3adc48d28149ebe8a4468ebea3a9d2e11a1b0",
          "duplicate": false,
          "id": 99,
          "payload_hash": "b407eb226c7693444b6101e542a72876091bb6ee912a6e4f5a5eace874f73a24",
          "received_at": "2026-07-02T05:10:54.00489935Z",
          "run_id": "7959c873df52",
          "source_event_id": "66020140-75d4-11f1-83f2-5f39ad57326c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/910|fee9bf7509da994e4a7302685f781d2f3462c1e8",
          "duplicate": false,
          "id": 98,
          "payload_hash": "d6a87516cfeca5b345baca606796e466e178182b4299639ccd38d108e62fd258",
          "received_at": "2026-07-02T05:05:39.933106196Z",
          "run_id": "2b86cff44106",
          "source_event_id": "aac9b0d0-75d3-11f1-991b-bfe0aacf8a73",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/909|6d4fb3df2cb631373c3c29ddc0b6b98cf24b282c",
          "duplicate": false,
          "id": 97,
          "payload_hash": "5f451e555ae824acf50110a40c02b4346e81fe513c4f7ece0906384f064629b8",
          "received_at": "2026-07-02T05:02:04.304213223Z",
          "run_id": "2176a639b82f",
          "source_event_id": "2a2df440-75d3-11f1-830b-9e18bba9fefc",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/908|b63ecd3bd5b6613558aefe47766bd9a778d4e395",
          "duplicate": false,
          "id": 96,
          "payload_hash": "164710b1cfd13be20099deb3e6eb475010358faa3af7f1a56f69f2a0205dae3b",
          "received_at": "2026-07-02T04:59:22.308121035Z",
          "run_id": "dbfd22c2fad1",
          "source_event_id": "c9b01c60-75d2-11f1-9626-9af8bb891891",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/907|5c4790cdd2318211381cd891a5dc7d3116f020b8",
          "duplicate": false,
          "id": 95,
          "payload_hash": "d520e0475a74161a297758866246da6cae90384c681bca00cfda79b4ed69dac4",
          "received_at": "2026-07-02T04:54:22.135263493Z",
          "run_id": "3fcec38233b1",
          "source_event_id": "16bbe2b0-75d2-11f1-9f41-9d037623e009",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/906|5cc651369fa38c0ef28793ee5603f3e0d67f97c0",
          "duplicate": false,
          "id": 94,
          "payload_hash": "b01c4c5c3fe7b3ef5418cdb70daa3a41b45edf8bec19ababa226489cb430810e",
          "received_at": "2026-07-02T04:49:46.060691433Z",
          "run_id": "7e0d1ffd9bba",
          "source_event_id": "722ad3a0-75d1-11f1-8c3e-e3366d4c7007",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/905|242df13e89214c842ba5989594b1db92e9292a21",
          "duplicate": false,
          "id": 93,
          "payload_hash": "8c6d11706508cdaf1ec1968c653e3413f8a1fdbc496f1ca0f9aceebeca0c2d7e",
          "received_at": "2026-07-02T04:43:29.515898423Z",
          "run_id": "435b75b6809a",
          "source_event_id": "91bd36a0-75d0-11f1-9953-0070d3617ead",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/904|662ae14a6c6e655258cf42889c92446664ebe208",
          "duplicate": false,
          "id": 92,
          "payload_hash": "c9faa169543ce11f727461a15f101dcf165fc01eeea1908da09d24b657d08dae",
          "received_at": "2026-07-02T04:40:37.647584938Z",
          "run_id": "e8706f3ca7d6",
          "source_event_id": "2b6199a0-75d0-11f1-988d-50d7d28fd4e7",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/903|a421063a28c2f2095b5f2e5ec6ecfb7224d1ba3b",
          "duplicate": false,
          "id": 91,
          "payload_hash": "a71f0c5f272560b2a6597dce0c50bbafe34f394aa150caec9b00d85f68fd4435",
          "received_at": "2026-07-02T04:37:24.529669595Z",
          "run_id": "86810e638b14",
          "source_event_id": "b828aeb0-75cf-11f1-90a1-f0fa5587200a",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/902|5bcd029eaf7053a277d0020b30899b709d6c0b7d",
          "duplicate": false,
          "id": 90,
          "payload_hash": "c7b5b6f800377e908f8e15c102e18892d057fcedf4a361bc00a483ff3243779e",
          "received_at": "2026-07-02T04:32:04.592237016Z",
          "run_id": "f29b46789e16",
          "source_event_id": "f9959a80-75ce-11f1-953c-ef8ed7dde324",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/901|086c17b5da87f6788bdbc07913f53b27c0b63cc8",
          "duplicate": false,
          "id": 89,
          "payload_hash": "fce45de9cea64bb67889b58b9292a6e3d3256a02ce7c09805ae1263c7d5dc759",
          "received_at": "2026-07-02T04:27:53.243365852Z",
          "run_id": "1074185d1b52",
          "source_event_id": "63c42df0-75ce-11f1-9cbd-c3526ecdfd21",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/900|447756ce72cdd4f767c0b1988fceab8e1ada7339",
          "duplicate": false,
          "id": 88,
          "payload_hash": "bf708108263e91baa1a4a202f2ea8a654137b4f575ce76854ca769df0a12bd50",
          "received_at": "2026-07-02T04:23:34.790506104Z",
          "run_id": "0d38b6b50b8a",
          "source_event_id": "c9b19540-75cd-11f1-8446-9068a26286c9",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/899|a66f722c6eaee322876295a2a6b807e0fddfbb10",
          "duplicate": false,
          "id": 87,
          "payload_hash": "bd3304d99d7462eb71ab71ece77e7304160b8d8d0c3363901da1e473c70b8f1f",
          "received_at": "2026-07-02T04:20:23.741100514Z",
          "run_id": "5c3effce348b",
          "source_event_id": "57d35850-75cd-11f1-9052-c07993cc0f4b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/898|acfbe7945f1b9e6242f35a42b793fe70e0145e3c",
          "duplicate": false,
          "id": 86,
          "payload_hash": "30471fe20a83cf0acd73219358211ee6952e8b36b54ed0be8c5d2874c4a743c7",
          "received_at": "2026-07-02T04:17:03.714795087Z",
          "run_id": "e2349dea99a9",
          "source_event_id": "e0a23120-75cc-11f1-8e92-aed04c8ffa28",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/897|8337d398cc573162f8c69e4cc34a923bb2cb40fa",
          "duplicate": false,
          "id": 85,
          "payload_hash": "223d89232bf39b0d4507939ada424b7b611e862bd2e34d0886bbae238311cc7d",
          "received_at": "2026-07-02T04:14:13.81574172Z",
          "run_id": "81194f49e269",
          "source_event_id": "7b52fb10-75cc-11f1-969c-311de30225e5",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/896|1184847c4ceb1fbacc16d934da411ab37ae1d63b",
          "duplicate": false,
          "id": 84,
          "payload_hash": "ef0ffd7b354805073fa2a9e2a53b979f84902603d4e482d8c65b86f2a0d76444",
          "received_at": "2026-07-02T04:10:33.145314878Z",
          "run_id": "3a50d65719ac",
          "source_event_id": "f7b8cb90-75cb-11f1-868a-c5db4798b7d5",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/895|3d088a938fcd925de7cf43051086529fbd06c183",
          "duplicate": false,
          "id": 83,
          "payload_hash": "a2b3e6d8c1e2d81ad2807ecbb3684b836edf1846ca3866b2a3d66d727cdf9b2b",
          "received_at": "2026-07-02T04:06:41.171020545Z",
          "run_id": "bcb657aba91e",
          "source_event_id": "6d791a20-75cb-11f1-9af4-5ee4c452378c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/894|984d25d171f4bdbc0bfc300c86671897f895c026",
          "duplicate": false,
          "id": 82,
          "payload_hash": "42e34136b4269fcecc6589e33a6f8eb6f6ad91c9fbe7aea18220c2a016ed4891",
          "received_at": "2026-07-02T04:00:27.823808249Z",
          "run_id": "f4d84be828ec",
          "source_event_id": "8edfc7f0-75ca-11f1-8a99-5d73aae45e05",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/893|51a68dd2c94934cfee73c96b90439a2351cad011",
          "duplicate": false,
          "id": 81,
          "payload_hash": "9bfa97e4ee87daac9ab7740df9e10b0ba33f9f3649357e9abb66005b9acfe1a2",
          "received_at": "2026-07-02T03:58:03.894560338Z",
          "run_id": "a53bbfb0adaf",
          "source_event_id": "39249b60-75ca-11f1-8dfe-57b3d7cc47fa",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/892|efd65b6b514ed429da7763adab2c904cf5eb70cf",
          "duplicate": false,
          "id": 80,
          "payload_hash": "0e3b26793fe976aa8df5c95ec06748b6327b30dbe1282ff91ecb1352219e3e9e",
          "received_at": "2026-07-02T03:53:11.351521265Z",
          "run_id": "5af5dbbfc776",
          "source_event_id": "8ad22460-75c9-11f1-816d-0e8194051786",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/891|9fd8e7348a51154d98ee9907b6eb3aa70b7b7f79",
          "duplicate": false,
          "id": 79,
          "payload_hash": "2341586af85cc5a94031ae8a6f90d9d99ba53e5a1cefdbf8ba563840529052b6",
          "received_at": "2026-07-02T03:50:10.79792381Z",
          "run_id": "94478400993d",
          "source_event_id": "1f3f1460-75c9-11f1-8f75-d0ba1ada0a89",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/890|730a7184183c87fc2dbbe5f6f79901779d225c2a",
          "duplicate": false,
          "id": 78,
          "payload_hash": "33b55e7547f682b4a28658cd8e82e1a0c8dc0d20e86c83fa977ef5d79e945079",
          "received_at": "2026-07-02T03:44:17.174812515Z",
          "run_id": "881c9a4a4462",
          "source_event_id": "4c6ee100-75c8-11f1-9332-36a5cc6dfc1c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/889|a1e75fa85a13262881ca296dbf0ae1188eb3aa72",
          "duplicate": false,
          "id": 77,
          "payload_hash": "b6ff96876f1392ee72268c766f9bf6f901304330cbb69e620ad36f3007a8045c",
          "received_at": "2026-07-02T03:41:32.298936324Z",
          "run_id": "1b7f62c3d247",
          "source_event_id": "ea128520-75c7-11f1-8707-7f7e6854f352",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/887|c90b3dbe35b7cc2e5652c02dfb38ec6d4d8388ef",
          "duplicate": false,
          "id": 76,
          "payload_hash": "6cf3754c4f61d9d2d608b1b10e68c251f859c480348f7d151dac67f93d14fa44",
          "received_at": "2026-07-02T03:02:30.118820664Z",
          "run_id": "a6e2fd520eef",
          "source_event_id": "76137d50-75c2-11f1-92cc-10efa4a0db1b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/886|8216ae034d1b228b294a477218a2a13c4920600c",
          "duplicate": false,
          "id": 75,
          "payload_hash": "6e75b447a5e23701542bb6a62a1242e8ccba861fcaea41ead6fca012a90b05ec",
          "received_at": "2026-07-02T02:23:03.381708058Z",
          "run_id": "cfaddad07faa",
          "source_event_id": "f3369ca0-75bc-11f1-8e4e-e14f492848db",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/885|39be1f453825e6e54bb54476494b5b6c62fbbc5b",
          "duplicate": false,
          "id": 74,
          "payload_hash": "deb2577a02c13e1dab72e15dd00139b37accb0c45ee0d508e21248dfb6bea35e",
          "received_at": "2026-07-02T01:44:23.682841713Z",
          "run_id": "8b0dd049d131",
          "source_event_id": "8cc287e0-75b7-11f1-95ea-600da08e62c8",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/884|1bd5dd29cbde99cbc99f7835b147ea1be2e31a74",
          "duplicate": false,
          "id": 73,
          "payload_hash": "849c75668d7fad244e18c4491197bc81bf243f056a9d5c3f7e6aeb238e3ba8b2",
          "received_at": "2026-07-02T00:26:50.825318926Z",
          "run_id": "e40e2fbb6c03",
          "source_event_id": "b7699250-75ac-11f1-8dbe-246e66b7d784",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/883|1e57f03d34d2eee9bc053d00a6c7678cce187747",
          "duplicate": false,
          "id": 72,
          "payload_hash": "40dd0450ee9f870617453fef34f53a6225d5ee9a8d0560902e317ec700b7b754",
          "received_at": "2026-07-02T00:14:54.177619901Z",
          "run_id": "a47d6a0903a3",
          "source_event_id": "0abbe720-75ab-11f1-9e4e-070e28e5c692",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/882|06068ab27a21dcf8845ac8ec8ef3fe3269fac020",
          "duplicate": false,
          "id": 71,
          "payload_hash": "cf6261231f9c66aacfee81cbfd0ca8cb321d0cec9fd9280ee7e094ef1a150113",
          "received_at": "2026-07-01T23:22:04.782599111Z",
          "run_id": "e9d5072fd99c",
          "source_event_id": "ab2b80b0-75a3-11f1-9b02-1ae1cdd05a50",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/881|24ad5caab06079f2427834b17b17f8796f246bf0",
          "duplicate": false,
          "id": 70,
          "payload_hash": "f97c5f6ff65f409d22ab5eb54c74ed8b685385eb99dc65b2117fde0f2d511972",
          "received_at": "2026-07-01T23:13:51.482818221Z",
          "run_id": "ee32c1b39d8d",
          "source_event_id": "83d01540-75a2-11f1-8ce6-05ac0f903a49",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/880|7591d97567adb5a9538f8fc5cffb4bbbdd5a9a56",
          "duplicate": false,
          "id": 69,
          "payload_hash": "a971159822b3f4b5e3e2958a840d6ede8601b7e21f77b2db3abc736a6438d966",
          "received_at": "2026-07-01T23:07:23.242955276Z",
          "run_id": "08eb3dbe6fa9",
          "source_event_id": "9db5d540-75a1-11f1-8eb1-090c74a00c12",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/879|44bec240d391592369189e1738fff832664b8c82",
          "duplicate": false,
          "id": 68,
          "payload_hash": "b7f38217f0fd6864d80e219a00d301510709d1b7749c3f27725015442f2e510d",
          "received_at": "2026-07-01T22:53:39.756615271Z",
          "run_id": "b8d342f6aaf9",
          "source_event_id": "b2f88f80-759f-11f1-9243-41b3f43873c2",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/878|fdcca380a29d4bd6d6989d51c1d11078887e9b1d",
          "duplicate": false,
          "id": 67,
          "payload_hash": "c0f91dadaa7d5224c81dfe8fdfded0d6e2ca5a4072184ef9d2197e4b1ad993ef",
          "received_at": "2026-07-01T22:44:43.169072499Z",
          "run_id": "e12d5f49106d",
          "source_event_id": "71a35c50-759e-11f1-8297-45b0830d07da",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/877|f9b6e8a07857a06780e9f910b06d5e23d2f39c83",
          "duplicate": false,
          "id": 66,
          "payload_hash": "f4686693c81fc0ff30ebe22af89343e624cfadfc1ca2338408c9dff5c2f37480",
          "received_at": "2026-07-01T22:39:14.259128317Z",
          "run_id": "20f92868b1b3",
          "source_event_id": "af09db10-759d-11f1-98b6-71e7181c5909",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/876|23635e34fd6e01c1f2f7352e0161e0e3bd9f54cc",
          "duplicate": false,
          "id": 65,
          "payload_hash": "212faa2efea77a91b8e73d845e3e294aa9d2e87dee685443d41de894488fa34b",
          "received_at": "2026-07-01T22:31:03.123173094Z",
          "run_id": "7e5aba81a30e",
          "source_event_id": "8a3f53b0-759c-11f1-82a9-165103c7c492",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/875|469fc01fc0b01fa578692fb5eb0a7bf905eef98d",
          "duplicate": false,
          "id": 64,
          "payload_hash": "5a836517b4a638fb23902c4dd13e30b43bdbc156fcf740c6bddc51c39047a01b",
          "received_at": "2026-07-01T22:22:05.981641815Z",
          "run_id": "5803526a4244",
          "source_event_id": "4a242720-759b-11f1-8bb4-171fea9e59f4",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/874|8f1d1bf02bba2bddca5b32bbca6952552248ffd2",
          "duplicate": false,
          "id": 63,
          "payload_hash": "c54479cddea4fbff7874ba29da1224af4985afd65b14ce5e8f39910bac5e5211",
          "received_at": "2026-07-01T20:53:41.77954904Z",
          "run_id": "5699a6651800",
          "source_event_id": "f0986100-758e-11f1-9ca2-b88d9cf97e94",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/873|927e0c7a9537ff772e94266e5012b1982d0b6a6e",
          "duplicate": false,
          "id": 62,
          "payload_hash": "4a118542b6d425ddace8a827a26a42e3622ec56637f13cc947f44f060a7ad83d",
          "received_at": "2026-07-01T19:17:31.094102111Z",
          "run_id": "9f8bc543c356",
          "source_event_id": "8108e1f0-7581-11f1-9287-61b0fc4b0853",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/871|b333453614034e013e0fd86862adcfc9b7c82729",
          "duplicate": false,
          "id": 61,
          "payload_hash": "4e5edec6658f80aecb8e0c3a797128bb48c919c4b913c23d31e8c9dbade2d17e",
          "received_at": "2026-07-01T17:56:58.427977744Z",
          "run_id": "90b91211d517",
          "source_event_id": "406ea5e0-7576-11f1-8421-de61115f8e35",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/872|2afafa05782b6e68d5478be8c7f1b53a25e350f1",
          "duplicate": false,
          "id": 60,
          "payload_hash": "49e12fc13f3167bf30d0a03bbb256e4a41a2614749e0ae212c05de99c18c0083",
          "received_at": "2026-07-01T17:55:35.803228655Z",
          "run_id": "218ecc8c3c23",
          "source_event_id": "0f224190-7576-11f1-916f-712bc7224e73",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/871|b6131b80b3afe31b750c5cb9c00535f10d7fdcd3",
          "duplicate": false,
          "id": 59,
          "payload_hash": "c93b70e52cbe0648c3da188409de2dadd8ab37c180edb7550f1b901ce7c23fc8",
          "received_at": "2026-07-01T17:41:55.878294201Z",
          "run_id": "23ba478a2079",
          "source_event_id": "267df0c0-7574-11f1-871b-f76477106392",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:canary-triage:DLV-c1id5ljacfzn",
          "duplicate": false,
          "id": 58,
          "payload_hash": "ae391cdb1fc4988dd5f953d99ce95dee30a3a737c447b966e6a88c48183a0548",
          "received_at": "2026-07-01T17:40:14.317729448Z",
          "run_id": "356ed977067c",
          "source_event_id": "DLV-c1id5ljacfzn",
          "task": "canary-triage",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/870|d0e737729e76b9b921d02f757718753d9e52b49b",
          "duplicate": false,
          "id": 57,
          "payload_hash": "81e2e626266b85489db80f62357d1071dcab7d211d3f8c7b3eaf2a6df8833b40",
          "received_at": "2026-07-01T17:06:03.499022978Z",
          "run_id": "8c6ddb5678f3",
          "source_event_id": "23967a80-756f-11f1-96c9-e8c2c637c38c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/869|110342fb01b74ce061f65ffa1a2111e3ce43558e",
          "duplicate": false,
          "id": 56,
          "payload_hash": "3706de6c540e4909973036a85439ec2443d5525248b2655b178ba60077b2cac2",
          "received_at": "2026-07-01T16:40:54.065266929Z",
          "run_id": "06473df79768",
          "source_event_id": "9fddcde0-756b-11f1-8f24-ae41f6796de7",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/869|d2ee706ef58c4a55582bf25fb20397d6c34f0f31",
          "duplicate": false,
          "id": 55,
          "payload_hash": "979cecfac20af072d9401aee58a3f4a3b913b9a926f83bb0f1a03c614df8010b",
          "received_at": "2026-07-01T16:35:35.99888897Z",
          "run_id": "9361f9067f86",
          "source_event_id": "e259dfc0-756a-11f1-8297-b4fcdf49c55d",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "cron:2026-07-01T14:00:00+00:00",
          "duplicate": false,
          "id": 54,
          "payload_hash": null,
          "received_at": "2026-07-01T14:00:00.981473694Z",
          "run_id": "b2ecc0925f03",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "cron:2026-06-30T14:00:00+00:00",
          "duplicate": false,
          "id": 53,
          "payload_hash": null,
          "received_at": "2026-06-30T14:00:08.86922315Z",
          "run_id": "aa494640246c",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "cron:2026-06-29T14:00:00+00:00",
          "duplicate": false,
          "id": 52,
          "payload_hash": null,
          "received_at": "2026-06-29T14:00:00.541287329Z",
          "run_id": "9844b05e7c23",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/868|f7b957604cb58aad654f4f9fe254eae92a3830bd",
          "duplicate": false,
          "id": 51,
          "payload_hash": "bf92c80d32fb2cb35b08d6023301efaf260f37ff9780b3541f1fc6bc197eaf52",
          "received_at": "2026-06-28T16:23:20.506598551Z",
          "run_id": "9ad35ccd13ad",
          "source_event_id": "aca72400-730d-11f1-8b38-d7189e8ceafd",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "cron:2026-06-28T15:00:00+00:00",
          "duplicate": false,
          "id": 50,
          "payload_hash": null,
          "received_at": "2026-06-28T15:00:05.412130215Z",
          "run_id": "3fb36e86c7d5",
          "source_event_id": null,
          "task": "gardener",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "cron:2026-06-28T14:00:00+00:00",
          "duplicate": false,
          "id": 49,
          "payload_hash": null,
          "received_at": "2026-06-28T14:00:04.32690913Z",
          "run_id": "58ee5e763484",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "cron:2026-06-27T14:00:00+00:00",
          "duplicate": false,
          "id": 48,
          "payload_hash": null,
          "received_at": "2026-06-27T14:00:08.233537074Z",
          "run_id": "ae304fb042fc",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "cron:2026-06-26T14:00:00+00:00",
          "duplicate": false,
          "id": 47,
          "payload_hash": null,
          "received_at": "2026-06-26T14:00:02.087950445Z",
          "run_id": "bfabaa5bd624",
          "source_event_id": null,
          "task": "model-catalog-watch",
          "trigger_kind": "cron"
        },
        {
          "dedupe_key": "cerberus-canary-20260625-bun-openrouter-1",
          "duplicate": false,
          "id": 46,
          "payload_hash": "373482af15cbf2b652bb0a8742a8d0a9b635fc7e9f9f025a70855e5a92a68c08",
          "received_at": "2026-06-25T17:55:43.797692302Z",
          "run_id": "0d12f1818992",
          "source_event_id": null,
          "task": "review",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "cerberus-canary-20260625-omp-fallback-1",
          "duplicate": false,
          "id": 45,
          "payload_hash": "373482af15cbf2b652bb0a8742a8d0a9b635fc7e9f9f025a70855e5a92a68c08",
          "received_at": "2026-06-25T17:50:12.243229084Z",
          "run_id": "2b5bb2c768cc",
          "source_event_id": null,
          "task": "review",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "cerberus-canary-20260625-wrapper-rebuild-1",
          "duplicate": false,
          "id": 44,
          "payload_hash": "373482af15cbf2b652bb0a8742a8d0a9b635fc7e9f9f025a70855e5a92a68c08",
          "received_at": "2026-06-25T17:43:57.443568441Z",
          "run_id": "9a9c12096eb9",
          "source_event_id": null,
          "task": "review",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "cerberus-canary-20260625-change-type-1",
          "duplicate": false,
          "id": 43,
          "payload_hash": "373482af15cbf2b652bb0a8742a8d0a9b635fc7e9f9f025a70855e5a92a68c08",
          "received_at": "2026-06-25T17:37:52.445891577Z",
          "run_id": "7f8fceefdcc1",
          "source_event_id": null,
          "task": "review",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": null,
          "duplicate": false,
          "id": 42,
          "payload_hash": "373482af15cbf2b652bb0a8742a8d0a9b635fc7e9f9f025a70855e5a92a68c08",
          "received_at": "2026-06-25T17:27:16.076758086Z",
          "run_id": "17591327a7a9",
          "source_event_id": null,
          "task": "review",
          "trigger_kind": "manual"
        },
        {
          "dedupe_key": "wh:review:f9c80e08f13a58368b098d02bb0ec72f272ec152",
          "duplicate": false,
          "id": 41,
          "payload_hash": "a7c23e9568748b2aff972409fca715c129c699722843a3f7684c32577a9fd668",
          "received_at": "2026-06-25T16:22:13.940527061Z",
          "run_id": "bd8dda640578",
          "source_event_id": "05ba96c0-70b2-11f1-96f4-e8b8e280ef3d",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:04add88b0f14354bb33b4778f3020bd7969607af",
          "duplicate": false,
          "id": 40,
          "payload_hash": "52b87e22d2ee31a96a53b3274e28cf79440db78725a672fb414b6e2bb7ba4d9f",
          "received_at": "2026-06-21T00:19:34.206438967Z",
          "run_id": "e2610fbbe1fd",
          "source_event_id": "e0a0ec30-6d06-11f1-8b44-7a1618570b1f",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:69ca0ef215906511a2bca2647333f53d11be6c12",
          "duplicate": false,
          "id": 39,
          "payload_hash": "dd333d612e9013547c49aef7a00ab9d4687b85878eb68738f5ec4d3b7fc0d53a",
          "received_at": "2026-06-20T23:39:41.675031586Z",
          "run_id": "4075fb12e4f1",
          "source_event_id": "4e85c320-6d01-11f1-941b-05311d419991",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:e59432ab1b0a5357e1e86eb3a1d762354e9f1ddd",
          "duplicate": false,
          "id": 38,
          "payload_hash": "1f344d9c1c0bb815a58e9192ccbca625b97f67a79cf6059cd8d923b41373aa25",
          "received_at": "2026-06-20T23:11:33.618590547Z",
          "run_id": "6d682c4f1ab2",
          "source_event_id": "6048de70-6cfd-11f1-9278-02050128e457",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:9a299f475576d5cec1b5c28da3daf704fd25c611",
          "duplicate": false,
          "id": 37,
          "payload_hash": "2b26f95139b5da3f72817f8701961b3f3196fcadfd4dc9878546ffd5522dec9a",
          "received_at": "2026-06-17T20:52:36.925836341Z",
          "run_id": "5776c30ccbed",
          "source_event_id": "7814b7c0-6a8e-11f1-9126-00c0ca0124fe",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:751deaef8d9d55ed9e65c8b50e36ff930578382b",
          "duplicate": false,
          "id": 36,
          "payload_hash": "de25d2ce2db48322afd23da3694169c406ea92817c39d27479e4218c1080bda6",
          "received_at": "2026-06-17T18:18:48.398762989Z",
          "run_id": "1bb01669398f",
          "source_event_id": "fb5a3990-6a78-11f1-9845-a33477f0daa5",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:7a88ced7b8558dbd839198cb67004a221031058c",
          "duplicate": false,
          "id": 35,
          "payload_hash": "23151ca224af16317627518dd1f92a08e5345c0d2b96ca4f752bf1e587409128",
          "received_at": "2026-06-17T13:42:24.998719209Z",
          "run_id": "fb379e102ece",
          "source_event_id": "5ea46290-6a52-11f1-9ab5-dc3ef8352408",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:04a996d53a1a927ef4b62db8b829fcd68c5d683d",
          "duplicate": false,
          "id": 34,
          "payload_hash": "8600042de291c1c7bf6c7f9a95ec3792b699cfef04f39b8cba491409e54c2f8b",
          "received_at": "2026-06-17T04:42:27.392853135Z",
          "run_id": "fb16690dd35b",
          "source_event_id": "f07e67c0-6a06-11f1-9e13-cd8c3ecef06d",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:ee87bd70a1a13df3a0e3f1641df7e21cfbc85d1b",
          "duplicate": false,
          "id": 33,
          "payload_hash": "8a64427381756b0c6caf153dc14456bfbd0c0e2129d99da26e0880a9d66615ca",
          "received_at": "2026-06-16T22:17:57.016204966Z",
          "run_id": "4f107f3d5902",
          "source_event_id": "39867330-69d1-11f1-8d54-fa92b6f6bb67",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:36c399d062ddb23ae835e7597c37a0319bad0aa4",
          "duplicate": false,
          "id": 32,
          "payload_hash": "30cae5c04aa577aa0c55cf9e1aafbd25981b2caccdcdbe396faeb6ebc8e5cc8b",
          "received_at": "2026-06-16T22:17:16.221475475Z",
          "run_id": "834415072521",
          "source_event_id": "211b88d0-69d1-11f1-9ee5-a876e918c07c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:cc578c02f9c23490f97415bf75cd1b2a9f3b04ae",
          "duplicate": false,
          "id": 31,
          "payload_hash": "f657cb56eaa608f6c9de6993757e74d46aff1c8b78e225a7f470f9961620f2a6",
          "received_at": "2026-06-16T21:36:04.352038834Z",
          "run_id": "f82942338f6b",
          "source_event_id": "5fb74580-69cb-11f1-8d44-81f696f6517b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:631a3be75f666af1c3c58a51dfa1caca74c98d3f",
          "duplicate": false,
          "id": 30,
          "payload_hash": "6d3846b57759e20c07146c343118bd55e45a9173cc412094a4c113043b1561cf",
          "received_at": "2026-06-16T21:20:06.377345913Z",
          "run_id": "0d8ddf688bbd",
          "source_event_id": "24e81ad0-69c9-11f1-8c4a-79928f29f8ae",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:1d3afd84d19e05d4c9805a90ee50671dd2e0aa1a",
          "duplicate": false,
          "id": 29,
          "payload_hash": "927a0613a1e744492c1d8af1ded739ed0e7e8b983df0e4f291923e322ce0504f",
          "received_at": "2026-06-16T21:08:19.383462208Z",
          "run_id": "d7292f4acb7c",
          "source_event_id": "7f612f80-69c7-11f1-9296-f6d638f74c3b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:e8d21e0a9f6455976271ba875765ca235c7bb3d0",
          "duplicate": false,
          "id": 28,
          "payload_hash": "8446a902b91d2f70c91b38f75ead14a100a1a134b34b7055dd45763dc24a9378",
          "received_at": "2026-06-16T21:00:38.597622183Z",
          "run_id": "5dcfe85288d2",
          "source_event_id": "6cca89d0-69c6-11f1-8906-c5f2d0d960df",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:f29866f90155258230ae3f59d1c728ef33f4963e",
          "duplicate": false,
          "id": 27,
          "payload_hash": "40bc350fbc09e4a409daec41650f15dbd174919662e2de7e01f3f9acb71787e9",
          "received_at": "2026-06-16T20:49:13.364054362Z",
          "run_id": "b13457a78779",
          "source_event_id": "d4211380-69c4-11f1-8451-e24655ebef25",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:473cbbb17a8d0f8d7cd612c6b4c541df95db6e5c",
          "duplicate": false,
          "id": 26,
          "payload_hash": "74064cbb44784200f7c36735f8e962d7b920c8be70d433c80b92a4a6302f5712",
          "received_at": "2026-06-16T19:33:48.686506882Z",
          "run_id": "93737fc73f87",
          "source_event_id": "4b6a5790-69ba-11f1-88b2-85aedbe3a4ae",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:0ecf165b784a6122919a780b009ace3f3c57ecfe",
          "duplicate": false,
          "id": 25,
          "payload_hash": "c3f56e2b6c7abafdd47c31420c06478b87965e6fbe30dcce72ceae26362d0e57",
          "received_at": "2026-06-16T19:22:22.52147847Z",
          "run_id": "ef7a883ec9d8",
          "source_event_id": "b265f000-69b8-11f1-86e8-69fd0584ada4",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:5d906cabbcc9e53570be92cf2a5f16f8e90a15c9",
          "duplicate": false,
          "id": 24,
          "payload_hash": "79e844ea44418f602429d6612831b15dc737aa42f2beb9b243feac9d777c97aa",
          "received_at": "2026-06-16T18:37:39.920941691Z",
          "run_id": "70a01b5c5032",
          "source_event_id": "736351a0-69b2-11f1-9801-3e9472cc06a1",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:0468f6c18221e788aed2d3487a90b51703a6c328",
          "duplicate": false,
          "id": 23,
          "payload_hash": "5c0aac380ebf84a2013d9e40ff180cba1a745dcb8ead7682fb59f9920631d5b9",
          "received_at": "2026-06-16T18:24:24.91936795Z",
          "run_id": "2f4cd84dff96",
          "source_event_id": "99838550-69b0-11f1-8378-5ee9bb65c830",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:d4f184d0079a3f22dabbc2014a7518b55ea0167d",
          "duplicate": false,
          "id": 22,
          "payload_hash": "b26921e73f8632afbe9b1ee6cd5d6e46f8da3124e4c59b25d1e3a4002f45a5b5",
          "received_at": "2026-06-16T15:39:25.853803713Z",
          "run_id": "42d8838bba20",
          "source_event_id": "8d16a3e0-6999-11f1-97bc-f7e3d4b4ee06",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:f64661f67358ed4cb3fd98061617a937d3eacbdd",
          "duplicate": false,
          "id": 21,
          "payload_hash": "ad46dbbb0438804a3358934a3f8002c5cf5dccff821178ab46ee867790cc7d25",
          "received_at": "2026-06-16T00:36:51.366884025Z",
          "run_id": "5f267b602007",
          "source_event_id": "76d54d80-691b-11f1-8729-df76fb4890b7",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:72ef05e2e63e4c80717111b9655bebf3fb0be24a",
          "duplicate": false,
          "id": 20,
          "payload_hash": "f3ca43981eec8af05d4ba6c3ceae3cefee4c0395c1eeee6f9bcd4b505414a467",
          "received_at": "2026-06-16T00:21:16.82858169Z",
          "run_id": "1f662bb2dc94",
          "source_event_id": "49918b10-6919-11f1-949d-4439b05772ad",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:72ff8a7cab57c513d114baf4513986e34fbbe3f3",
          "duplicate": false,
          "id": 19,
          "payload_hash": "35d6b0181301ec73e2c6c41f15b342d5fab42cf728d21c8c355ad45fec219003",
          "received_at": "2026-06-16T00:05:52.940881932Z",
          "run_id": "c42f6989bcce",
          "source_event_id": "23132ea0-6917-11f1-845b-0c36736de5d2",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:803061e8d44e81f07a87224066cbc280013ca749",
          "duplicate": false,
          "id": 18,
          "payload_hash": "7dc56761c5fc98ebc7f4e3e6d04868b06698f6a679108d5c0a19daf9f8029853",
          "received_at": "2026-06-16T00:02:09.071153026Z",
          "run_id": "b934d5698686",
          "source_event_id": "9d8b11d0-6916-11f1-86d4-48baeb74f4db",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:062d0572776849bc966503b12ab1e05067a0480d",
          "duplicate": false,
          "id": 17,
          "payload_hash": "3be65c32a64f909ed8d650bb78a484dafa0e0a8adec298fcb5b8c40898cab67f",
          "received_at": "2026-06-15T22:01:55.791930732Z",
          "run_id": "287927ccc03b",
          "source_event_id": "d222b940-6905-11f1-9039-1d8b2237539e",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:926529f12be6d7ee5034eba3f3d381bc7e64541c",
          "duplicate": false,
          "id": 16,
          "payload_hash": "5a6a5246b235c71ad6d4e120f027c00846055385d686c3195ca32ae9d19b9a9e",
          "received_at": "2026-06-15T21:55:17.612811021Z",
          "run_id": "7d20a25b5f36",
          "source_event_id": "e4dc1410-6904-11f1-9717-dfc47619e3c1",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:b617b3fb7f4e8a9af29dcb0cbf2129f3a8bbad3c",
          "duplicate": false,
          "id": 15,
          "payload_hash": "737e4f3650b06c67dda14f87da5b8e26473b7fa0e3d2964dfcb6092f1a231f36",
          "received_at": "2026-06-15T21:47:33.803838173Z",
          "run_id": "7af49577152f",
          "source_event_id": "d06b5730-6903-11f1-8b83-92191ea6039b",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:87e007fdee5a4b89f7c353a536ec10407cd2e010",
          "duplicate": false,
          "id": 14,
          "payload_hash": "92d8d205fa15136b55b817aff7c1983f3cd50d5d8581ae4f3ca8094242db45f0",
          "received_at": "2026-06-15T21:24:58.870440234Z",
          "run_id": "516385b5f22c",
          "source_event_id": "a8a729c0-6900-11f1-8f35-84e366ae6e82",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:65d7aa472b1f8a6413c3aff8aaac2ea9cb41ee4f",
          "duplicate": false,
          "id": 13,
          "payload_hash": "38fc05aa86c61fbab0c4f52986147b1d5dbb65fc02252ff6d9aea31607d1fe8c",
          "received_at": "2026-06-13T22:26:22.656292231Z",
          "run_id": "ea152ebbc389",
          "source_event_id": "e78f0df0-6776-11f1-9adb-6ecc0a3021b4",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:4506fdfaea4821a64ced73fd35b63a6e21ff9c2d",
          "duplicate": false,
          "id": 12,
          "payload_hash": "86e2060a534764783804b808188bcb0478c393236d4358c908dd6591dd418d91",
          "received_at": "2026-06-13T16:11:28.844688383Z",
          "run_id": "bdc925507b27",
          "source_event_id": "884d8440-6742-11f1-8e9e-64d6f7b58c77",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:3756a4d4b43652f0b3ba61cdc2d3b643ad9c328c",
          "duplicate": false,
          "id": 11,
          "payload_hash": "267a2a7880ab1dc9c1651f372a79afac19289c288fe2f9527ff80d2f6a923f89",
          "received_at": "2026-06-13T15:49:58.542266442Z",
          "run_id": "f1bc495fd652",
          "source_event_id": "873f7700-673f-11f1-9ce6-0349b8727164",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:373e48bb462f3526262773dd858dcbdbb3a310bf",
          "duplicate": false,
          "id": 10,
          "payload_hash": "f756e6fb9f62a6cfaf6102f2f8dbbff991ab4a4c16a34a5c2f309062248c0188",
          "received_at": "2026-06-13T15:48:43.523574033Z",
          "run_id": "26709b34a927",
          "source_event_id": "5a7ce090-673f-11f1-9b3a-3718abacb95c",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:1ed3988997f613f1c86f8ab996383dba41339147",
          "duplicate": false,
          "id": 9,
          "payload_hash": "e46301ffe9d09a08a82cf60c5b617bda24ca1e81139947bc74a6c25bdb0af64e",
          "received_at": "2026-06-13T15:18:27.590018412Z",
          "run_id": "85af0f04311e",
          "source_event_id": "20321300-673b-11f1-8ed7-82d04bc8d461",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:ecfa81f78eecc84130253d321071689ba8de25da",
          "duplicate": false,
          "id": 8,
          "payload_hash": "8c69a20a9070736fd6c600b05032139ef001dcdf3f21ad90731012cb98d74d32",
          "received_at": "2026-06-13T15:10:46.542062609Z",
          "run_id": "76f3f7a100cc",
          "source_event_id": "0d717720-673a-11f1-9eb5-e65ac8278afd",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:3b7e30aacdb59ccfabaa698d67d01e72dd25ff0e",
          "duplicate": false,
          "id": 7,
          "payload_hash": "e80b4e7a6d13b4e11789594ae899ddf2b9c571c4e54c84343c13bcd023b4a6b3",
          "received_at": "2026-06-13T15:10:00.581923268Z",
          "run_id": "f69275df26b2",
          "source_event_id": "f1f68da0-6739-11f1-8984-589329943dd9",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:17b3be3a766a3d315451858cb25115e03a021118",
          "duplicate": false,
          "id": 6,
          "payload_hash": "59cf743b9438f48f8aa3a9ffd381f55628c21b49e3f219ef37f92dc2ac1f729c",
          "received_at": "2026-06-13T15:05:47.972110575Z",
          "run_id": "ad64376e9596",
          "source_event_id": "5b48f1e0-6739-11f1-98b9-80acbf78453e",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:3947de0fc5714d915933e3cbd35e5b777689b5ea",
          "duplicate": false,
          "id": 5,
          "payload_hash": "655ab20df8fbe5e32b63119cff72b531e0e1caf3490a2c5aa6cb784c9c128cfa",
          "received_at": "2026-06-12T21:10:44.98238575Z",
          "run_id": "e8e8cfd3f365",
          "source_event_id": "2c9b8c90-66a3-11f1-85d9-d9ff88b4f463",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:c1505cdb21d6340f25b2c9e38047a50a251f5cd8",
          "duplicate": false,
          "id": 4,
          "payload_hash": "484064c17f7b04a8379c175a570f33257428ec6d36be22730f8336dede66565f",
          "received_at": "2026-06-12T21:00:16.624709579Z",
          "run_id": "4636a4164125",
          "source_event_id": "b602f150-66a1-11f1-8025-0dc7dfb463b8",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:404253ce14b3b4b36a0932e15e17556989ba6d9f",
          "duplicate": false,
          "id": 3,
          "payload_hash": "b6306ce7296311c7f4da045e8483c6a69d86415d9b5270c177f85a5790de400c",
          "received_at": "2026-06-12T20:30:02.205578498Z",
          "run_id": "6308d6ba2116",
          "source_event_id": "7c9e8c20-669d-11f1-92c2-59ae0691239a",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:e6b1807e82b4087f027a3f4761b0e23c9a416613",
          "duplicate": false,
          "id": 2,
          "payload_hash": "b8497b54d85641367a96889f43dc6916814799330909d7bc2934f519ce5ebf6c",
          "received_at": "2026-06-12T18:45:26.23057007Z",
          "run_id": "c2375fce5e9e",
          "source_event_id": "dfd82670-668e-11f1-9746-42fadec4d223",
          "task": "review",
          "trigger_kind": "webhook"
        },
        {
          "dedupe_key": "wh:review:a5aafd93a8c07f3498302d65100c0f197d4294dd",
          "duplicate": false,
          "id": 1,
          "payload_hash": "218eb4eceaaecd56382296d1962b7487f234804c44c435f0a58472caae6b52ef",
          "received_at": "2026-06-12T18:30:07.64992008Z",
          "run_id": "6c24c4fac8b8",
          "source_event_id": "bc282c40-668c-11f1-879b-dcbbcf96307b",
          "task": "review",
          "trigger_kind": "webhook"
        }
      ]
    },
    "leases": [],
    "ledger": {
      "schema_version": 1,
      "supported_schema_version": 1
    },
    "summary": {
      "active_leases": 0,
      "cost_today_usd": 0.0,
      "max_cost_per_day_usd": 25.0,
      "open_dlq": 1,
      "parked_tasks": 1,
      "plane_paused": false,
      "recent_ingress_events": 149,
      "tasks": 32
    },
    "tasks": [
      {
        "agent": "storm-arbiter@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2-thinking",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "arbiter",
        "verdict": "arbiter"
      },
      {
        "agent": "bb-builder-rust@v2",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 4.0,
          "max_runs_per_day": 5,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 90,
          "tool_action_cap": null,
          "turn_cap": 80
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "omp",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "build",
        "verdict": null
      },
      {
        "agent": "bb-builder-rust-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 1.5,
          "max_runs_per_day": 5,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 90,
          "tool_action_cap": null,
          "turn_cap": 80
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "build-glm",
        "verdict": null
      },
      {
        "agent": "bb-builder-rust-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 1.5,
          "max_runs_per_day": 5,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 90,
          "tool_action_cap": null,
          "turn_cap": 80
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "build-kimi",
        "verdict": null
      },
      {
        "agent": "canary-triager@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.75,
          "max_runs_per_day": 8,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": 40
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": {
            "created_at": "2026-07-02T05:32:02.936189258Z",
            "error": "prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701",
            "id": 1,
            "run_id": "2e3013116b4c",
            "status": "open"
          },
          "open": 1,
          "total": 1
        },
        "harness": "pi",
        "ingress": {
          "events": 2,
          "latest": {
            "dedupe_key": "wh:canary-triage:DLV-zj5royvznl01",
            "duplicate": false,
            "id": 105,
            "payload_hash": "cde2fbdb20d5803690f4d9f24f5d306686863e6d969218286daa0221e737444a",
            "received_at": "2026-07-02T05:31:51.303375135Z",
            "run_id": "2e3013116b4c",
            "source_event_id": "DLV-zj5royvznl01",
            "task": "canary-triage",
            "trigger_kind": "webhook"
          }
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-flash",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {
            "failure": 1,
            "success": 1
          },
          "cost_usd": 0.0175550618,
          "duration_ms": 127999,
          "latest": {
            "agent": "canary-triager@v1",
            "cost_usd": null,
            "created_at": "2026-07-02T05:31:51.303375135Z",
            "duration_ms": 11230,
            "id": "2e3013116b4c",
            "reason": "dead_letter:1 prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701",
            "state": "failure",
            "updated_at": "2026-07-02T05:32:02.936484338Z"
          },
          "latest_failure": {
            "agent": "canary-triager@v1",
            "cost_usd": null,
            "created_at": "2026-07-02T05:31:51.303375135Z",
            "duration_ms": 11230,
            "id": "2e3013116b4c",
            "reason": "dead_letter:1 prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701",
            "state": "failure",
            "updated_at": "2026-07-02T05:32:02.936484338Z"
          },
          "recent": 2
        },
        "safe_next_actions": [
          {
            "command": "bb dlq replay 1",
            "kind": "replay_pre_execute_dlq",
            "reason": "prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701"
          },
          {
            "command": "bb runs show 2e3013116b4c --json",
            "kind": "inspect_artifact",
            "reason": "dead_letter:1 prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701"
          }
        ],
        "substrate": "sprites",
        "task": "canary-triage",
        "verdict": null
      },
      {
        "agent": "ci-diagnoser@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": 30
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-flash",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "ci-diagnose",
        "verdict": null
      },
      {
        "agent": "ci-diagnoser-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.75,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": 30
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "ci-diagnose-glm",
        "verdict": null
      },
      {
        "agent": "ci-diagnoser-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.75,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": 30
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "ci-diagnose-kimi",
        "verdict": null
      },
      {
        "agent": "storm-correctness@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.6,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 45,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-pro",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "correctness",
        "verdict": "correctness"
      },
      {
        "agent": "storm-correctness-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.6,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 45,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "correctness-glm",
        "verdict": "correctness-glm"
      },
      {
        "agent": "storm-correctness-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.6,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 45,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "correctness-kimi",
        "verdict": "correctness-kimi"
      },
      {
        "agent": "fix-prompter@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.2,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 15,
          "tool_action_cap": null,
          "turn_cap": 25
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-flash",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "fix-prompt",
        "verdict": null
      },
      {
        "agent": "gardener@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 2,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 1,
          "latest": {
            "dedupe_key": "cron:2026-06-28T15:00:00+00:00",
            "duplicate": false,
            "id": 50,
            "payload_hash": null,
            "received_at": "2026-06-28T15:00:05.412130215Z",
            "run_id": "3fb36e86c7d5",
            "source_event_id": null,
            "task": "gardener",
            "trigger_kind": "cron"
          }
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-flash",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {
            "success": 1
          },
          "cost_usd": 0.041485347799999996,
          "duration_ms": 268743,
          "latest": {
            "agent": "gardener@v1",
            "cost_usd": 0.041485347799999996,
            "created_at": "2026-06-28T15:00:05.412130215Z",
            "duration_ms": 268743,
            "id": "3fb36e86c7d5",
            "reason": null,
            "state": "success",
            "updated_at": "2026-06-28T15:04:34.537226734Z"
          },
          "latest_failure": null,
          "recent": 1
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "gardener",
        "verdict": null
      },
      {
        "agent": "gardener-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.5,
          "max_runs_per_day": 2,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "gardener-glm",
        "verdict": null
      },
      {
        "agent": "gardener-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.5,
          "max_runs_per_day": 2,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "gardener-kimi",
        "verdict": null
      },
      {
        "agent": "incident-triager@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "quarantine",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 5.0,
          "max_runs_per_day": 3,
          "output_bytes_cap": 120000,
          "runs_today": 0,
          "timeout_minutes": 120,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "command",
        "ingress": {
          "events": 3,
          "latest": {
            "dedupe_key": "manual-drill:INC-ay76lctwao3z:after-89082a1",
            "duplicate": false,
            "id": 148,
            "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
            "received_at": "2026-07-02T20:35:27.593091525Z",
            "run_id": "3f2e52af59e5",
            "source_event_id": null,
            "task": "incident-triage",
            "trigger_kind": "manual"
          }
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {
            "failure": 2,
            "success": 1
          },
          "cost_usd": -0.0,
          "duration_ms": 489765,
          "latest": {
            "agent": "incident-triager@v1",
            "cost_usd": null,
            "created_at": "2026-07-02T20:35:27.593091525Z",
            "duration_ms": 251738,
            "id": "3f2e52af59e5",
            "reason": null,
            "state": "success",
            "updated_at": "2026-07-02T20:39:39.330556555Z"
          },
          "latest_failure": {
            "agent": "incident-triager@v1",
            "cost_usd": null,
            "created_at": "2026-07-02T20:27:03.79042321Z",
            "duration_ms": 232604,
            "id": "ab5d92dbe4f7",
            "reason": "harness exit 15: setsid: child 1549 did not exit normally: Success",
            "state": "failure",
            "updated_at": "2026-07-02T20:30:56.394306644Z"
          },
          "recent": 3
        },
        "safe_next_actions": [
          {
            "command": "bb runs show ab5d92dbe4f7 --json",
            "kind": "inspect_artifact",
            "reason": "harness exit 15: setsid: child 1549 did not exit normally: Success"
          }
        ],
        "substrate": "sprites",
        "task": "incident-triage",
        "verdict": null
      },
      {
        "agent": "model-catalog-watcher@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": null,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 20,
          "tool_action_cap": null,
          "turn_cap": 30
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 7,
          "latest": {
            "dedupe_key": "cron:2026-07-02T14:00:00+00:00",
            "duplicate": false,
            "id": 126,
            "payload_hash": null,
            "received_at": "2026-07-02T14:00:00.92315996Z",
            "run_id": "0c803861cc73",
            "source_event_id": null,
            "task": "model-catalog-watch",
            "trigger_kind": "cron"
          }
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-flash",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {
            "success": 7
          },
          "cost_usd": 0.23961379300000002,
          "duration_ms": 1399253,
          "latest": {
            "agent": "model-catalog-watcher@v1",
            "cost_usd": 0.0456839796,
            "created_at": "2026-07-02T14:00:00.92315996Z",
            "duration_ms": 229891,
            "id": "0c803861cc73",
            "reason": null,
            "state": "success",
            "updated_at": "2026-07-02T14:03:51.309866016Z"
          },
          "latest_failure": null,
          "recent": 7
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "model-catalog-watch",
        "verdict": null
      },
      {
        "agent": "model-evaluator@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 2.0,
          "max_runs_per_day": 10,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": 40
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "openai/gpt-5.5",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "model-eval",
        "verdict": null
      },
      {
        "agent": "storm-product@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "x-ai/grok-4.3",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "product",
        "verdict": "product"
      },
      {
        "agent": "storm-product-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.35,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "product-glm",
        "verdict": "product-glm"
      },
      {
        "agent": "storm-product-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.35,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "product-kimi",
        "verdict": "product-kimi"
      },
      {
        "agent": "cerberus-reviewer@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 1.25,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "command",
        "ingress": {
          "events": 135,
          "latest": {
            "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/942|29b9887d675ca6e52baecfcdaa3f9fe0f9f5b981",
            "duplicate": false,
            "id": 145,
            "payload_hash": "64c711a02327887539ff7fe3e3284808a27aeb87a60dfb187b0fe641ddc6be29",
            "received_at": "2026-07-02T19:42:25.479165513Z",
            "run_id": "b7c8dbe5b444",
            "source_event_id": "260f0040-764e-11f1-8223-fbaf3c8987f9",
            "task": "review",
            "trigger_kind": "webhook"
          }
        },
        "lease": null,
        "model": "cerberus-review-pr",
        "parked": "32 runs today >= max_runs_per_day 20",
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 30,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {
            "blocked_budget": 30,
            "failure": 46,
            "retired": 34,
            "success": 25
          },
          "cost_usd": 2.3821846399999993,
          "duration_ms": 11137641,
          "latest": {
            "agent": null,
            "cost_usd": null,
            "created_at": "2026-07-02T19:42:25.479165513Z",
            "duration_ms": null,
            "id": "b7c8dbe5b444",
            "reason": "task parked: 32 runs today >= max_runs_per_day 20",
            "state": "blocked_budget",
            "updated_at": "2026-07-02T19:42:25.479165513Z"
          },
          "latest_failure": {
            "agent": "cerberus-reviewer@v1",
            "cost_usd": null,
            "created_at": "2026-07-02T17:31:01.676671007Z",
            "duration_ms": 37360,
            "id": "343a2c1bf51b",
            "reason": "harness exit 2: warning: --allow-env OPENROUTER_API_KEY looks like a credential name forwarded into a substrate with webfetch/bash access; a prompt-injected diff could try to exfiltrate it -- consider --openrouter-scoped-key instead, which mints a capped, revocable key per review\nError: parse ReviewArtifact.v1 emitted at /tmp/cerberus-03pBSw/packet/review-artifact.json\n\nCaused by:\n    expected value at line 48 column 18",
            "state": "failure",
            "updated_at": "2026-07-02T17:31:39.13767345Z"
          },
          "recent": 135
        },
        "safe_next_actions": [
          {
            "command": "bb task unpark review",
            "kind": "unpark_after_reason_cleared",
            "reason": "32 runs today >= max_runs_per_day 20"
          },
          {
            "command": "bb runs show 343a2c1bf51b --json",
            "kind": "inspect_artifact",
            "reason": "harness exit 2: warning: --allow-env OPENROUTER_API_KEY looks like a credential name forwarded into a substrate with webfetch/bash access; a prompt-injected diff could try to exfiltrate it -- consider --openrouter-scoped-key instead, which mints a capped, revocable key per review\nError: parse ReviewArtifact.v1 emitted at /tmp/cerberus-03pBSw/packet/review-artifact.json\n\nCaused by:\n    expected value at line 48 column 18"
          }
        ],
        "substrate": "sprites",
        "task": "review",
        "verdict": null
      },
      {
        "agent": "review-coordinator-deepseek@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.75,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-pro",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "review-deepseek",
        "verdict": null
      },
      {
        "agent": "review-coordinator-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.75,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "review-glm",
        "verdict": null
      },
      {
        "agent": "review-coordinator@v3",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.75,
          "max_runs_per_day": 20,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.6:minimal",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "review-kimi",
        "verdict": null
      },
      {
        "agent": "storm-security@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-pro",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "security",
        "verdict": "security"
      },
      {
        "agent": "storm-security-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.5,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "security-glm",
        "verdict": "security-glm"
      },
      {
        "agent": "storm-security-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.5,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "security-kimi",
        "verdict": "security-kimi"
      },
      {
        "agent": "storm-simplification@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "deepseek/deepseek-v4-flash",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "simplification",
        "verdict": "simplification"
      },
      {
        "agent": "storm-simplification-glm@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.35,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "z-ai/glm-5.2",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "simplification-glm",
        "verdict": "simplification-glm"
      },
      {
        "agent": "storm-simplification-kimi@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.35,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "pi",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "moonshotai/kimi-k2.7-code",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "simplification-kimi",
        "verdict": "simplification-kimi"
      },
      {
        "agent": "verifier@v1",
        "budget": {
          "cost_enforcement": {
            "in_flight": true,
            "mode": "kill",
            "source": "task.max_cost_per_run_usd plus agent.policy.side_effect_policy"
          },
          "max_cost_per_run_usd": 0.25,
          "max_runs_per_day": 40,
          "output_bytes_cap": null,
          "runs_today": 0,
          "timeout_minutes": 30,
          "tool_action_cap": null,
          "turn_cap": null
        },
        "dlq": {
          "acknowledged": 0,
          "latest_open": null,
          "open": 0,
          "total": 0
        },
        "harness": "command",
        "ingress": {
          "events": 0,
          "latest": null
        },
        "lease": null,
        "model": "",
        "parked": null,
        "progress": {
          "running": []
        },
        "provider_key": null,
        "queue": {
          "blocked_budget": 0,
          "oldest_pending_age_seconds": null,
          "oldest_pending_created_at": null,
          "pending": 0,
          "running": 0
        },
        "runs": {
          "by_state": {},
          "cost_usd": -0.0,
          "duration_ms": 0,
          "latest": null,
          "latest_failure": null,
          "recent": 0
        },
        "safe_next_actions": [
          {
            "command": "bb status --json",
            "kind": "monitor",
            "reason": "no operator action suggested"
          }
        ],
        "substrate": "sprites",
        "task": "verify",
        "verdict": "verify"
      }
    ]
  },
  "tasks": [
    {
      "agent": "storm-arbiter@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 40,
      "model": "moonshotai/kimi-k2-thinking",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "arbiter",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "arbiter"
    },
    {
      "agent": "bb-builder-rust@v2",
      "agent_role": "builder",
      "agent_skills": [
        "harness-kit/deliver#builder",
        "harness-kit/ci#rust-local-gate",
        "bitterblossom/operator-min"
      ],
      "harness": "omp",
      "max_cost_per_run_usd": 4.0,
      "max_runs_per_day": 5,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "build",
      "timeout_minutes": 90,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 80,
      "verdict": null
    },
    {
      "agent": "bb-builder-rust-glm@v1",
      "agent_role": "builder",
      "agent_skills": [
        "harness-kit/deliver#builder",
        "harness-kit/ci#rust-local-gate",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 1.5,
      "max_runs_per_day": 5,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "build-glm",
      "timeout_minutes": 90,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 80,
      "verdict": null
    },
    {
      "agent": "bb-builder-rust-kimi@v1",
      "agent_role": "builder",
      "agent_skills": [
        "harness-kit/deliver#builder",
        "harness-kit/ci#rust-local-gate",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 1.5,
      "max_runs_per_day": 5,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "build-kimi",
      "timeout_minutes": 90,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 80,
      "verdict": null
    },
    {
      "agent": "canary-triager@v1",
      "agent_role": "diagnoser",
      "agent_skills": [
        "harness-kit/diagnose#incident-triage",
        "harness-kit/canary#report-only-responder",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 0.75,
      "max_runs_per_day": 8,
      "model": "deepseek/deepseek-v4-flash",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "canary-triage",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        },
        {
          "action": null,
          "dedupe_key": "header:X-Delivery-Id",
          "filters": [
            {
              "any_of": [
                "incident.opened",
                "incident.updated"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/event"
            },
            {
              "any_of": [
                "canary"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/incident/service"
            }
          ],
          "kind": "webhook",
          "route": "canary-triage",
          "secret_env": "BB_HOOK_CANARY_TRIAGE"
        }
      ],
      "triggers": 2,
      "turn_cap": 40,
      "verdict": null
    },
    {
      "agent": "ci-diagnoser@v1",
      "agent_role": "diagnoser",
      "agent_skills": [
        "harness-kit/diagnose#ci-failure",
        "harness-kit/ci#failure-triage",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 20,
      "model": "deepseek/deepseek-v4-flash",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "ci-diagnose",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        },
        {
          "action": null,
          "dedupe_key": "json:/check_suite/head_sha",
          "filters": [
            {
              "any_of": [
                "misty-step/bitterblossom"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/repository/full_name"
            },
            {
              "any_of": [
                "completed"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/action"
            },
            {
              "any_of": null,
              "equals": "completed",
              "max": null,
              "not_any_of": null,
              "pointer": "/check_suite/status"
            },
            {
              "any_of": [
                "failure"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/check_suite/conclusion"
            },
            {
              "any_of": [
                "github-actions"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/check_suite/app/slug"
            }
          ],
          "kind": "webhook",
          "route": "ci-diagnose",
          "secret_env": "BB_HOOK_CI_DIAGNOSE"
        }
      ],
      "triggers": 2,
      "turn_cap": 30,
      "verdict": null
    },
    {
      "agent": "ci-diagnoser-glm@v1",
      "agent_role": "diagnoser",
      "agent_skills": [
        "harness-kit/diagnose#ci-failure",
        "harness-kit/ci#failure-triage",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 0.75,
      "max_runs_per_day": 20,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "ci-diagnose-glm",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 30,
      "verdict": null
    },
    {
      "agent": "ci-diagnoser-kimi@v1",
      "agent_role": "diagnoser",
      "agent_skills": [
        "harness-kit/diagnose#ci-failure",
        "harness-kit/ci#failure-triage",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 0.75,
      "max_runs_per_day": 20,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "ci-diagnose-kimi",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 30,
      "verdict": null
    },
    {
      "agent": "storm-correctness@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.6,
      "max_runs_per_day": 40,
      "model": "deepseek/deepseek-v4-pro",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "correctness",
      "timeout_minutes": 45,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "correctness"
    },
    {
      "agent": "storm-correctness-glm@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.6,
      "max_runs_per_day": 40,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "correctness-glm",
      "timeout_minutes": 45,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "correctness-glm"
    },
    {
      "agent": "storm-correctness-kimi@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.6,
      "max_runs_per_day": 40,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "correctness-kimi",
      "timeout_minutes": 45,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "correctness-kimi"
    },
    {
      "agent": "fix-prompter@v1",
      "agent_role": "fix-prompter",
      "agent_skills": [
        "harness-kit/diagnose#fix-prompt",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 0.2,
      "max_runs_per_day": 20,
      "model": "deepseek/deepseek-v4-flash",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "fix-prompt",
      "timeout_minutes": 15,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 25,
      "verdict": null
    },
    {
      "agent": "gardener@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 2,
      "model": "deepseek/deepseek-v4-flash",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "gardener",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        },
        {
          "kind": "cron",
          "schedule": "0 15 * * 1"
        }
      ],
      "triggers": 2,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "gardener-glm@v1",
      "agent_role": "gardener",
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.5,
      "max_runs_per_day": 2,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "gardener-glm",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "gardener-kimi@v1",
      "agent_role": "gardener",
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.5,
      "max_runs_per_day": 2,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "gardener-kimi",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "incident-triager@v1",
      "agent_role": "incident-responder",
      "agent_skills": [
        "harness-kit/diagnose#incident-triage",
        "harness-kit/deliver#builder",
        "harness-kit/code-review#pr-review",
        "bitterblossom/operator-min"
      ],
      "harness": "command",
      "max_cost_per_run_usd": 5.0,
      "max_runs_per_day": 3,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": 120000,
      "parked": null,
      "policy": {
        "authority": "merge",
        "iteration_cap": null,
        "model_allowlist": [
          "z-ai/glm-5.2"
        ],
        "output_bytes_cap": 120000,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": "quarantine",
        "tool_action_cap": null,
        "trigger_bindings": [
          "manual",
          "webhook"
        ],
        "turn_cap": null,
        "wall_clock_minutes": 120
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "incident-triage",
      "timeout_minutes": 120,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        },
        {
          "action": null,
          "dedupe_key": "header:X-Delivery-Id",
          "filters": [
            {
              "any_of": [
                "incident.opened",
                "incident.updated"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/event"
            },
            {
              "any_of": [
                "canary",
                "bastion",
                "powder"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/incident/service"
            }
          ],
          "kind": "webhook",
          "route": "incident-triage",
          "secret_env": "BB_HOOK_INCIDENT_TRIAGE"
        }
      ],
      "triggers": 2,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "model-catalog-watcher@v1",
      "agent_role": "catalog-watcher",
      "agent_skills": [
        "harness-kit/research#model-catalog",
        "harness-kit/harness-engineering#models",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": null,
      "model": "deepseek/deepseek-v4-flash",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "model-catalog-watch",
      "timeout_minutes": 20,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        },
        {
          "kind": "cron",
          "schedule": "0 14 * * *"
        }
      ],
      "triggers": 2,
      "turn_cap": 30,
      "verdict": null
    },
    {
      "agent": "model-evaluator@v1",
      "agent_role": "evaluator",
      "agent_skills": [
        "harness-kit/research#model-selection",
        "harness-kit/harness-engineering#models",
        "bitterblossom/operator-min"
      ],
      "harness": "pi",
      "max_cost_per_run_usd": 2.0,
      "max_runs_per_day": 10,
      "model": "openai/gpt-5.5",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "model-eval",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": 40,
      "verdict": null
    },
    {
      "agent": "storm-product@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 40,
      "model": "x-ai/grok-4.3",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "product",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "product"
    },
    {
      "agent": "storm-product-glm@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.35,
      "max_runs_per_day": 40,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "product-glm",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "product-glm"
    },
    {
      "agent": "storm-product-kimi@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.35,
      "max_runs_per_day": 40,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "product-kimi",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "product-kimi"
    },
    {
      "agent": "cerberus-reviewer@v1",
      "agent_role": "reviewer",
      "agent_skills": [
        "cerberus/review-pr#external-specialist"
      ],
      "harness": "command",
      "max_cost_per_run_usd": 1.25,
      "max_runs_per_day": 20,
      "model": "cerberus-review-pr",
      "output_bytes_cap": null,
      "parked": "32 runs today >= max_runs_per_day 20",
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "review",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        },
        {
          "action": null,
          "dedupe_key": "json:/pull_request/html_url|json:/pull_request/head/sha",
          "filters": [
            {
              "any_of": [
                "misty-step",
                "phrazzld"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/repository/owner/login"
            },
            {
              "any_of": [
                "opened",
                "ready_for_review",
                "synchronize"
              ],
              "equals": null,
              "max": null,
              "not_any_of": null,
              "pointer": "/action"
            },
            {
              "any_of": null,
              "equals": null,
              "max": null,
              "not_any_of": [
                "dependabot[bot]",
                "renovate[bot]"
              ],
              "pointer": "/sender/login"
            },
            {
              "any_of": null,
              "equals": false,
              "max": null,
              "not_any_of": null,
              "pointer": "/pull_request/draft"
            },
            {
              "any_of": null,
              "equals": null,
              "max": 2500.0,
              "not_any_of": null,
              "pointer": "/pull_request/additions"
            },
            {
              "any_of": null,
              "equals": null,
              "max": 50.0,
              "not_any_of": null,
              "pointer": "/pull_request/changed_files"
            }
          ],
          "kind": "webhook",
          "route": "review",
          "secret_env": "BB_HOOK_REVIEW"
        }
      ],
      "triggers": 2,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "review-coordinator-deepseek@v1",
      "agent_role": "reviewer",
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.75,
      "max_runs_per_day": 20,
      "model": "deepseek/deepseek-v4-pro",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "review-deepseek",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "review-coordinator-glm@v1",
      "agent_role": "reviewer",
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.75,
      "max_runs_per_day": 20,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "review-glm",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "review-coordinator@v3",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.75,
      "max_runs_per_day": 20,
      "model": "moonshotai/kimi-k2.6:minimal",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "review-kimi",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": null
    },
    {
      "agent": "storm-security@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 40,
      "model": "deepseek/deepseek-v4-pro",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "security",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "security"
    },
    {
      "agent": "storm-security-glm@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.5,
      "max_runs_per_day": 40,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "security-glm",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "security-glm"
    },
    {
      "agent": "storm-security-kimi@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.5,
      "max_runs_per_day": 40,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "security-kimi",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "security-kimi"
    },
    {
      "agent": "storm-simplification@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 40,
      "model": "deepseek/deepseek-v4-flash",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "simplification",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "simplification"
    },
    {
      "agent": "storm-simplification-glm@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.35,
      "max_runs_per_day": 40,
      "model": "z-ai/glm-5.2",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "simplification-glm",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "simplification-glm"
    },
    {
      "agent": "storm-simplification-kimi@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "pi",
      "max_cost_per_run_usd": 0.35,
      "max_runs_per_day": 40,
      "model": "moonshotai/kimi-k2.7-code",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "simplification-kimi",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "simplification-kimi"
    },
    {
      "agent": "verifier@v1",
      "agent_role": null,
      "agent_skills": [],
      "harness": "command",
      "max_cost_per_run_usd": 0.25,
      "max_runs_per_day": 40,
      "model": "",
      "output_bytes_cap": null,
      "parked": null,
      "policy": {
        "authority": null,
        "iteration_cap": null,
        "model_allowlist": [],
        "output_bytes_cap": null,
        "provider_key_name": null,
        "provider_spend_cap_usd": null,
        "side_effect_policy": null,
        "tool_action_cap": null,
        "trigger_bindings": [],
        "turn_cap": null,
        "wall_clock_minutes": null
      },
      "provider_key": null,
      "runs_today": 0,
      "source": null,
      "substrate": "sprites",
      "task": "verify",
      "timeout_minutes": 30,
      "tool_action_cap": null,
      "trigger_details": [
        {
          "kind": "manual"
        }
      ],
      "triggers": 1,
      "turn_cap": null,
      "verdict": "verify"
    }
  ],
  "runs": [
    {
      "agent_name": "dispatch-demo",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-03T17:34:40.725988545Z",
      "duration_ms": 2320,
      "id": "831e11c3654e",
      "idempotency_key": null,
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "dispatch-demo",
      "trace_id": "339abc276680",
      "trigger_kind": "manual",
      "updated_at": "2026-07-03T17:34:43.23234643Z"
    },
    {
      "agent_name": "incident-triager",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T20:35:27.593091525Z",
      "duration_ms": 251738,
      "id": "3f2e52af59e5",
      "idempotency_key": "manual-drill:INC-ay76lctwao3z:after-89082a1",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "incident-triage",
      "trace_id": "f567b4ef74fa",
      "trigger_kind": "manual",
      "updated_at": "2026-07-02T20:39:39.330556555Z"
    },
    {
      "agent_name": "incident-triager",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T20:27:03.79042321Z",
      "duration_ms": 232604,
      "id": "ab5d92dbe4f7",
      "idempotency_key": "manual-drill:INC-ay76lctwao3z:after-08862ca",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 15: setsid: child 1549 did not exit normally: Success",
      "task": "incident-triage",
      "trace_id": "82763dd3260b",
      "trigger_kind": "manual",
      "updated_at": "2026-07-02T20:30:56.394306644Z"
    },
    {
      "agent_name": "incident-triager",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T20:15:47.807137741Z",
      "duration_ms": 5423,
      "id": "1141ed106fea",
      "idempotency_key": "manual-drill:INC-ay76lctwao3z:DLV-zo93xxqrjhsd",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 127: ",
      "task": "incident-triage",
      "trace_id": "62f238ff4c72",
      "trigger_kind": "manual",
      "updated_at": "2026-07-02T20:15:53.232600328Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:42:25.479165513Z",
      "duration_ms": null,
      "id": "b7c8dbe5b444",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/942|29b9887d675ca6e52baecfcdaa3f9fe0f9f5b981",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: 32 runs today >= max_runs_per_day 20",
      "task": "review",
      "trace_id": "1dbce05b00eb",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:42:25.479165513Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:39:20.004846437Z",
      "duration_ms": null,
      "id": "6cd91be583d3",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/171|7b80ee9373d67ca3427974440d25e396eff1d35e",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: 32 runs today >= max_runs_per_day 20",
      "task": "review",
      "trace_id": "b4ae0daf9c61",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:39:20.004846437Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:38:13.751999126Z",
      "duration_ms": null,
      "id": "22ecfaaa51fb",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/941|1b19364954f5b260a4329a15579f919b4cda6f89",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: 32 runs today >= max_runs_per_day 20",
      "task": "review",
      "trace_id": "177bf465de3c",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:38:13.751999126Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:37:30.296729887Z",
      "duration_ms": null,
      "id": "e2a1e8c6344d",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/171|669f51cd04e76463989f0dc45676025d20bed3ab",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: 32 runs today >= max_runs_per_day 20",
      "task": "review",
      "trace_id": "857990640d23",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:37:30.296729887Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:36:27.275408649Z",
      "duration_ms": null,
      "id": "1f9952f858e3",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/190|179ace024f80b2f0edf9090c158c833fcbcc660a",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: 32 runs today >= max_runs_per_day 20",
      "task": "review",
      "trace_id": "3252f1a69c54",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:36:27.275408649Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:32:45.318472447Z",
      "duration_ms": null,
      "id": "d35af67fd726",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/940|e9f8f6e194caa095b1d362b90d87b612fb67ab50",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "32 runs today >= max_runs_per_day 20",
      "task": "review",
      "trace_id": "b02587538bed",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:32:45.535262707Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T19:03:59.519403435Z",
      "duration_ms": 101830,
      "id": "0a2fce61171b",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/35|3e2886073fd2e13ac039a3a99b72dc5a4c4a39ea",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "c3e1db1b572b",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:05:41.56190887Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T18:57:58.710795265Z",
      "duration_ms": 159905,
      "id": "a77a70573ac2",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/34|5fa7567e6813f5929f6512408541678e2b1bec66",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "daf35420c09a",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T19:00:38.957133922Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T18:41:04.760868693Z",
      "duration_ms": 79107,
      "id": "3e8f5463cc94",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/34|d799138d76f630109d6f1bfc30d1eff00ab0c09d",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "e1eb4a6cd69e",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T18:42:24.107385131Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T18:32:49.750424879Z",
      "duration_ms": 62986,
      "id": "358a56e0c4ac",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/939|d0c50fbd28f1a8ccaab9cfa2a32a59447f647a56",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "fe75b672a1d9",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T18:33:53.220872507Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T18:16:39.174581201Z",
      "duration_ms": 63590,
      "id": "48b934b545ed",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/33|a56d7abdd398a1c51db62c9112ded211982b855d",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "794d9691dd98",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T18:17:42.807825236Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T18:13:04.673743914Z",
      "duration_ms": 54380,
      "id": "1b18f92bfe9a",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/33|992aff3d6b44686dac66c5f99a1edbafa13d365b",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "dd5e85dd2e81",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T18:13:59.484239939Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:59:17.351285066Z",
      "duration_ms": 46667,
      "id": "eff45ee21f95",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/weave/pull/20|d4436c79c7b0e6389c5a34cecea799c672c105bf",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "d7f5d78e52c9",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T18:00:04.334314977Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:33:26.205128251Z",
      "duration_ms": 48975,
      "id": "987e960ec0f5",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/938|7d552c59d3469e13fecdb87b8e244475304135ff",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "53658ea5b437",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:34:15.323607607Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:31:01.676671007Z",
      "duration_ms": 37360,
      "id": "343a2c1bf51b",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/31|2c87fa76564bdf5ab8d88c4ce9162e178b3aacac",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: warning: --allow-env OPENROUTER_API_KEY looks like a credential name forwarded into a substrate with webfetch/bash access; a prompt-injected diff could try to exfiltrate it -- consider --openrouter-scoped-key instead, which mints a capped, revocable key per review\nError: parse ReviewArtifact.v1 emitted at /tmp/cerberus-03pBSw/packet/review-artifact.json\n\nCaused by:\n    expected value at line 48 column 18",
      "task": "review",
      "trace_id": "d4e977f1ba35",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:31:39.13767345Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:25:29.505888899Z",
      "duration_ms": 90613,
      "id": "12eb41792f97",
      "idempotency_key": null,
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "0cc1b62ea90d",
      "trigger_kind": "manual",
      "updated_at": "2026-07-02T17:27:00.121283905Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:24:06.918703975Z",
      "duration_ms": 108777,
      "id": "7e771eed0896",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/937|6e37aa41aa28f4f3dbaab62ec2bba7055996ae09",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "578d2d43e5d2",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:25:55.826047419Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:13:05.520666046Z",
      "duration_ms": null,
      "id": "b9e0734d7343",
      "idempotency_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/31|4161046c2c31f23fbdf16ebe0171d9789470edee",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "superseded by manual dispatch 12eb41792f97, which succeeded and posted the PASS review; original webhook row stuck behind a budget_limits check mystery worth a follow-up ticket",
      "task": "review",
      "trace_id": "885c98964bdb",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:27:46.018268924Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T17:04:13.655054442Z",
      "duration_ms": 135374,
      "id": "a218d8f1d928",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/936|6bd80262f6f047a5135805d3ec21c307d58422f3",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "f5d52e849c9c",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:20:23.730971495Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.0456839796,
      "created_at": "2026-07-02T14:00:00.92315996Z",
      "duration_ms": 229891,
      "id": "0c803861cc73",
      "idempotency_key": "cron:2026-07-02T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "64f68e5c8a87",
      "trigger_kind": "cron",
      "updated_at": "2026-07-02T14:03:51.309866016Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T09:37:44.107378069Z",
      "duration_ms": null,
      "id": "2e230792e695",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/935|44844f4f63b7b7fa711b4a29ab696786f389dbef",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "6cf5323ea430",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:07.285125455Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T09:26:31.687748284Z",
      "duration_ms": null,
      "id": "252f5e9c7b21",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/934|1eec414c0afd00bc6144a8abceb53c30b7f0e2d4",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "bd8c881c96d7",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:06.784916657Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T09:00:56.408178329Z",
      "duration_ms": null,
      "id": "d5c899a19ce3",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/933|56352021f5a4998ac74f8b89c60a3e5c59749534",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "f57b540137bd",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:06.284293338Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T08:22:59.775047084Z",
      "duration_ms": null,
      "id": "fb4a5e37c5aa",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/932|a446a5fe51ea755cce5ba648d1d24dc3150dcd95",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "e2b6340a349d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:05.7848128Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T08:15:58.464814673Z",
      "duration_ms": null,
      "id": "e9126dc52ef7",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/931|a7851a85e64204635e0ef0963f2f72fbad3d8b95",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "7b6f4307b5ec",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:05.283566521Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T08:04:09.446822383Z",
      "duration_ms": null,
      "id": "1f4d6d1c5f17",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/930|587045158254de6693f5e0b753462adc6148268f",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "312fe7be214a",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:04.783054982Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T07:57:29.486077584Z",
      "duration_ms": null,
      "id": "f2b59076b4a9",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/929|344880fbe6f3b6ee452dd84642b65e773e5689f7",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "fc6348969f86",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:04.282241953Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T07:44:02.834354963Z",
      "duration_ms": null,
      "id": "0a42ed01208b",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/928|051e85b88e6a1513c703ec94ed821350a334217d",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "e3cb59a010ea",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:03.781586015Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T07:36:10.173902573Z",
      "duration_ms": null,
      "id": "a3ffcb8e37e0",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/927|041fef9f380b0ad3a0a4dc6826c7db7931999716",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "b41a06e23f17",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:03.281119436Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T07:28:39.304040322Z",
      "duration_ms": null,
      "id": "c8f5cd440828",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/926|3efde42ba50313b63724a1b60bd5590d0de4e29c",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "a919b3695405",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:02.780649947Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T07:04:57.04289324Z",
      "duration_ms": null,
      "id": "9451667fdd60",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/925|e45b839cde3d2b116e67f2d4c205bac3843181f5",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "96d90d5b670a",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:02.279915219Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T06:54:20.171193125Z",
      "duration_ms": null,
      "id": "9402e80ac51d",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/924|186341b101771455b0dccb82a95a4a4f9e2bbcef",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "675d8e95b169",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:01.77911004Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T06:35:53.497143696Z",
      "duration_ms": null,
      "id": "c813d43a33ce",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/923|c6c9268df42b025afd216ec85b8d6107b3bda508",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "b4f90e406b5c",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:01.278589171Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T06:30:45.117348158Z",
      "duration_ms": null,
      "id": "5959d446e1e8",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/922|ec9a438e1a78c16807cca7a0c0661e94ae6126a4",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "7b281c36287d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:00.777879312Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T06:15:40.548544338Z",
      "duration_ms": null,
      "id": "82d9c1e37404",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/921|cb632c5254e11564f94d768ca108fbdc5e5a5e27",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "d333a6337fb6",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:16:00.277377974Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T06:04:49.869342448Z",
      "duration_ms": null,
      "id": "dbc6c8f14b18",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/920|c6f7bcd04fd3c722d2cc0e90d16460813f7b2bca",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "3122a4e6ee6f",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:59.776483564Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:59:47.950027783Z",
      "duration_ms": null,
      "id": "54b42938cd02",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/919|c6592466ba3e4fb0066545c3b3cb3078c6c8a42e",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "553b3b654483",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:59.275771456Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:52:55.442945707Z",
      "duration_ms": null,
      "id": "49111d3a2618",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/918|7da2c470b2bbb24b053af69108e78048f528cdc8",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "b082bd2a62af",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:58.775056308Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:50:13.875893562Z",
      "duration_ms": null,
      "id": "076dced2e2bd",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/917|6b8e3c6f214af13cdc89f35fb187d838b18bb4eb",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "0ea5982d2adb",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:58.275013159Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:44:12.84345609Z",
      "duration_ms": null,
      "id": "2de44089c5ba",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/916|df05510348e721e49ef844c9e8fcb4d79bbb5ab0",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "412e3c67c13a",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:57.77381777Z"
    },
    {
      "agent_name": "canary-triager",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:31:51.303375135Z",
      "duration_ms": 11230,
      "id": "2e3013116b4c",
      "idempotency_key": "wh:canary-triage:DLV-zj5royvznl01",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "dead_letter:1 prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701",
      "task": "canary-triage",
      "trace_id": "c9f438667a9d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T05:32:02.936484338Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:31:30.076285861Z",
      "duration_ms": null,
      "id": "8a991eba91eb",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/915|0d2dc0130004e22cdc80a7db0aa9c702e6d24944",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "f5e24ea56fc8",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:57.272959451Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:25:47.368394096Z",
      "duration_ms": null,
      "id": "bb02acf5ddca",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/914|9d5618270f5daecf4917609e01f99b2ecba0bc63",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "cd1f6fd27fd8",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:56.772347233Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:22:10.762554803Z",
      "duration_ms": null,
      "id": "dd5c98cdcbdf",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/913|d0bb0a84e568f09a7328262ff3b27bc4ace31347",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "e1ca104111b5",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:56.271552144Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:20:23.538514234Z",
      "duration_ms": null,
      "id": "3168c75c2b6f",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/913|55dde26eb1cac8adc949830ea9eec25b37083fcb",
      "parent_run_id": null,
      "state": "blocked_budget",
      "state_reason": "task parked: halting stale-PR backlog drain (59 pending from 2026-06-16 onward); re-scoping to target runs only",
      "task": "review",
      "trace_id": "4570d0b3515b",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:55.771698176Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:16:27.014581161Z",
      "duration_ms": null,
      "id": "e389b7fdaba1",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/912|4f44c27fabc0fce64b968d58e8e13d7bce47f698",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "f22dfd4c10e9",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.179811077Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:10:54.00489935Z",
      "duration_ms": null,
      "id": "7959c873df52",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/911|b8f3adc48d28149ebe8a4468ebea3a9d2e11a1b0",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "fa3faf516869",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.18550346Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:05:39.933106196Z",
      "duration_ms": null,
      "id": "2b86cff44106",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/910|fee9bf7509da994e4a7302685f781d2f3462c1e8",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "20c7788de517",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.190985472Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T05:02:04.304213223Z",
      "duration_ms": null,
      "id": "2176a639b82f",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/909|6d4fb3df2cb631373c3c29ddc0b6b98cf24b282c",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "82d16a588f56",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.196699155Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:59:22.308121035Z",
      "duration_ms": null,
      "id": "dbfd22c2fad1",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/908|b63ecd3bd5b6613558aefe47766bd9a778d4e395",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "eb32249b08c2",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.202437428Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:54:22.135263493Z",
      "duration_ms": null,
      "id": "3fcec38233b1",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/907|5c4790cdd2318211381cd891a5dc7d3116f020b8",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "694ca592bfca",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.20776971Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:49:46.060691433Z",
      "duration_ms": null,
      "id": "7e0d1ffd9bba",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/906|5cc651369fa38c0ef28793ee5603f3e0d67f97c0",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "cdf5cf9fabd2",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.213063813Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:43:29.515898423Z",
      "duration_ms": null,
      "id": "435b75b6809a",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/905|242df13e89214c842ba5989594b1db92e9292a21",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "1d0d7e765b2c",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.218685085Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:40:37.647584938Z",
      "duration_ms": null,
      "id": "e8706f3ca7d6",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/904|662ae14a6c6e655258cf42889c92446664ebe208",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "d9500808d234",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.224120708Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:37:24.529669595Z",
      "duration_ms": 3413,
      "id": "86810e638b14",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/903|a421063a28c2f2095b5f2e5ec6ecfb7224d1ba3b",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "4184c4cc3905",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:37:27.978598386Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:32:04.592237016Z",
      "duration_ms": 2213,
      "id": "f29b46789e16",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/902|5bcd029eaf7053a277d0020b30899b709d6c0b7d",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "5d7edf3cb64e",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:32:07.117615251Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:27:53.243365852Z",
      "duration_ms": 2618,
      "id": "1074185d1b52",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/901|086c17b5da87f6788bdbc07913f53b27c0b63cc8",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "085be761b5a6",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:27:55.897600359Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:23:34.790506104Z",
      "duration_ms": 4715,
      "id": "0d38b6b50b8a",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/900|447756ce72cdd4f767c0b1988fceab8e1ada7339",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "5ec42b4dcda0",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:23:39.859950212Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:20:23.741100514Z",
      "duration_ms": 2113,
      "id": "5c3effce348b",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/899|a66f722c6eaee322876295a2a6b807e0fddfbb10",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "5350641e14c6",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:20:26.163320261Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:17:03.714795087Z",
      "duration_ms": 6620,
      "id": "e2349dea99a9",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/898|acfbe7945f1b9e6242f35a42b793fe70e0145e3c",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "1d7a92940fde",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:17:10.567529921Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:14:13.81574172Z",
      "duration_ms": 2514,
      "id": "81194f49e269",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/897|8337d398cc573162f8c69e4cc34a923bb2cb40fa",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "b24cb0c6365c",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:14:16.377782952Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:10:33.145314878Z",
      "duration_ms": 2415,
      "id": "3a50d65719ac",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/896|1184847c4ceb1fbacc16d934da411ab37ae1d63b",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "72740fd36235",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:10:35.671921849Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:06:41.171020545Z",
      "duration_ms": 5220,
      "id": "bcb657aba91e",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/895|3d088a938fcd925de7cf43051086529fbd06c183",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "fec589bd3037",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:06:46.859607532Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T04:00:27.823808249Z",
      "duration_ms": 3218,
      "id": "f4d84be828ec",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/894|984d25d171f4bdbc0bfc300c86671897f895c026",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "a450a9d5227b",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T04:00:31.168605024Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T03:58:03.894560338Z",
      "duration_ms": 5520,
      "id": "a53bbfb0adaf",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/893|51a68dd2c94934cfee73c96b90439a2351cad011",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "418dbdf335f3",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T03:58:09.900757069Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T03:53:11.351521265Z",
      "duration_ms": 3816,
      "id": "5af5dbbfc776",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/892|efd65b6b514ed429da7763adab2c904cf5eb70cf",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "7d537126f58e",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T03:53:15.551615588Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T03:50:10.79792381Z",
      "duration_ms": 2416,
      "id": "94478400993d",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/891|9fd8e7348a51154d98ee9907b6eb3aa70b7b7f79",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "e22338681b63",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T03:50:13.56640466Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T03:44:17.174812515Z",
      "duration_ms": 2916,
      "id": "881c9a4a4462",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/890|730a7184183c87fc2dbbe5f6f79901779d225c2a",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "935650e95ca7",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T03:44:20.389293388Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T03:41:32.298936324Z",
      "duration_ms": 15044,
      "id": "1b7f62c3d247",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/889|a1e75fa85a13262881ca296dbf0ae1188eb3aa72",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "9983d6ed07b5",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T03:41:47.437669931Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T03:02:30.118820664Z",
      "duration_ms": 7942,
      "id": "a6e2fd520eef",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/887|c90b3dbe35b7cc2e5652c02dfb38ec6d4d8388ef",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "398319e4eee0",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T03:02:38.191129269Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T02:23:03.381708058Z",
      "duration_ms": 23740,
      "id": "cfaddad07faa",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/886|8216ae034d1b228b294a477218a2a13c4920600c",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "04fa503d7c5d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T02:23:27.320522903Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T01:44:23.682841713Z",
      "duration_ms": 11929,
      "id": "8b0dd049d131",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/885|39be1f453825e6e54bb54476494b5b6c62fbbc5b",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "5364961a712a",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T01:44:35.871335526Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T00:26:50.825318926Z",
      "duration_ms": 6522,
      "id": "e40e2fbb6c03",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/884|1bd5dd29cbde99cbc99f7835b147ea1be2e31a74",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "a4c6053e86ea",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T00:26:57.680465861Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-02T00:14:54.177619901Z",
      "duration_ms": 51378,
      "id": "a47d6a0903a3",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/883|1e57f03d34d2eee9bc053d00a6c7678cce187747",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "bca330182bc2",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T00:15:45.683204983Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T23:22:04.782599111Z",
      "duration_ms": 6223,
      "id": "e9d5072fd99c",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/882|06068ab27a21dcf8845ac8ec8ef3fe3269fac020",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "e86e7f56dbe3",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T23:22:11.449463031Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T23:13:51.482818221Z",
      "duration_ms": 7628,
      "id": "ee32c1b39d8d",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/881|24ad5caab06079f2427834b17b17f8796f246bf0",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "4bf81fb6d4bc",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T23:13:59.599959041Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T23:07:23.242955276Z",
      "duration_ms": 5519,
      "id": "08eb3dbe6fa9",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/880|7591d97567adb5a9538f8fc5cffb4bbbdd5a9a56",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for GitHub reads and posting",
      "task": "review",
      "trace_id": "893aaa5831f2",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T23:07:28.802321847Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T22:53:39.756615271Z",
      "duration_ms": 62088,
      "id": "b8d342f6aaf9",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/879|44bec240d391592369189e1738fff832664b8c82",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "c2364000e9cc",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T22:54:41.966836142Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T22:44:43.169072499Z",
      "duration_ms": 2717,
      "id": "e12d5f49106d",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/878|fdcca380a29d4bd6d6989d51c1d11078887e9b1d",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "9c0959d0d231",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T22:44:46.33767037Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T22:39:14.259128317Z",
      "duration_ms": 2320,
      "id": "20f92868b1b3",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/877|f9b6e8a07857a06780e9f910b06d5e23d2f39c83",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "a7c88f64accc",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T22:39:16.78100411Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T22:31:03.123173094Z",
      "duration_ms": 3220,
      "id": "7e5aba81a30e",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/876|23635e34fd6e01c1f2f7352e0161e0e3bd9f54cc",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "9212f696b675",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T22:31:06.446637979Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T22:22:05.981641815Z",
      "duration_ms": 5418,
      "id": "5803526a4244",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/875|469fc01fc0b01fa578692fb5eb0a7bf905eef98d",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "f06150263e02",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T22:22:11.884349282Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T20:53:41.77954904Z",
      "duration_ms": 3318,
      "id": "5699a6651800",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/874|8f1d1bf02bba2bddca5b32bbca6952552248ffd2",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "f5544895f447",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T20:53:45.229796346Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T19:17:31.094102111Z",
      "duration_ms": 16033,
      "id": "9f8bc543c356",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/873|927e0c7a9537ff772e94266e5012b1982d0b6a6e",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "afeded092c8d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T19:17:47.614909785Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T17:56:58.427977744Z",
      "duration_ms": 2117,
      "id": "90b91211d517",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/871|b333453614034e013e0fd86862adcfc9b7c82729",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "35016553994d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T17:57:00.974624365Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T17:55:35.803228655Z",
      "duration_ms": 2920,
      "id": "218ecc8c3c23",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/872|2afafa05782b6e68d5478be8c7f1b53a25e350f1",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "64baecf02c7d",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T17:55:38.738062224Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T17:41:55.878294201Z",
      "duration_ms": 51082,
      "id": "23ba478a2079",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/871|b6131b80b3afe31b750c5cb9c00535f10d7fdcd3",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "27aaa690e02f",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T17:42:46.997753428Z"
    },
    {
      "agent_name": "canary-triager",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.0175550618,
      "created_at": "2026-07-01T17:40:14.317729448Z",
      "duration_ms": 116769,
      "id": "356ed977067c",
      "idempotency_key": "wh:canary-triage:DLV-c1id5ljacfzn",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "canary-triage",
      "trace_id": "028688fe628b",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T17:42:11.135847147Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T17:06:03.499022978Z",
      "duration_ms": 31353,
      "id": "8c6ddb5678f3",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/870|d0e737729e76b9b921d02f757718753d9e52b49b",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: review-pr --post requires an explicit GitHub token via --gh-token-file <path> or --gh-token-env <VAR>; ambient gh auth is refused for posting",
      "task": "review",
      "trace_id": "ac225fec2755",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T17:06:35.237674503Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T16:40:54.065266929Z",
      "duration_ms": 72234,
      "id": "06473df79768",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/869|110342fb01b74ce061f65ffa1a2111e3ce43558e",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: parse ReviewArtifact.v1 emitted at /tmp/cerberus-dij4sH/packet/review-artifact.json\n\nCaused by:\n    invalid type: string \"high\", expected f32 at line 33 column 26",
      "task": "review",
      "trace_id": "7893126c43db",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T16:42:06.393315845Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-07-01T16:35:35.99888897Z",
      "duration_ms": 208670,
      "id": "9361f9067f86",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/869|d2ee706ef58c4a55582bf25fb20397d6c34f0f31",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 2: Error: parse ReviewArtifact.v1 emitted at /tmp/cerberus-9jtsmU/packet/review-artifact.json\n\nCaused by:\n    invalid type: string \"high\", expected f32 at line 33 column 26",
      "task": "review",
      "trace_id": "888c4318bc84",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-01T16:39:04.683513307Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.0311208106,
      "created_at": "2026-07-01T14:00:00.981473694Z",
      "duration_ms": 129375,
      "id": "b2ecc0925f03",
      "idempotency_key": "cron:2026-07-01T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "6b934706d2eb",
      "trigger_kind": "cron",
      "updated_at": "2026-07-01T14:02:10.456326553Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.040167224200000004,
      "created_at": "2026-06-30T14:00:08.86922315Z",
      "duration_ms": 295675,
      "id": "aa494640246c",
      "idempotency_key": "cron:2026-06-30T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "c7f3c5dc6021",
      "trigger_kind": "cron",
      "updated_at": "2026-06-30T14:05:04.801041571Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.030583573300000003,
      "created_at": "2026-06-29T14:00:00.541287329Z",
      "duration_ms": 175248,
      "id": "9844b05e7c23",
      "idempotency_key": "cron:2026-06-29T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "e36d4c08f825",
      "trigger_kind": "cron",
      "updated_at": "2026-06-29T14:02:56.247563458Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-28T16:23:20.506598551Z",
      "duration_ms": 55486,
      "id": "9ad35ccd13ad",
      "idempotency_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/868|f7b957604cb58aad654f4f9fe254eae92a3830bd",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "40ee058971d3",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-28T16:24:16.120577741Z"
    },
    {
      "agent_name": "gardener",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.041485347799999996,
      "created_at": "2026-06-28T15:00:05.412130215Z",
      "duration_ms": 268743,
      "id": "3fb36e86c7d5",
      "idempotency_key": "cron:2026-06-28T15:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "gardener",
      "trace_id": "4af09fe0d9b1",
      "trigger_kind": "cron",
      "updated_at": "2026-06-28T15:04:34.537226734Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.0234773125,
      "created_at": "2026-06-28T14:00:04.32690913Z",
      "duration_ms": 136780,
      "id": "58ee5e763484",
      "idempotency_key": "cron:2026-06-28T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "b732df41d877",
      "trigger_kind": "cron",
      "updated_at": "2026-06-28T14:02:21.387533136Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.022625290600000004,
      "created_at": "2026-06-27T14:00:08.233537074Z",
      "duration_ms": 180362,
      "id": "ae304fb042fc",
      "idempotency_key": "cron:2026-06-27T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "0bd28a8a7f97",
      "trigger_kind": "cron",
      "updated_at": "2026-06-27T14:03:08.774536773Z"
    },
    {
      "agent_name": "model-catalog-watcher",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.0459556022,
      "created_at": "2026-06-26T14:00:02.087950445Z",
      "duration_ms": 251922,
      "id": "bfabaa5bd624",
      "idempotency_key": "cron:2026-06-26T14:00:00+00:00",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "model-catalog-watch",
      "trace_id": "0f2597498ee9",
      "trigger_kind": "cron",
      "updated_at": "2026-06-26T14:04:14.075706445Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-25T17:55:43.797692302Z",
      "duration_ms": 78814,
      "id": "0d12f1818992",
      "idempotency_key": "cerberus-canary-20260625-bun-openrouter-1",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "2c0758b57d64",
      "trigger_kind": "manual",
      "updated_at": "2026-06-25T17:57:13.123156358Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-25T17:50:12.243229084Z",
      "duration_ms": 6923,
      "id": "2b5bb2c768cc",
      "idempotency_key": "cerberus-canary-20260625-omp-fallback-1",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 1: Error: read emitted review artifact /tmp/cerberus-EccCCN/packet/review-artifact.json\n\nCaused by:\n    No such file or directory (os error 2)",
      "task": "review",
      "trace_id": "90c2d0d21e78",
      "trigger_kind": "manual",
      "updated_at": "2026-06-25T17:50:30.499598369Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-25T17:43:57.443568441Z",
      "duration_ms": 6127,
      "id": "9a9c12096eb9",
      "idempotency_key": "cerberus-canary-20260625-wrapper-rebuild-1",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 1: Error: harness binary \"opencode\" was not found in trusted search path: /usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin",
      "task": "review",
      "trace_id": "f28a386931d4",
      "trigger_kind": "manual",
      "updated_at": "2026-06-25T17:44:13.533363893Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-25T17:37:52.445891577Z",
      "duration_ms": 2717,
      "id": "7f8fceefdcc1",
      "idempotency_key": "cerberus-canary-20260625-change-type-1",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 1: Error: parse gh pr view JSON\n\nCaused by:\n    missing field `changeType` at line 1 column 1263",
      "task": "review",
      "trace_id": "199011c288dc",
      "trigger_kind": "manual",
      "updated_at": "2026-06-25T17:38:12.515965695Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-25T17:27:16.076758086Z",
      "duration_ms": 2920,
      "id": "17591327a7a9",
      "idempotency_key": null,
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 1: Error: parse gh pr view JSON\n\nCaused by:\n    missing field `changeType` at line 1 column 1263",
      "task": "review",
      "trace_id": "713e9ea55aea",
      "trigger_kind": "manual",
      "updated_at": "2026-06-25T17:30:25.505291283Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-25T16:22:13.940527061Z",
      "duration_ms": null,
      "id": "bd8dda640578",
      "idempotency_key": "wh:review:f9c80e08f13a58368b098d02bb0ec72f272ec152",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "3c15b9befeaf",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.229534821Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-21T00:19:34.206438967Z",
      "duration_ms": null,
      "id": "e2610fbbe1fd",
      "idempotency_key": "wh:review:04add88b0f14354bb33b4778f3020bd7969607af",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "a65452760565",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.234969483Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-20T23:39:41.675031586Z",
      "duration_ms": null,
      "id": "4075fb12e4f1",
      "idempotency_key": "wh:review:69ca0ef215906511a2bca2647333f53d11be6c12",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "8682e3c79d28",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.241527376Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-20T23:11:33.618590547Z",
      "duration_ms": null,
      "id": "6d682c4f1ab2",
      "idempotency_key": "wh:review:e59432ab1b0a5357e1e86eb3a1d762354e9f1ddd",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "a18b5f50222a",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.24877261Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-17T20:52:36.925836341Z",
      "duration_ms": null,
      "id": "5776c30ccbed",
      "idempotency_key": "wh:review:9a299f475576d5cec1b5c28da3daf704fd25c611",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "36166cbc87d6",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.256445303Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-17T18:18:48.398762989Z",
      "duration_ms": null,
      "id": "1bb01669398f",
      "idempotency_key": "wh:review:751deaef8d9d55ed9e65c8b50e36ff930578382b",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "ccf1160e55bc",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.264145307Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-17T13:42:24.998719209Z",
      "duration_ms": null,
      "id": "fb379e102ece",
      "idempotency_key": "wh:review:7a88ced7b8558dbd839198cb67004a221031058c",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "4afc5bb29bf1",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.27010308Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-17T04:42:27.392853135Z",
      "duration_ms": null,
      "id": "fb16690dd35b",
      "idempotency_key": "wh:review:04a996d53a1a927ef4b62db8b829fcd68c5d683d",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "d6c6e885e12e",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.275612113Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T22:17:57.016204966Z",
      "duration_ms": null,
      "id": "4f107f3d5902",
      "idempotency_key": "wh:review:ee87bd70a1a13df3a0e3f1641df7e21cfbc85d1b",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "fa01ea061202",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.281369645Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T22:17:16.221475475Z",
      "duration_ms": null,
      "id": "834415072521",
      "idempotency_key": "wh:review:36c399d062ddb23ae835e7597c37a0319bad0aa4",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "18c58dc9bc90",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.286865788Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T21:36:04.352038834Z",
      "duration_ms": null,
      "id": "f82942338f6b",
      "idempotency_key": "wh:review:cc578c02f9c23490f97415bf75cd1b2a9f3b04ae",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "946f83f89783",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.292352161Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T21:20:06.377345913Z",
      "duration_ms": null,
      "id": "0d8ddf688bbd",
      "idempotency_key": "wh:review:631a3be75f666af1c3c58a51dfa1caca74c98d3f",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "7fecefe98570",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.297933383Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T21:08:19.383462208Z",
      "duration_ms": null,
      "id": "d7292f4acb7c",
      "idempotency_key": "wh:review:1d3afd84d19e05d4c9805a90ee50671dd2e0aa1a",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "870234a33683",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.303350326Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T21:00:38.597622183Z",
      "duration_ms": null,
      "id": "5dcfe85288d2",
      "idempotency_key": "wh:review:e8d21e0a9f6455976271ba875765ca235c7bb3d0",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "0dc4e6e218db",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.308839338Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T20:49:13.364054362Z",
      "duration_ms": null,
      "id": "b13457a78779",
      "idempotency_key": "wh:review:f29866f90155258230ae3f59d1c728ef33f4963e",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "4fed1bb59ce6",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.314502641Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T19:33:48.686506882Z",
      "duration_ms": null,
      "id": "93737fc73f87",
      "idempotency_key": "wh:review:473cbbb17a8d0f8d7cd612c6b4c541df95db6e5c",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "52e464b2e500",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.319949144Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T19:22:22.52147847Z",
      "duration_ms": null,
      "id": "ef7a883ec9d8",
      "idempotency_key": "wh:review:0ecf165b784a6122919a780b009ace3f3c57ecfe",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "156455df97b3",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.325475986Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T18:37:39.920941691Z",
      "duration_ms": null,
      "id": "70a01b5c5032",
      "idempotency_key": "wh:review:5d906cabbcc9e53570be92cf2a5f16f8e90a15c9",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "9046e290c8c0",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.331110499Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T18:24:24.91936795Z",
      "duration_ms": null,
      "id": "2f4cd84dff96",
      "idempotency_key": "wh:review:0468f6c18221e788aed2d3487a90b51703a6c328",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "37283b27eb5f",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.336724562Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T15:39:25.853803713Z",
      "duration_ms": null,
      "id": "42d8838bba20",
      "idempotency_key": "wh:review:d4f184d0079a3f22dabbc2014a7518b55ea0167d",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "b9f9b7e1b34c",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.342667085Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T00:36:51.366884025Z",
      "duration_ms": null,
      "id": "5f267b602007",
      "idempotency_key": "wh:review:f64661f67358ed4cb3fd98061617a937d3eacbdd",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "6e348e621f47",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.347999707Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T00:21:16.82858169Z",
      "duration_ms": null,
      "id": "1f662bb2dc94",
      "idempotency_key": "wh:review:72ef05e2e63e4c80717111b9655bebf3fb0be24a",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "0b0e82720bb9",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.35343454Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T00:05:52.940881932Z",
      "duration_ms": null,
      "id": "c42f6989bcce",
      "idempotency_key": "wh:review:72ff8a7cab57c513d114baf4513986e34fbbe3f3",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "6cf2cfc64d69",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.358813712Z"
    },
    {
      "agent_name": null,
      "agent_version": null,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-16T00:02:09.071153026Z",
      "duration_ms": null,
      "id": "b934d5698686",
      "idempotency_key": "wh:review:803061e8d44e81f07a87224066cbc280013ca749",
      "parent_run_id": null,
      "state": "retired",
      "state_reason": "stale-2026-07-02-backlog",
      "task": "review",
      "trace_id": "c786383f751f",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:18:00.364183635Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-15T22:01:55.791930732Z",
      "duration_ms": 128676,
      "id": "287927ccc03b",
      "idempotency_key": "wh:review:062d0572776849bc966503b12ab1e05067a0480d",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "4e6735c00d43",
      "trigger_kind": "webhook",
      "updated_at": "2026-07-02T17:15:38.698201732Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-15T21:55:17.612811021Z",
      "duration_ms": 2118,
      "id": "7d20a25b5f36",
      "idempotency_key": "wh:review:926529f12be6d7ee5034eba3f3d381bc7e64541c",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 1: Error: gh pr view 842 --json number,title,body,url,baseRefName,baseRefOid,headRefName,headRefOid,files -R misty-step/bitterblossom failed: HTTP 401: Bad credentials (https://api.github.com/graphql)\nTry authenticating with:  gh auth login",
      "task": "review",
      "trace_id": "38aa21c9e1ec",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-25T17:28:31.173085209Z"
    },
    {
      "agent_name": "cerberus-reviewer",
      "agent_version": 1,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-15T21:47:33.803838173Z",
      "duration_ms": 27148,
      "id": "7af49577152f",
      "idempotency_key": "wh:review:b617b3fb7f4e8a9af29dcb0cbf2129f3a8bbad3c",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "harness exit 1: Error: gh pr view 841 --json number,title,body,url,baseRefName,baseRefOid,headRefName,headRefOid,files -R misty-step/bitterblossom failed: HTTP 401: Bad credentials (https://api.github.com/graphql)\nTry authenticating with:  gh auth login",
      "task": "review",
      "trace_id": "c7619df9fb1a",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-25T17:28:28.683385702Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.9708088699999999,
      "created_at": "2026-06-15T21:24:58.870440234Z",
      "duration_ms": 557775,
      "id": "516385b5f22c",
      "idempotency_key": "wh:review:87e007fdee5a4b89f7c353a536ec10407cd2e010",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "2a33789de5cd",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-15T21:34:16.688067048Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.0450112,
      "created_at": "2026-06-13T22:26:22.656292231Z",
      "duration_ms": 201754,
      "id": "ea152ebbc389",
      "idempotency_key": "wh:review:65d7aa472b1f8a6413c3aff8aaac2ea9cb41ee4f",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "30e192502e70",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T22:29:44.773127003Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.18387589000000004,
      "created_at": "2026-06-13T16:11:28.844688383Z",
      "duration_ms": 212167,
      "id": "bdc925507b27",
      "idempotency_key": "wh:review:4506fdfaea4821a64ced73fd35b63a6e21ff9c2d",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "ddfcca363147",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T16:15:01.419494352Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.20408191999999997,
      "created_at": "2026-06-13T15:49:58.542266442Z",
      "duration_ms": 341308,
      "id": "f1bc495fd652",
      "idempotency_key": "wh:review:3756a4d4b43652f0b3ba61cdc2d3b643ad9c328c",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "33fa413131f2",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T16:02:56.696820955Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.10372864,
      "created_at": "2026-06-13T15:48:43.523574033Z",
      "duration_ms": 322981,
      "id": "26709b34a927",
      "idempotency_key": "wh:review:373e48bb462f3526262773dd858dcbdbb3a310bf",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "e82e3bc61d86",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T15:57:15.238450862Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.06016937,
      "created_at": "2026-06-13T15:18:27.590018412Z",
      "duration_ms": 137971,
      "id": "85af0f04311e",
      "idempotency_key": "wh:review:1ed3988997f613f1c86f8ab996383dba41339147",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "92ba504a4dd4",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T15:51:52.161855388Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.30596332000000004,
      "created_at": "2026-06-13T15:10:46.542062609Z",
      "duration_ms": 722544,
      "id": "76f3f7a100cc",
      "idempotency_key": "wh:review:ecfa81f78eecc84130253d321071689ba8de25da",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "6691c782ff12",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T15:49:33.915257071Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.13543308,
      "created_at": "2026-06-13T15:10:00.581923268Z",
      "duration_ms": 1046930,
      "id": "f69275df26b2",
      "idempotency_key": "wh:review:3b7e30aacdb59ccfabaa698d67d01e72dd25ff0e",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "363b36845522",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T15:37:31.327069504Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-13T15:05:47.972110575Z",
      "duration_ms": 856207,
      "id": "ad64376e9596",
      "idempotency_key": "wh:review:17b3be3a766a3d315451858cb25115e03a021118",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "unparseable harness output: pi output: assistant message has no text content",
      "task": "review",
      "trace_id": "69e6a14e7ebe",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-13T15:20:04.228736248Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.06122844999999999,
      "created_at": "2026-06-12T21:10:44.98238575Z",
      "duration_ms": 105850,
      "id": "e8e8cfd3f365",
      "idempotency_key": "wh:review:3947de0fc5714d915933e3cbd35e5b777689b5ea",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "e0e12853fab2",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-12T21:12:31.267471513Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.20968486,
      "created_at": "2026-06-12T21:00:16.624709579Z",
      "duration_ms": 497004,
      "id": "4636a4164125",
      "idempotency_key": "wh:review:c1505cdb21d6340f25b2c9e38047a50a251f5cd8",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "b2a0497e471d",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-12T21:08:33.655487451Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-12T20:30:02.205578498Z",
      "duration_ms": 1801878,
      "id": "6308d6ba2116",
      "idempotency_key": "wh:review:404253ce14b3b4b36a0932e15e17556989ba6d9f",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "timeout after 1800s",
      "task": "review",
      "trace_id": "357630463882",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-12T21:00:04.242833035Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": null,
      "created_at": "2026-06-12T18:45:26.23057007Z",
      "duration_ms": 1800848,
      "id": "c2375fce5e9e",
      "idempotency_key": "wh:review:e6b1807e82b4087f027a3f4761b0e23c9a416613",
      "parent_run_id": null,
      "state": "failure",
      "state_reason": "timeout after 1800s",
      "task": "review",
      "trace_id": "9c24cb9952bc",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-12T19:15:27.522385187Z"
    },
    {
      "agent_name": "review-coordinator",
      "agent_version": 2,
      "config_source_ref": null,
      "config_source_repo": null,
      "cost_usd": 0.10219903999999999,
      "created_at": "2026-06-12T18:30:07.64992008Z",
      "duration_ms": 582789,
      "id": "6c24c4fac8b8",
      "idempotency_key": "wh:review:a5aafd93a8c07f3498302d65100c0f197d4294dd",
      "parent_run_id": null,
      "state": "success",
      "state_reason": null,
      "task": "review",
      "trace_id": "665032ae86aa",
      "trigger_kind": "webhook",
      "updated_at": "2026-06-12T18:39:50.593691851Z"
    }
  ],
  "dlq": [
    {
      "id": 1,
      "run_id": "2e3013116b4c",
      "task": "canary-triage",
      "payload": "{\"event\":\"incident.updated\",\"incident\":{\"id\":\"INC-noyl4xbdsn26\",\"opened_at\":\"2026-07-01T17:40:11.816819599Z\",\"resolved_at\":null,\"service\":\"canary\",\"severity\":\"medium\",\"signals\":[{\"attached_at\":\"2026-07-01T17:40:11.816819599Z\",\"resolved_at\":\"2026-07-02T05:31:47.124657742Z\",\"signal_ref\":\"f371a3e21b1cc5462353a4b30e2bd64c7bc42fb4baa08dda1a6eac9ff3b4e3bc\",\"signal_type\":\"error_group\"},{\"attached_at\":\"2026-07-02T05:31:47.124657742Z\",\"resolved_at\":null,\"signal_ref\":\"MON-5csw7imodfcq\",\"signal_type\":\"health_transition\"}],\"state\":\"investigating\",\"title\":\"canary incident\"},\"project_id\":\"PROJECT-bootstrap\",\"replay\":{\"incident_url\":\"/api/v1/incidents/INC-noyl4xbdsn26\",\"report_url\":\"/api/v1/report?window=1h\",\"timeline_url\":\"/api/v1/timeline?service=canary&window=1h\"},\"schema_version\":\"canary.incident_event.v1\",\"signal\":{\"fingerprint\":\"f371a3e21b1cc5462353a4b30e2bd64c7bc42fb4baa08dda1a6eac9ff3b4e3bc\",\"kind\":\"error_group\",\"observed_at\":\"2026-07-01T17:40:11.816819599Z\",\"severity\":\"medium\"},\"subject\":{\"id\":\"INC-noyl4xbdsn26\",\"service\":\"canary\",\"type\":\"incident\"},\"tenant_id\":\"TENANT-bootstrap\",\"timestamp\":\"2026-07-02T05:31:47.124657742Z\"}",
      "error": "prepare: repo sync https://github.com/misty-step/bitterblossom.git failed: fatal: couldn't find remote ref factory/bitterblossom-lane-20260701",
      "created_at": "2026-07-02T05:32:02.936189258Z",
      "replayed_run_id": null,
      "acknowledged_reason": null,
      "acknowledged_at": null,
      "status": "open"
    }
  ],
  "leases": [],
  "ingress": [
    {
      "id": 149,
      "run_id": "831e11c3654e",
      "task": "dispatch-demo",
      "trigger_kind": "manual",
      "source_event_id": null,
      "dedupe_key": null,
      "payload_hash": "e254bb2856de6adb3fcb0c8ff2592551d0cb14bb6090ca54c7550c447c6a7249",
      "duplicate": false,
      "received_at": "2026-07-03T17:34:40.725988545Z"
    },
    {
      "id": 148,
      "run_id": "3f2e52af59e5",
      "task": "incident-triage",
      "trigger_kind": "manual",
      "source_event_id": null,
      "dedupe_key": "manual-drill:INC-ay76lctwao3z:after-89082a1",
      "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
      "duplicate": false,
      "received_at": "2026-07-02T20:35:27.593091525Z"
    },
    {
      "id": 147,
      "run_id": "ab5d92dbe4f7",
      "task": "incident-triage",
      "trigger_kind": "manual",
      "source_event_id": null,
      "dedupe_key": "manual-drill:INC-ay76lctwao3z:after-08862ca",
      "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
      "duplicate": false,
      "received_at": "2026-07-02T20:27:03.79042321Z"
    },
    {
      "id": 146,
      "run_id": "1141ed106fea",
      "task": "incident-triage",
      "trigger_kind": "manual",
      "source_event_id": null,
      "dedupe_key": "manual-drill:INC-ay76lctwao3z:DLV-zo93xxqrjhsd",
      "payload_hash": "39bf281f5d346f3b639c91e0c13ecc0ebac67febefb25769554c8e3244c30f7c",
      "duplicate": false,
      "received_at": "2026-07-02T20:15:47.807137741Z"
    },
    {
      "id": 145,
      "run_id": "b7c8dbe5b444",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "260f0040-764e-11f1-8223-fbaf3c8987f9",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/942|29b9887d675ca6e52baecfcdaa3f9fe0f9f5b981",
      "payload_hash": "64c711a02327887539ff7fe3e3284808a27aeb87a60dfb187b0fe641ddc6be29",
      "duplicate": false,
      "received_at": "2026-07-02T19:42:25.479165513Z"
    },
    {
      "id": 144,
      "run_id": "6cd91be583d3",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "b78126d0-764d-11f1-84f7-8cb912650f39",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/171|7b80ee9373d67ca3427974440d25e396eff1d35e",
      "payload_hash": "09a896a2701408c53d7f8bcece525db9f03f0cca6ac9d59d9151cc7ae6c3ca84",
      "duplicate": false,
      "received_at": "2026-07-02T19:39:20.004846437Z"
    },
    {
      "id": 143,
      "run_id": "22ecfaaa51fb",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "90096450-764d-11f1-80e1-061b4e076951",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/941|1b19364954f5b260a4329a15579f919b4cda6f89",
      "payload_hash": "3ac37cd6b8dc23fad6180db76b73e4f5077baec4ef34e2c404f6b36592f70cca",
      "duplicate": false,
      "received_at": "2026-07-02T19:38:13.751999126Z"
    },
    {
      "id": 142,
      "run_id": "e2a1e8c6344d",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "762a5080-764d-11f1-9180-8b894ee5b4cc",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/171|669f51cd04e76463989f0dc45676025d20bed3ab",
      "payload_hash": "fc56268256fbe4963428c8bfc5c68f9d0a405c967ab8ff0a6933e351e68689c0",
      "duplicate": false,
      "received_at": "2026-07-02T19:37:30.296729887Z"
    },
    {
      "id": 141,
      "run_id": "1f9952f858e3",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "509b2420-764d-11f1-88f5-56940a863d7c",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/landmark/pull/190|179ace024f80b2f0edf9090c158c833fcbcc660a",
      "payload_hash": "eee980168014ea259f7b8daea3cbb7bb55e40ac7adff00b2408198603fa8418e",
      "duplicate": false,
      "received_at": "2026-07-02T19:36:27.275408649Z"
    },
    {
      "id": 140,
      "run_id": "d35af67fd726",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "cc408ad0-764c-11f1-9d1a-316e5a62000e",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/940|e9f8f6e194caa095b1d362b90d87b612fb67ab50",
      "payload_hash": "fde22d5556888b770a0b57c32dccd7c7833e0062c1eb1f3427b3ea4c3bd3867e",
      "duplicate": false,
      "received_at": "2026-07-02T19:32:45.318472447Z"
    },
    {
      "id": 139,
      "run_id": "0a2fce61171b",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "c7ab9630-7648-11f1-981b-f2ff76054af8",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/35|3e2886073fd2e13ac039a3a99b72dc5a4c4a39ea",
      "payload_hash": "86bfa6a38ca89ed1287d75605a70f46d507fe4fb56726aba999f4fea826d3b87",
      "duplicate": false,
      "received_at": "2026-07-02T19:03:59.519403435Z"
    },
    {
      "id": 138,
      "run_id": "a77a70573ac2",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "f09aabe0-7647-11f1-91f4-017a01505841",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/34|5fa7567e6813f5929f6512408541678e2b1bec66",
      "payload_hash": "04e0becfb004e58826aa617af3218b337066519b0275bbfedc7286b5ba881647",
      "duplicate": false,
      "received_at": "2026-07-02T18:57:58.710795265Z"
    },
    {
      "id": 137,
      "run_id": "3e8f5463cc94",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "9444e4c0-7645-11f1-9e03-6892221e866d",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/34|d799138d76f630109d6f1bfc30d1eff00ab0c09d",
      "payload_hash": "b1641242b9fb96ec6051233905cae547f97af7fb859ff55fd89c34b1e3042a40",
      "duplicate": false,
      "received_at": "2026-07-02T18:41:04.760868693Z"
    },
    {
      "id": 136,
      "run_id": "358a56e0c4ac",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "6d2e7190-7644-11f1-8519-703357b09fc7",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/939|d0c50fbd28f1a8ccaab9cfa2a32a59447f647a56",
      "payload_hash": "7e42ce5d8357e7e4afd5672970ab177d8b69424b57951fd37208d81e290a3e4b",
      "duplicate": false,
      "received_at": "2026-07-02T18:32:49.750424879Z"
    },
    {
      "id": 135,
      "run_id": "48b934b545ed",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "2ab37880-7642-11f1-8b94-59350993c937",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/33|a56d7abdd398a1c51db62c9112ded211982b855d",
      "payload_hash": "1a60607edbc53d185306119bfc659b9b8cd8af2a5992bfb20886594c7619cbf6",
      "duplicate": false,
      "received_at": "2026-07-02T18:16:39.174581201Z"
    },
    {
      "id": 134,
      "run_id": "1b18f92bfe9a",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "aaa72ab0-7641-11f1-8032-ba928da890a3",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/33|992aff3d6b44686dac66c5f99a1edbafa13d365b",
      "payload_hash": "417f2efc1442b817ccfd850d642b1b0ecf28cf8547b28022545c1d1a7b6bd417",
      "duplicate": false,
      "received_at": "2026-07-02T18:13:04.673743914Z"
    },
    {
      "id": 133,
      "run_id": "eff45ee21f95",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "bdc493f0-763f-11f1-8964-02eec59e9692",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/weave/pull/20|d4436c79c7b0e6389c5a34cecea799c672c105bf",
      "payload_hash": "badf6c53d4be9c29a46e80a2e282750cfa22780c23eb10541f14ed53d4e2d37b",
      "duplicate": false,
      "received_at": "2026-07-02T17:59:17.351285066Z"
    },
    {
      "id": 132,
      "run_id": "987e960ec0f5",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "210f6b50-763c-11f1-97d5-212a09c4c883",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/938|7d552c59d3469e13fecdb87b8e244475304135ff",
      "payload_hash": "66cf9122bcb1a6dfd27dd595d164a8fa1dee261242279cf410e577fb95448679",
      "duplicate": false,
      "received_at": "2026-07-02T17:33:26.205128251Z"
    },
    {
      "id": 131,
      "run_id": "343a2c1bf51b",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "6a9d5580-763b-11f1-952f-454f358bbc7d",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/31|2c87fa76564bdf5ab8d88c4ce9162e178b3aacac",
      "payload_hash": "d5069aef8a31e90d0a39be3383585de0e4b53c54ae9bd575cdb93360039e5985",
      "duplicate": false,
      "received_at": "2026-07-02T17:31:01.676671007Z"
    },
    {
      "id": 130,
      "run_id": "12eb41792f97",
      "task": "review",
      "trigger_kind": "manual",
      "source_event_id": null,
      "dedupe_key": null,
      "payload_hash": "2a819cde6cb1b83739462d8ce2cfa85cb6441e95fff3e9c31f4d3be88182c19f",
      "duplicate": false,
      "received_at": "2026-07-02T17:25:29.505888899Z"
    },
    {
      "id": 129,
      "run_id": "7e771eed0896",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "d3d33110-763a-11f1-89ef-ce0abbba2ce3",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/937|6e37aa41aa28f4f3dbaab62ec2bba7055996ae09",
      "payload_hash": "6fada402a924ec51ca4f7fc07864a074c40228e4b66353bcc9c919b1dd5f726a",
      "duplicate": false,
      "received_at": "2026-07-02T17:24:06.918703975Z"
    },
    {
      "id": 128,
      "run_id": "b9e0734d7343",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "499326f0-7639-11f1-9ac1-9fac2ce53f89",
      "dedupe_key": "wh:review-fleet:https://github.com/misty-step/powder/pull/31|4161046c2c31f23fbdf16ebe0171d9789470edee",
      "payload_hash": "c4d238f47249795d4137353bc98ec60488f1800aeb3a8ecdca770040ab643a36",
      "duplicate": false,
      "received_at": "2026-07-02T17:13:05.520666046Z"
    },
    {
      "id": 127,
      "run_id": "a218d8f1d928",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "0c93e060-7638-11f1-8d59-521541892295",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/936|6bd80262f6f047a5135805d3ec21c307d58422f3",
      "payload_hash": "520e63eefacd13b58e9b6a52ded26ad704e6c282d278438a43b4c17e6bdfa073",
      "duplicate": false,
      "received_at": "2026-07-02T17:04:13.655054442Z"
    },
    {
      "id": 126,
      "run_id": "0c803861cc73",
      "task": "model-catalog-watch",
      "trigger_kind": "cron",
      "source_event_id": null,
      "dedupe_key": "cron:2026-07-02T14:00:00+00:00",
      "payload_hash": null,
      "duplicate": false,
      "received_at": "2026-07-02T14:00:00.92315996Z"
    },
    {
      "id": 125,
      "run_id": "2e230792e695",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "ac9d1c00-75f9-11f1-82b6-161c23ecbd30",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/935|44844f4f63b7b7fa711b4a29ab696786f389dbef",
      "payload_hash": "c59bc299e815a08f42422ab23857757f41493a42b677e7ed07554e99fcc836f3",
      "duplicate": false,
      "received_at": "2026-07-02T09:37:44.107378069Z"
    },
    {
      "id": 124,
      "run_id": "252f5e9c7b21",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "1b1c3a50-75f8-11f1-86e8-d587cd9743ce",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/934|1eec414c0afd00bc6144a8abceb53c30b7f0e2d4",
      "payload_hash": "93d64a60550749319880bda2e7e0d19ea6725c3c7fceff753e49a90b054d3072",
      "duplicate": false,
      "received_at": "2026-07-02T09:26:31.687748284Z"
    },
    {
      "id": 123,
      "run_id": "d5c899a19ce3",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "88d5a990-75f4-11f1-970b-59d8cd9474a0",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/933|56352021f5a4998ac74f8b89c60a3e5c59749534",
      "payload_hash": "45bb0992a80367b28ebb48d4285fae99f0582de2f8db441bbae8c9f23a799725",
      "duplicate": false,
      "received_at": "2026-07-02T09:00:56.408178329Z"
    },
    {
      "id": 122,
      "run_id": "fb4a5e37c5aa",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "3bc58530-75ef-11f1-8bce-5bb95b6e92f2",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/932|a446a5fe51ea755cce5ba648d1d24dc3150dcd95",
      "payload_hash": "58a55cf69f2e9668117c47a9c63ee12b5906a760d5d3f8a2b55b58e74763e8b1",
      "duplicate": false,
      "received_at": "2026-07-02T08:22:59.775047084Z"
    },
    {
      "id": 121,
      "run_id": "e9126dc52ef7",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "40c19250-75ee-11f1-8b8a-42c657e4ece9",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/931|a7851a85e64204635e0ef0963f2f72fbad3d8b95",
      "payload_hash": "43146fd6f1480e7fad4e1b6712af51f79230fc689acbaff151f52e01f073895f",
      "duplicate": false,
      "received_at": "2026-07-02T08:15:58.464814673Z"
    },
    {
      "id": 120,
      "run_id": "1f4d6d1c5f17",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "9a1009b0-75ec-11f1-9fbc-6778bb9ba1da",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/930|587045158254de6693f5e0b753462adc6148268f",
      "payload_hash": "a93f1198ae563a0c984a99cc6ef8665168fc19bc2da2e77fa70f668f8ec23dc3",
      "duplicate": false,
      "received_at": "2026-07-02T08:04:09.446822383Z"
    },
    {
      "id": 119,
      "run_id": "f2b59076b4a9",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "aba72ba0-75eb-11f1-8804-15abe1dccefc",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/929|344880fbe6f3b6ee452dd84642b65e773e5689f7",
      "payload_hash": "406a87384e90a08de4c8e608233f4a77a44f25eaf87531a9cfebe0556e211961",
      "duplicate": false,
      "received_at": "2026-07-02T07:57:29.486077584Z"
    },
    {
      "id": 118,
      "run_id": "0a42ed01208b",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "cae82fc0-75e9-11f1-9e9a-28ce4f0999f8",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/928|051e85b88e6a1513c703ec94ed821350a334217d",
      "payload_hash": "0a99b0ef5d6e596f6b714c34644c80a9fcd5a380490455a19895ffed894f57bb",
      "duplicate": false,
      "received_at": "2026-07-02T07:44:02.834354963Z"
    },
    {
      "id": 117,
      "run_id": "a3ffcb8e37e0",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "ada3c650-75e8-11f1-856c-4872581499c7",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/927|041fef9f380b0ad3a0a4dc6826c7db7931999716",
      "payload_hash": "344afd906b94ba88ae9422718224aeb3fd2e414df5174b9fd6d8902ebee27546",
      "duplicate": false,
      "received_at": "2026-07-02T07:36:10.173902573Z"
    },
    {
      "id": 116,
      "run_id": "c8f5cd440828",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "a475ba30-75e7-11f1-966e-2bdb878f5357",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/926|3efde42ba50313b63724a1b60bd5590d0de4e29c",
      "payload_hash": "17d1478fc79d6c58a138f544db86e0a40789416788bc639429aa247cdcbf982d",
      "duplicate": false,
      "received_at": "2026-07-02T07:28:39.304040322Z"
    },
    {
      "id": 115,
      "run_id": "9451667fdd60",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "54b3f870-75e4-11f1-9abb-8eb36536b562",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/925|e45b839cde3d2b116e67f2d4c205bac3843181f5",
      "payload_hash": "c152c0d17bdb0c415eac06a1014f7ff36e0d97e46c3665362758960adb65ab66",
      "duplicate": false,
      "received_at": "2026-07-02T07:04:57.04289324Z"
    },
    {
      "id": 114,
      "run_id": "9402e80ac51d",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "d922a0e0-75e2-11f1-8050-d8e712059a97",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/924|186341b101771455b0dccb82a95a4a4f9e2bbcef",
      "payload_hash": "e80bc7418f9eb37961914113f337fbe0043e435fbf9cdd74cfeb19ee3b519869",
      "duplicate": false,
      "received_at": "2026-07-02T06:54:20.171193125Z"
    },
    {
      "id": 113,
      "run_id": "c813d43a33ce",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "4577a8b0-75e0-11f1-98e3-b87d20dff969",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/923|c6c9268df42b025afd216ec85b8d6107b3bda508",
      "payload_hash": "d8dae5883e676e0df0da5dfb76b194f56be462b45877e72c6258916a14ddd696",
      "duplicate": false,
      "received_at": "2026-07-02T06:35:53.497143696Z"
    },
    {
      "id": 112,
      "run_id": "5959d446e1e8",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "8db32240-75df-11f1-9098-d05ee8055064",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/922|ec9a438e1a78c16807cca7a0c0661e94ae6126a4",
      "payload_hash": "1faf5d6d0e3fded1176a845092e1a878744beac21a536b9fffcb7e3cf3b609e3",
      "duplicate": false,
      "received_at": "2026-07-02T06:30:45.117348158Z"
    },
    {
      "id": 111,
      "run_id": "82d9c1e37404",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "72936300-75dd-11f1-8d85-4f59255b62d6",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/921|cb632c5254e11564f94d768ca108fbdc5e5a5e27",
      "payload_hash": "33d44a7d7ff864c624960efac6f3bf7ee793e862712b13fbf24b3c3ac21a49f3",
      "duplicate": false,
      "received_at": "2026-07-02T06:15:40.548544338Z"
    },
    {
      "id": 110,
      "run_id": "dbc6c8f14b18",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "eeb409a0-75db-11f1-93e4-89f66d9f25c2",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/920|c6f7bcd04fd3c722d2cc0e90d16460813f7b2bca",
      "payload_hash": "199dd6c0cb870a37714696f1b52fd3b2f9337e46faa56f92fffeaaada47c9404",
      "duplicate": false,
      "received_at": "2026-07-02T06:04:49.869342448Z"
    },
    {
      "id": 109,
      "run_id": "54b42938cd02",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "3aac6420-75db-11f1-95ac-7ada801e782d",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/919|c6592466ba3e4fb0066545c3b3cb3078c6c8a42e",
      "payload_hash": "338e14ffc41672607bcdeecc923d7ce5d1dd6f3c5c5a8f7aa415bf39bf9b76fa",
      "duplicate": false,
      "received_at": "2026-07-02T05:59:47.950027783Z"
    },
    {
      "id": 108,
      "run_id": "49111d3a2618",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "44e1e010-75da-11f1-83f5-8a8fd9694a2a",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/918|7da2c470b2bbb24b053af69108e78048f528cdc8",
      "payload_hash": "ddc14598282afdf85159e5c67a13e8e6ce67c3a1807a0c0958d12fe701e5f7d1",
      "duplicate": false,
      "received_at": "2026-07-02T05:52:55.442945707Z"
    },
    {
      "id": 107,
      "run_id": "076dced2e2bd",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "e4896a80-75d9-11f1-8ae8-b3c57d87bd5f",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/917|6b8e3c6f214af13cdc89f35fb187d838b18bb4eb",
      "payload_hash": "ee11a4b244e2c653ea0c43871002dafea169652d4cb5ba162b4a13aa6cb8e4f2",
      "duplicate": false,
      "received_at": "2026-07-02T05:50:13.875893562Z"
    },
    {
      "id": 106,
      "run_id": "2de44089c5ba",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "0d69da30-75d9-11f1-9be8-8f5d85e88a4c",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/916|df05510348e721e49ef844c9e8fcb4d79bbb5ab0",
      "payload_hash": "31cf86ec9254fbe7240bd112c4c6001582b1eee188c1a44edd22e7b4286e8ced",
      "duplicate": false,
      "received_at": "2026-07-02T05:44:12.84345609Z"
    },
    {
      "id": 105,
      "run_id": "2e3013116b4c",
      "task": "canary-triage",
      "trigger_kind": "webhook",
      "source_event_id": "DLV-zj5royvznl01",
      "dedupe_key": "wh:canary-triage:DLV-zj5royvznl01",
      "payload_hash": "cde2fbdb20d5803690f4d9f24f5d306686863e6d969218286daa0221e737444a",
      "duplicate": false,
      "received_at": "2026-07-02T05:31:51.303375135Z"
    },
    {
      "id": 104,
      "run_id": "8a991eba91eb",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "46bc9ae0-75d7-11f1-83c7-084ec5df33ca",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/915|0d2dc0130004e22cdc80a7db0aa9c702e6d24944",
      "payload_hash": "b6614e9a846c08ed8bf96d2fd9a45e4bbeb5378dbc20b1e770bf29c03d21cdc5",
      "duplicate": false,
      "received_at": "2026-07-02T05:31:30.076285861Z"
    },
    {
      "id": 103,
      "run_id": "bb02acf5ddca",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "7a714210-75d6-11f1-8551-cfef5ca87329",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/914|9d5618270f5daecf4917609e01f99b2ecba0bc63",
      "payload_hash": "4814102eaa8820f47a9a0c61f47cfd083b3efebdeccc76c613d7f73822be1072",
      "duplicate": false,
      "received_at": "2026-07-02T05:25:47.368394096Z"
    },
    {
      "id": 102,
      "run_id": "dd5c98cdcbdf",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "f951fda0-75d5-11f1-8469-63421ee56f6b",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/913|d0bb0a84e568f09a7328262ff3b27bc4ace31347",
      "payload_hash": "b112d3b6633ee160b877054ef23a19665baf34af496698c7ab0c46eee0850880",
      "duplicate": false,
      "received_at": "2026-07-02T05:22:10.762554803Z"
    },
    {
      "id": 101,
      "run_id": "3168c75c2b6f",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "b98808e0-75d5-11f1-9e2f-4b17e376005b",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/913|55dde26eb1cac8adc949830ea9eec25b37083fcb",
      "payload_hash": "f6bc73baf7c99cde9caa8b6ad49b9293ce6881e0dc686657e9b2fb866436ab6d",
      "duplicate": false,
      "received_at": "2026-07-02T05:20:23.538514234Z"
    },
    {
      "id": 100,
      "run_id": "e389b7fdaba1",
      "task": "review",
      "trigger_kind": "webhook",
      "source_event_id": "2c703130-75d5-11f1-95f3-5911824b310c",
      "dedupe_key": "wh:review:https://github.com/misty-step/bitterblossom/pull/912|4f44c27fabc0fce64b968d58e8e13d7bce47f698",
      "payload_hash": "d9e0c84870c0698d32cfb9a028992e67df285fd9bf074e39447ed021e4bed81c",
      "duplicate": false,
      "received_at": "2026-07-02T05:16:27.014581161Z"
    }
  ]
};
