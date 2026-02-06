# Sentry Setup — Misty Step

> Last updated: 2026-02-05  
> Org: `misty-step` | Plan: Developer | 12 projects (all Next.js on Vercel)

---

## Table of Contents

1. [Organization Overview](#organization-overview)
2. [Project Inventory & Status](#project-inventory--status)
3. [Alert Rules Configuration](#alert-rules-configuration)
4. [Sentry API Reference](#sentry-api-reference)
5. [sentry-watcher.sh Script](#sentry-watchersh-script)
6. [Monitoring Strategy](#monitoring-strategy)
7. [Current Issues Summary](#current-issues-summary)

---

## Organization Overview

| Field | Value |
|-------|-------|
| **Org Slug** | `misty-step` |
| **Org ID** | `4510313803677696` |
| **Status** | Active |
| **Created** | 2025-11-05 |
| **Members** | 1 (phrazzld@pm.me — owner) |
| **Team** | `misty-step` (1 member) |
| **Platform** | All projects: `javascript-nextjs` |
| **CLI** | `sentry-cli 3.1.0` installed |

### Authentication

```bash
# Token stored in ~/.secrets as SENTRY_MASTER_TOKEN
source ~/.secrets
# Auth header: Authorization: Bearer $SENTRY_MASTER_TOKEN
```

---

## Project Inventory & Status

### Activity Classification (as of 2026-02-05)

| Project | ID | First Event | 90d Events | Sessions | Status |
|---------|-----|-------------|-----------|----------|--------|
| **scry** | 4510313841426432 | 2025-11-11 | 1,254 | ✅ | 🟢 **ACTIVE** |
| **gitpulse** | 4510403670638592 | 2026-01-11 | 808 | ✅ | 🟢 **ACTIVE** |
| **linejam** | 4510762050650112 | — | 135 | ✅ | 🟡 **SESSIONS ONLY** |
| **sploot** | 4510314419585024 | 2025-11-07 | 43 | ✅ | 🟢 **ACTIVE** |
| **volume** | 4510314421092352 | 2026-01-14 | 21 | ❌ | 🟡 **LOW** |
| **caesar-in-a-year** | 4510779979726849 | 2026-01-27 | 4 | ✅ | 🟡 **LOW** |
| **heartbeat** | 4510477306626048 | 2025-12-16 | 3 | ✅ | 🟡 **LOW** |
| **misty-step** | 4510314424172544 | 2025-11-07 | 2 | ✅ | 🟡 **LOW** |
| **chrondle** | 4510762044751872 | 2026-01-23 | 2 | ✅ | 🟡 **LOW** |
| **bibliomnomnom** | 4510757480759296 | 2026-01-23 | 1 | ✅ | 🟡 **LOW** |
| **conviction** | 4510779934900224 | — | 0 | ❌ | 🔴 **DORMANT** |
| **vanity** | 4510314423386112 | — | 0 | ❌ | 🔴 **DORMANT** |

**Notes:**
- **scry** is the busiest project by far (1,254 errors in 90 days)
- **gitpulse** has high error count but may include noise  
- **linejam** has sessions but no firstEvent — likely tracking sessions without errors
- **conviction** and **vanity** have never received an event — SDKs may not be configured
- **sploot** has a recurring PrismaClientKnownRequestError (12 occurrences, last: 2026-02-02)

---

## Alert Rules Configuration

### Rules Applied to ALL 12 Projects

Every project now has these 5 standardized alert rules:

| # | Rule Name | Trigger | Action | Frequency |
|---|-----------|---------|--------|-----------|
| 1 | **Send a notification for high priority issues** | Sentry marks new/existing issue as high priority | Email IssueOwners → ActiveMembers | 5 min |
| 2 | **New Error Alert** | A new issue is created | Email IssueOwners → ActiveMembers | 5 min |
| 3 | **Error Rate Spike** | Issue seen >10 times in 1 hour | Email IssueOwners → ActiveMembers | 60 min |
| 4 | **New Unhandled Exception** | New issue + `error.unhandled = true` | Email IssueOwners → ActiveMembers | 5 min |
| 5 | **Regression Alert** | Issue changes from resolved → unresolved | Email IssueOwners → ActiveMembers | 5 min |

### Additional Rules (Project-Specific)

**bibliomnomnom** has one extra rule:
- **Critical: Auth/Payment Error** — Fires on every event matching tags containing `stripe`, `clerk`, or `webhook`

### Email Delivery Note

All rules use `IssueOwners → ActiveMembers` fallthrough. Since the only org member is `phrazzld@pm.me`, all alerts go there. To add kaylee@mistystep.io:

```bash
# Option 1: Add as org member
# Go to Settings → Members → Invite: kaylee@mistystep.io

# Option 2: Create rules targeting specific member (after adding)
# Use targetType: "Member" with targetIdentifier: <member_id>
```

**⚠️ kaylee@mistystep.io is NOT currently a Sentry org member.** Alerts currently route to phrazzld@pm.me. Add kaylee@mistystep.io as a member to receive direct email alerts.

---

## Sentry API Reference

### Authentication

```bash
# All requests require:
Authorization: Bearer $SENTRY_MASTER_TOKEN
```

### Base URL

```
https://sentry.io/api/0/
```

### Core Endpoints

#### Organization

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/organizations/misty-step/` | Org details |
| GET | `/organizations/misty-step/projects/` | List all projects |
| GET | `/organizations/misty-step/members/` | List org members |
| GET | `/organizations/misty-step/teams/` | List teams |
| GET | `/organizations/misty-step/stats_v2/?field=sum(quantity)&groupBy=project&category=error&interval=1d&statsPeriod=30d` | Org-wide error stats by project |

#### Projects

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects/misty-step/{slug}/` | Project details |
| GET | `/projects/misty-step/{slug}/stats/?stat=received&resolution=1d` | Event stats (daily) |
| GET | `/projects/misty-step/{slug}/stats/?stat=received&resolution=1h&since={epoch}` | Hourly stats since timestamp |

#### Issues

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects/misty-step/{slug}/issues/?query=is:unresolved` | Unresolved issues |
| GET | `/projects/misty-step/{slug}/issues/?query=is:unresolved+level:fatal` | Fatal unresolved issues |
| GET | `/projects/misty-step/{slug}/issues/?query=is:unresolved+!has:assignee` | Unassigned issues |
| GET | `/projects/misty-step/{slug}/issues/?query=is:unresolved&sort=freq` | By frequency |
| GET | `/projects/misty-step/{slug}/issues/?query=is:unresolved&sort=date` | By date |
| GET | `/projects/misty-step/{slug}/issues/?query=is:unresolved&sort=new` | By newest |
| GET | `/issues/{issue_id}/` | Single issue details |
| GET | `/issues/{issue_id}/events/` | Events for an issue |
| PUT | `/issues/{issue_id}/` | Update issue (resolve, assign, etc.) |

#### Alert Rules (Issue Alerts)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects/misty-step/{slug}/rules/` | List alert rules |
| POST | `/projects/misty-step/{slug}/rules/` | Create alert rule |
| GET | `/projects/misty-step/{slug}/rules/{rule_id}/` | Get single rule |
| PUT | `/projects/misty-step/{slug}/rules/{rule_id}/` | Update rule |
| DELETE | `/projects/misty-step/{slug}/rules/{rule_id}/` | Delete rule |
| GET | `/projects/misty-step/{slug}/rules/configuration/` | Available conditions, filters, actions |

#### Combined/Metric Alerts

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/organizations/misty-step/combined-rules/` | All alert rules across org |

#### Events & Tags

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/projects/misty-step/{slug}/events/` | Recent events |
| GET | `/projects/misty-step/{slug}/events/{event_id}/` | Event details |
| GET | `/projects/misty-step/{slug}/tags/` | Available tags |
| GET | `/projects/misty-step/{slug}/tags/{tag_key}/values/` | Tag values |

#### Releases

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/organizations/misty-step/releases/` | List releases |
| GET | `/organizations/misty-step/releases/{version}/` | Release details |
| GET | `/organizations/misty-step/releases/{version}/deploys/` | Deploys for release |

### Available Alert Rule Conditions

```
sentry.rules.conditions.first_seen_event.FirstSeenEventCondition
  → "A new issue is created"

sentry.rules.conditions.regression_event.RegressionEventCondition
  → "The issue changes state from resolved to unresolved"

sentry.rules.conditions.reappeared_event.ReappearedEventCondition
  → "The issue changes state from archived to escalating"

sentry.rules.conditions.high_priority_issue.NewHighPriorityIssueCondition
  → "Sentry marks a new issue as high priority"

sentry.rules.conditions.high_priority_issue.ExistingHighPriorityIssueCondition
  → "Sentry marks an existing issue as high priority"

sentry.rules.conditions.event_frequency.EventFrequencyCondition
  → "The issue is seen more than {value} times in {interval}"
  → intervals: 1m, 5m, 15m, 1h, 1d, 1w, 30d

sentry.rules.conditions.event_frequency.EventUniqueUserFrequencyCondition
  → "The issue is seen by more than {value} users in {interval}"

sentry.rules.conditions.event_frequency.EventFrequencyPercentCondition
  → "The issue affects more than {value} percent of sessions in {interval}"
  → intervals: 5m, 10m, 30m, 1h
```

### Available Alert Filters

```
AgeComparisonFilter     → Issue is older/newer than X minutes/hours/days/weeks
IssueOccurrencesFilter  → Issue has happened at least X times
AssignedToFilter        → Issue is assigned to Unassigned/Team/Member
LatestReleaseFilter     → Event is from the latest release
IssueCategoryFilter     → Category: error/performance/profile/cron/replay/feedback/uptime/etc.
EventAttributeFilter    → Match on: message, platform, environment, type,
                          error.handled, error.unhandled, exception.type,
                          exception.value, user.*, http.*, sdk.name, stacktrace.*
TaggedEventFilter       → Match event tags (key/value with co/eq/sw/ew/etc.)
LevelFilter             → Event level is (eq/gte/lte) fatal/error/warning/info/debug
```

### Available Alert Actions

```
NotifyEmailAction           → Email to IssueOwners/Team/Member (with fallthrough)
NotifyEventAction           → Legacy integration notification
NotifyEventServiceAction    → Service notification (GitHub Auto-Triage available)
DiscordNotifyServiceAction  → Discord (not connected)
SlackNotifyServiceAction    → Slack (not connected)
PagerDutyNotifyServiceAction → PagerDuty (not connected)
OpsgenieNotifyTeamAction    → Opsgenie (not connected)
```

### Example: Create Alert Rule via API

```bash
curl -X POST -H "Authorization: Bearer $SENTRY_MASTER_TOKEN" \
  -H "Content-Type: application/json" \
  "https://sentry.io/api/0/projects/misty-step/{slug}/rules/" \
  -d '{
    "name": "My Alert Rule",
    "actionMatch": "all",
    "filterMatch": "all",
    "conditions": [
      {"id": "sentry.rules.conditions.first_seen_event.FirstSeenEventCondition"}
    ],
    "filters": [
      {
        "id": "sentry.rules.filters.event_attribute.EventAttributeFilter",
        "attribute": "error.unhandled",
        "match": "eq",
        "value": "true"
      }
    ],
    "actions": [
      {
        "id": "sentry.mail.actions.NotifyEmailAction",
        "targetType": "IssueOwners",
        "targetIdentifier": "",
        "fallthroughType": "ActiveMembers"
      }
    ],
    "frequency": 5
  }'
```

### Rate Limits

- **40 requests per window** (observed)
- Headers: `x-sentry-rate-limit-remaining`, `x-sentry-rate-limit-limit`, `x-sentry-rate-limit-reset`

---

## sentry-watcher.sh Script

**Location:** `~/bitterblossom/scripts/sentry-watcher.sh`

### What It Does

1. Sources `SENTRY_MASTER_TOKEN` from `~/.secrets`
2. Fetches all projects from `misty-step` org
3. For each project, retrieves:
   - All unresolved issues
   - 24-hour event statistics
4. Compares issue IDs against previous run (state in `/tmp/sentry-state.json`)
5. Detects anomalies:
   - **New issues** not seen in previous check
   - **High frequency** issues (>10 events)
   - **Unhandled exceptions**
   - **Resolved issues** (disappeared since last check)
6. Outputs JSON report to stdout
7. Saves state for next comparison

### Usage

```bash
# Basic run
~/bitterblossom/scripts/sentry-watcher.sh

# Quiet mode (no stderr logs)
~/bitterblossom/scripts/sentry-watcher.sh --quiet

# Custom state file
~/bitterblossom/scripts/sentry-watcher.sh --state-file /path/to/state.json

# Save report to file
~/bitterblossom/scripts/sentry-watcher.sh --quiet > /tmp/sentry-report.json

# Cron: every 30 minutes
*/30 * * * * /Users/yawgmoth/bitterblossom/scripts/sentry-watcher.sh --quiet > /tmp/sentry-report-$(date +\%Y\%m\%d-\%H\%M).json 2>/dev/null
```

### Output Format

```json
{
  "report": {
    "timestamp": "2026-02-06T02:06:51Z",
    "previous_check": "1970-01-01T00:00:00Z",
    "org": "misty-step",
    "projects_scanned": 12,
    "total_unresolved_issues": 15,
    "new_issues_since_last_check": 0,
    "total_events_24h": 2,
    "anomaly_count": 6
  },
  "projects": [
    {
      "project": "scry",
      "unresolved_issues": 5,
      "new_since_last_check": 0,
      "events_24h": 0,
      "unhandled": 2,
      "high_frequency": 0
    }
  ],
  "anomalies": [
    {
      "project": "sploot",
      "type": "high_frequency",
      "count": 1,
      "issues": [{"id": "7117400497", "title": "PrismaClientKnownRequestError", "count": 12}]
    }
  ]
}
```

---

## Monitoring Strategy

### Current State

| Layer | Status | Notes |
|-------|--------|-------|
| **Sentry Issue Alerts** | ✅ Complete | 5 rules on all 12 projects |
| **sentry-watcher.sh** | ✅ Ready | Polls API, compares state, outputs JSON |
| **Email Alerts** | ⚠️ Partial | Goes to phrazzld@pm.me only (add kaylee@mistystep.io as member) |
| **Metric Alerts** | ❌ Not configured | Sentry metric alerts need Business plan or higher |
| **Integrations** | ❌ None | Discord, Slack, PagerDuty all disconnected |

### Recommendations

1. **Add kaylee@mistystep.io as Sentry org member** — Required for direct email alerts
2. **Connect Discord or Slack** — For real-time team notification
3. **Set up a cron job** for `sentry-watcher.sh` — Every 15-30 min
4. **Address active issues:**
   - **sploot**: PrismaClientKnownRequestError recurring (12 events) — likely DB connection/schema issue
   - **scry**: 5 unresolved issues including "ArrowDown is not defined" (keyboard event handler bug)
   - **misty-step**: 2 EPIPE errors (server-side streaming/pipe broken)
5. **Investigate dormant projects**: conviction and vanity have never sent events — verify SDK setup

---

## Current Issues Summary (2026-02-05)

### 15 Unresolved Issues Across 7 Projects

| Project | Issue | Level | Count | Unhandled | Last Seen |
|---------|-------|-------|-------|-----------|-----------|
| **sploot** | PrismaClientKnownRequestError | error | 12 | No | 2026-02-02 |
| **sploot** | TypeError: Failed to fetch | error | 2 | ✅ Yes | 2026-01-12 |
| **scry** | ReferenceError: ArrowDown is not defined | error | 3 | No | 2026-01-14 |
| **scry** | Error: CONVEX Q Server Error | error | 1 | No | 2026-01-26 |
| **scry** | Error: Authentication required | error | 1 | No | 2026-01-18 |
| **scry** | Error: aborted | fatal | 1 | ✅ Yes | 2026-01-16 |
| **scry** | Error: write EPIPE | fatal | 1 | ✅ Yes | 2026-01-11 |
| **caesar-in-a-year** | Error: (empty) | error | 4 | ✅ Yes | 2026-01-27 |
| **misty-step** | Error: write EPIPE (×2 issues) | fatal | 1 ea. | ✅ Yes | 2026-01-17 |
| **gitpulse** | Error: write EPIPE | fatal | 1 | ✅ Yes | 2026-01-11 |
| **volume** | Error: No user found for Clerk ID | error | 1 | No | 2026-01-16 |
| **chrondle** | Test errors (3 issues) | error | 1 ea. | No | 2026-01-23 |

### Common Patterns

- **EPIPE errors** (4 issues across 3 projects): Server-side streaming connection broken. Common in Next.js SSR when client disconnects mid-response. Consider handling gracefully.
- **PrismaClientKnownRequestError** (sploot): Recurring DB issue — investigate connection pooling or schema constraints.
- **Test errors** (chrondle): Clean up — these are setup test events.
