//! Revisioned workflow configuration store (bitterblossom-workflow-store).
//!
//! The plane SQLite database is authoritative for active workflow
//! configuration. Every edit is an immutable revision; activation selects
//! one revision; rollback re-activates an earlier snapshot as a NEW
//! revision; accepted runs pin the revision active at acceptance and keep
//! it forever. CLI (`bb workflow ...`) and HTTP (`/api/workflows...`) both
//! call these store functions, so the two surfaces cannot drift.
//!
//! Declarative workflow documents (TOML) are import/export interchange:
//! importing a document identical to the latest stored revision is a no-op,
//! which is what keeps files from becoming a second live authority.
//!
//! Mechanism only: the store validates document *structure* (names, route
//! targets, trigger shapes) and owns lifecycle/revision/pinning arithmetic.
//! What a workflow's goal means, and what its agents do, stays outside the
//! spine.

use std::collections::BTreeMap;

use anyhow::{bail, Context, Result};
use rusqlite::{params, OptionalExtension};
use serde::{Deserialize, Serialize};

use crate::ledger::{new_id, now, Ledger};
use crate::spec::{Task, TriggerSpec};

pub const WORKFLOW_STATES: &[&str] = &["draft", "active", "paused", "archived"];
pub const TRIGGER_KINDS: &[&str] = &["manual", "cron", "webhook", "internal", "test", "replay"];
/// Route target meaning "this step completes the workflow".
pub const ROUTE_DONE: &str = "done";

fn lifecycle_allowed(from: &str, to: &str) -> bool {
    matches!(
        (from, to),
        ("draft", "active")
            | ("active", "active")
            | ("active", "paused")
            | ("paused", "active")
            | ("draft", "archived")
            | ("active", "archived")
            | ("paused", "archived")
    )
}

/// The declarative workflow document. Stored canonically as JSON in
/// `workflow_revisions.document`; TOML is the file interchange shape.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkflowDoc {
    pub name: String,
    pub goal: String,
    #[serde(default, rename = "trigger", skip_serializing_if = "Vec::is_empty")]
    pub triggers: Vec<WorkflowTrigger>,
    #[serde(rename = "step")]
    pub steps: Vec<WorkflowStep>,
    #[serde(default, skip_serializing_if = "WorkflowPolicies::is_empty")]
    pub policies: WorkflowPolicies,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkflowTrigger {
    pub kind: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub schedule: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub route: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub secret_env: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub dedupe_key: Option<String>,
}

/// One step commissions one pinned agent with a natural-language goal.
/// The agent binding is a materialized snapshot (name, version, harness,
/// model), not a reference to mutable config: a revision must stay
/// launch-meaningful even after agent files change or disappear.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkflowStep {
    pub name: String,
    pub agent: StepAgent,
    pub goal: String,
    /// Outcome -> next step name, or "done". Empty means: successful
    /// completion of this terminal step implies `completed`.
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub routes: BTreeMap<String, String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct StepAgent {
    pub name: String,
    pub version: u32,
    pub harness: String,
    pub model: String,
}

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct WorkflowPolicies {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub timeout_minutes: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_runs_per_day: Option<u32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub max_cost_per_run_usd: Option<f64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub concurrency: Option<u32>,
}

impl WorkflowPolicies {
    fn is_empty(&self) -> bool {
        self == &Self::default()
    }
}

impl WorkflowDoc {
    /// Structural validation only — no workload judgment. Rejects malformed
    /// names, empty goals, unknown trigger kinds, unparseable cron schedules,
    /// duplicate step names, and routes to unknown steps.
    pub fn validate(&self) -> Result<()> {
        // No '/': every name-scoped HTTP route addresses the workflow as one
        // URL path segment (tiny_http does no percent-decoding), so a name
        // containing '/' would be stored but unaddressable over HTTP.
        let name_ok = !self.name.is_empty()
            && self
                .name
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, '-' | '_' | '.'));
        if !name_ok {
            bail!(
                "workflow name {:?} must be non-empty [A-Za-z0-9._-] (one URL path segment)",
                self.name
            );
        }
        if self.goal.trim().is_empty() {
            bail!("workflow '{}': goal is required", self.name);
        }
        for trigger in &self.triggers {
            match trigger.kind.as_str() {
                "manual" | "internal" | "test" | "replay" => {}
                "cron" => {
                    let schedule = trigger.schedule.as_deref().unwrap_or("");
                    crate::ingress::parse_schedule(schedule)
                        .with_context(|| format!("workflow '{}': bad cron trigger", self.name))?;
                }
                "webhook" => {
                    if trigger.route.as_deref().unwrap_or("").trim().is_empty()
                        || trigger
                            .secret_env
                            .as_deref()
                            .unwrap_or("")
                            .trim()
                            .is_empty()
                    {
                        bail!(
                            "workflow '{}': webhook trigger requires route and secret_env",
                            self.name
                        );
                    }
                }
                other => bail!(
                    "workflow '{}': trigger kind '{other}' is unknown (known: {})",
                    self.name,
                    TRIGGER_KINDS.join(", ")
                ),
            }
        }
        if self.steps.is_empty() {
            bail!("workflow '{}': at least one step is required", self.name);
        }
        let mut names = std::collections::BTreeSet::new();
        for step in &self.steps {
            if step.name.trim().is_empty() {
                bail!("workflow '{}': step name is required", self.name);
            }
            if !names.insert(step.name.clone()) {
                bail!(
                    "workflow '{}': step '{}' declared more than once",
                    self.name,
                    step.name
                );
            }
            if step.goal.trim().is_empty() {
                bail!(
                    "workflow '{}': step '{}' goal is required",
                    self.name,
                    step.name
                );
            }
            if step.agent.name.trim().is_empty()
                || step.agent.harness.trim().is_empty()
                || step.agent.version == 0
            {
                bail!(
                    "workflow '{}': step '{}' agent needs name, version >= 1, harness",
                    self.name,
                    step.name
                );
            }
        }
        for step in &self.steps {
            for (outcome, target) in &step.routes {
                if outcome.trim().is_empty() {
                    bail!(
                        "workflow '{}': step '{}' has an empty route outcome",
                        self.name,
                        step.name
                    );
                }
                if target != ROUTE_DONE && !names.contains(target) {
                    bail!(
                        "workflow '{}': step '{}' routes '{outcome}' to unknown step '{target}'",
                        self.name,
                        step.name
                    );
                }
            }
        }
        Ok(())
    }

    /// The stored, diffed, and compared shape: pretty JSON with
    /// deterministic field order (struct order + BTreeMap routes).
    pub fn canonical_json(&self) -> Result<String> {
        Ok(serde_json::to_string_pretty(self)?)
    }

    pub fn from_canonical_json(text: &str) -> Result<Self> {
        let doc: Self = serde_json::from_str(text).context("parse stored workflow document")?;
        Ok(doc)
    }

    /// The declarative interchange shape (files, GitOps). Semantic
    /// round-trip: `from_toml(to_toml(doc)) == doc`.
    pub fn to_toml(&self) -> Result<String> {
        Ok(toml::to_string_pretty(self)?)
    }

    pub fn from_toml(text: &str) -> Result<Self> {
        let doc: Self = toml::from_str(text).context("parse workflow document TOML")?;
        doc.validate()?;
        Ok(doc)
    }

    /// Migration source: convert one loaded file-defined task into a
    /// workflow document. Files stay import material — nothing here writes
    /// back to task/agent files.
    pub fn from_task(task: &Task) -> Result<Self> {
        let triggers = task
            .spec
            .triggers
            .iter()
            .map(|t| match t {
                TriggerSpec::Manual => WorkflowTrigger {
                    kind: "manual".into(),
                    schedule: None,
                    route: None,
                    secret_env: None,
                    dedupe_key: None,
                },
                TriggerSpec::Cron { schedule } => WorkflowTrigger {
                    kind: "cron".into(),
                    schedule: Some(schedule.clone()),
                    route: None,
                    secret_env: None,
                    dedupe_key: None,
                },
                TriggerSpec::Webhook {
                    route,
                    secret_env,
                    dedupe_key,
                    ..
                } => WorkflowTrigger {
                    kind: "webhook".into(),
                    schedule: None,
                    route: Some(route.clone()),
                    secret_env: Some(secret_env.clone()),
                    dedupe_key: dedupe_key.clone(),
                },
            })
            .collect();
        let doc = Self {
            name: task.name.clone(),
            goal: task.card.trim().to_string(),
            triggers,
            steps: vec![WorkflowStep {
                name: "execute".into(),
                agent: StepAgent {
                    name: task.agent_name.clone(),
                    version: task.agent.version,
                    harness: task.agent.harness.clone(),
                    model: task.agent.model.clone(),
                },
                goal: task.card.trim().to_string(),
                routes: BTreeMap::new(),
            }],
            policies: WorkflowPolicies {
                timeout_minutes: task.spec.budget.timeout_minutes,
                max_runs_per_day: task.spec.budget.max_runs_per_day,
                max_cost_per_run_usd: task.spec.budget.max_cost_per_run_usd,
                concurrency: None,
            },
        };
        doc.validate()?;
        Ok(doc)
    }
}

#[derive(Debug, Clone, Serialize)]
pub struct WorkflowRow {
    pub id: String,
    pub name: String,
    pub state: String,
    pub active_revision: Option<i64>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Serialize)]
pub struct WorkflowRevisionRow {
    pub workflow_id: String,
    pub revision: i64,
    pub document: String,
    pub source: String,
    pub note: Option<String>,
    pub created_at: String,
}

#[derive(Debug, Serialize)]
pub struct WorkflowEventRow {
    pub id: i64,
    pub workflow_id: String,
    pub kind: String,
    pub data: Option<String>,
    pub at: String,
}

#[derive(Debug, Serialize)]
pub struct WorkflowRunRow {
    pub id: String,
    pub workflow_id: String,
    pub workflow: String,
    pub revision: i64,
    pub trigger_kind: String,
    pub payload: Option<String>,
    pub created_at: String,
}

#[derive(Debug, Serialize)]
#[serde(tag = "disposition", rename_all = "snake_case")]
pub enum AcceptOutcome {
    Accepted { run: WorkflowRunRow },
    Suppressed { workflow: String, reason: String },
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ImportOutcome {
    Created,
    Revised,
    Unchanged,
}

const WORKFLOW_SELECT: &str =
    "SELECT id, name, state, active_revision, created_at, updated_at FROM workflows";
const REVISION_SELECT: &str =
    "SELECT workflow_id, revision, document, source, note, created_at FROM workflow_revisions";
const WORKFLOW_RUN_SELECT: &str = "SELECT r.id, r.workflow_id, w.name, r.revision, \
     r.trigger_kind, r.payload, r.created_at \
     FROM workflow_runs r JOIN workflows w ON w.id = r.workflow_id";

fn row_to_workflow(r: &rusqlite::Row<'_>) -> rusqlite::Result<WorkflowRow> {
    Ok(WorkflowRow {
        id: r.get(0)?,
        name: r.get(1)?,
        state: r.get(2)?,
        active_revision: r.get(3)?,
        created_at: r.get(4)?,
        updated_at: r.get(5)?,
    })
}

fn row_to_revision(r: &rusqlite::Row<'_>) -> rusqlite::Result<WorkflowRevisionRow> {
    Ok(WorkflowRevisionRow {
        workflow_id: r.get(0)?,
        revision: r.get(1)?,
        document: r.get(2)?,
        source: r.get(3)?,
        note: r.get(4)?,
        created_at: r.get(5)?,
    })
}

fn row_to_workflow_run(r: &rusqlite::Row<'_>) -> rusqlite::Result<WorkflowRunRow> {
    Ok(WorkflowRunRow {
        id: r.get(0)?,
        workflow_id: r.get(1)?,
        workflow: r.get(2)?,
        revision: r.get(3)?,
        trigger_kind: r.get(4)?,
        payload: r.get(5)?,
        created_at: r.get(6)?,
    })
}

impl Ledger {
    /// Run `body` inside one immediate (write-locking) transaction so
    /// concurrent revisions/activations serialize instead of interleaving.
    fn workflow_tx<T>(&self, body: impl FnOnce() -> Result<T>) -> Result<T> {
        self.conn.execute_batch("BEGIN IMMEDIATE")?;
        match body() {
            Ok(value) => {
                self.conn.execute_batch("COMMIT")?;
                Ok(value)
            }
            Err(err) => {
                let _ = self.conn.execute_batch("ROLLBACK");
                Err(err)
            }
        }
    }

    fn workflow_audit(&self, workflow_id: &str, kind: &str, data: Option<&str>) -> Result<()> {
        self.conn.execute(
            "INSERT INTO workflow_events (workflow_id, kind, data, at) VALUES (?1, ?2, ?3, ?4)",
            params![workflow_id, kind, data, now()],
        )?;
        Ok(())
    }

    fn insert_revision(
        &self,
        workflow_id: &str,
        document: &str,
        source: &str,
        note: Option<&str>,
    ) -> Result<i64> {
        let next: i64 = self.conn.query_row(
            "SELECT COALESCE(MAX(revision), 0) + 1 FROM workflow_revisions WHERE workflow_id = ?1",
            params![workflow_id],
            |r| r.get(0),
        )?;
        self.conn.execute(
            "INSERT INTO workflow_revisions (workflow_id, revision, document, source, note, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
            params![workflow_id, next, document, source, note, now()],
        )?;
        Ok(next)
    }

    pub fn create_workflow(
        &self,
        doc: &WorkflowDoc,
        source: &str,
        note: Option<&str>,
    ) -> Result<(WorkflowRow, i64)> {
        doc.validate()?;
        let document = doc.canonical_json()?;
        self.workflow_tx(|| {
            if self.workflow_by_name_opt(&doc.name)?.is_some() {
                bail!(
                    "workflow '{}' already exists; use revise or import",
                    doc.name
                );
            }
            let id = format!("wf-{}", new_id());
            let ts = now();
            self.conn.execute(
                "INSERT INTO workflows (id, name, state, active_revision, created_at, updated_at)
                 VALUES (?1, ?2, 'draft', NULL, ?3, ?3)",
                params![id, doc.name, ts],
            )?;
            let revision = self.insert_revision(&id, &document, source, note)?;
            self.workflow_audit(&id, "created", Some(&format!("revision {revision}")))?;
            Ok((self.workflow(&id)?, revision))
        })
    }

    /// Append a new immutable revision. Refuses a document identical to the
    /// latest revision so "revise" always means an actual change.
    pub fn revise_workflow(
        &self,
        name: &str,
        doc: &WorkflowDoc,
        source: &str,
        note: Option<&str>,
    ) -> Result<(WorkflowRow, i64)> {
        doc.validate()?;
        if doc.name != name {
            bail!(
                "document names workflow '{}' but revises '{name}'; renames are a new workflow",
                doc.name
            );
        }
        let document = doc.canonical_json()?;
        self.workflow_tx(|| {
            let wf = self.workflow_by_name(name)?;
            if wf.state == "archived" {
                bail!("workflow '{name}' is archived; revisions are frozen");
            }
            let latest = self.latest_workflow_revision(&wf.id)?;
            if latest.document == document {
                bail!(
                    "document is identical to revision {}; nothing to revise",
                    latest.revision
                );
            }
            let revision = self.insert_revision(&wf.id, &document, source, note)?;
            self.workflow_audit(&wf.id, "revised", Some(&format!("revision {revision}")))?;
            Ok((self.workflow(&wf.id)?, revision))
        })
    }

    /// Import a declarative document: create when the name is new, revise
    /// when it changed, and no-op when identical to the latest revision —
    /// the property that keeps files interchange, not a second authority.
    pub fn import_workflow(
        &self,
        doc: &WorkflowDoc,
        source: &str,
        note: Option<&str>,
    ) -> Result<(WorkflowRow, i64, ImportOutcome)> {
        doc.validate()?;
        let document = doc.canonical_json()?;
        self.workflow_tx(|| {
            let Some(wf) = self.workflow_by_name_opt(&doc.name)? else {
                let id = format!("wf-{}", new_id());
                let ts = now();
                self.conn.execute(
                    "INSERT INTO workflows (id, name, state, active_revision, created_at, updated_at)
                     VALUES (?1, ?2, 'draft', NULL, ?3, ?3)",
                    params![id, doc.name, ts],
                )?;
                let revision = self.insert_revision(&id, &document, source, note)?;
                self.workflow_audit(&id, "created", Some(&format!("revision {revision}")))?;
                return Ok((self.workflow(&id)?, revision, ImportOutcome::Created));
            };
            let latest = self.latest_workflow_revision(&wf.id)?;
            if latest.document == document {
                return Ok((wf, latest.revision, ImportOutcome::Unchanged));
            }
            if wf.state == "archived" {
                bail!("workflow '{}' is archived; revisions are frozen", doc.name);
            }
            let revision = self.insert_revision(&wf.id, &document, source, note)?;
            self.workflow_audit(&wf.id, "revised", Some(&format!("revision {revision}")))?;
            Ok((self.workflow(&wf.id)?, revision, ImportOutcome::Revised))
        })
    }

    /// Activate one revision (default: latest). New acceptances pin the new
    /// revision; existing runs keep the revision they pinned at acceptance.
    pub fn activate_workflow(&self, name: &str, revision: Option<i64>) -> Result<WorkflowRow> {
        self.workflow_tx(|| {
            let wf = self.workflow_by_name(name)?;
            if !lifecycle_allowed(&wf.state, "active") {
                bail!("workflow '{name}' is {}; cannot activate", wf.state);
            }
            let revision = match revision {
                Some(r) => {
                    self.workflow_revision_row(&wf.id, r)?;
                    r
                }
                None => self.latest_workflow_revision(&wf.id)?.revision,
            };
            self.conn.execute(
                "UPDATE workflows SET state = 'active', active_revision = ?2, updated_at = ?3
                 WHERE id = ?1",
                params![wf.id, revision, now()],
            )?;
            self.workflow_audit(&wf.id, "activated", Some(&format!("revision {revision}")))?;
            self.workflow(&wf.id)
        })
    }

    /// Pause suppresses new run acceptance; active work elsewhere finishes.
    pub fn pause_workflow(&self, name: &str, reason: &str) -> Result<WorkflowRow> {
        self.workflow_lifecycle(name, "paused", "paused", Some(reason))
    }

    /// Resume re-enables acceptance on the already-active revision. It never
    /// replays events suppressed while paused; those stay audit dispositions.
    pub fn resume_workflow(&self, name: &str) -> Result<WorkflowRow> {
        self.workflow_lifecycle(name, "active", "resumed", None)
    }

    /// Archived workflows are frozen, never deleted: historical runs keep
    /// their revision referents readable forever.
    pub fn archive_workflow(&self, name: &str) -> Result<WorkflowRow> {
        self.workflow_lifecycle(name, "archived", "archived", None)
    }

    fn workflow_lifecycle(
        &self,
        name: &str,
        to: &str,
        audit_kind: &str,
        data: Option<&str>,
    ) -> Result<WorkflowRow> {
        self.workflow_tx(|| {
            let wf = self.workflow_by_name(name)?;
            let allowed = match (wf.state.as_str(), to) {
                // resume is paused -> active only; plain activate handles the rest.
                (from, "active") => from == "paused",
                (from, to) => lifecycle_allowed(from, to) && from != to,
            };
            if !allowed {
                bail!("workflow '{name}' is {}; cannot move to {to}", wf.state);
            }
            self.conn.execute(
                "UPDATE workflows SET state = ?2, updated_at = ?3 WHERE id = ?1",
                params![wf.id, to, now()],
            )?;
            self.workflow_audit(&wf.id, audit_kind, data)?;
            self.workflow(&wf.id)
        })
    }

    /// Rollback re-activates an earlier snapshot as a NEW revision — history
    /// is never rewritten. The workflow must already have an activation.
    pub fn rollback_workflow(&self, name: &str, to_revision: i64) -> Result<(WorkflowRow, i64)> {
        self.workflow_tx(|| {
            let wf = self.workflow_by_name(name)?;
            if !matches!(wf.state.as_str(), "active" | "paused") {
                bail!(
                    "workflow '{name}' is {}; rollback needs active or paused",
                    wf.state
                );
            }
            let snapshot = self.workflow_revision_row(&wf.id, to_revision)?;
            let revision = self.insert_revision(
                &wf.id,
                &snapshot.document,
                "rollback",
                Some(&format!("rollback to revision {to_revision}")),
            )?;
            self.conn.execute(
                "UPDATE workflows SET active_revision = ?2, updated_at = ?3 WHERE id = ?1",
                params![wf.id, revision, now()],
            )?;
            self.workflow_audit(
                &wf.id,
                "rolled_back",
                Some(&format!("revision {to_revision} -> {revision}")),
            )?;
            Ok((self.workflow(&wf.id)?, revision))
        })
    }

    /// Accept one triggering event: pin the revision active right now, in
    /// the same transaction that reads it, so a concurrent activation can
    /// never leave a run pinned to a revision that was not active at its
    /// acceptance. Paused workflows suppress acceptance and record the
    /// disposition; draft/archived refuse.
    pub fn accept_workflow_run(
        &self,
        name: &str,
        trigger_kind: &str,
        payload: Option<&str>,
    ) -> Result<AcceptOutcome> {
        if !TRIGGER_KINDS.contains(&trigger_kind) {
            bail!(
                "trigger kind '{trigger_kind}' is unknown (known: {})",
                TRIGGER_KINDS.join(", ")
            );
        }
        if let Some(payload) = payload {
            serde_json::from_str::<serde_json::Value>(payload).context("payload must be JSON")?;
        }
        self.workflow_tx(|| {
            let wf = self.workflow_by_name(name)?;
            match wf.state.as_str() {
                "active" => {}
                "paused" => {
                    let reason = format!("workflow paused; {trigger_kind} event suppressed");
                    self.workflow_audit(&wf.id, "event_suppressed", Some(&reason))?;
                    return Ok(AcceptOutcome::Suppressed {
                        workflow: wf.name,
                        reason,
                    });
                }
                other => bail!("workflow '{name}' is {other}; only active workflows accept runs"),
            }
            let revision = wf
                .active_revision
                .context("active workflow has no active revision (corrupt state)")?;
            let id = format!("wfr-{}", new_id());
            self.conn.execute(
                "INSERT INTO workflow_runs (id, workflow_id, revision, trigger_kind, payload, created_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                params![id, wf.id, revision, trigger_kind, payload, now()],
            )?;
            self.workflow_audit(
                &wf.id,
                "run_accepted",
                Some(&format!("run {id} pinned revision {revision}")),
            )?;
            Ok(AcceptOutcome::Accepted {
                run: self.workflow_run(&id)?,
            })
        })
    }

    pub fn workflow(&self, id: &str) -> Result<WorkflowRow> {
        self.conn
            .query_row(
                &format!("{WORKFLOW_SELECT} WHERE id = ?1"),
                params![id],
                row_to_workflow,
            )
            .with_context(|| format!("workflow {id} not found"))
    }

    fn workflow_by_name_opt(&self, name: &str) -> Result<Option<WorkflowRow>> {
        Ok(self
            .conn
            .query_row(
                &format!("{WORKFLOW_SELECT} WHERE name = ?1"),
                params![name],
                row_to_workflow,
            )
            .optional()?)
    }

    pub fn workflow_by_name(&self, name: &str) -> Result<WorkflowRow> {
        self.workflow_by_name_opt(name)?
            .with_context(|| format!("workflow '{name}' not found"))
    }

    pub fn list_workflows(&self) -> Result<Vec<WorkflowRow>> {
        let mut stmt = self
            .conn
            .prepare(&format!("{WORKFLOW_SELECT} ORDER BY name"))?;
        let rows = stmt
            .query_map([], row_to_workflow)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    fn workflow_revision_row(
        &self,
        workflow_id: &str,
        revision: i64,
    ) -> Result<WorkflowRevisionRow> {
        self.conn
            .query_row(
                &format!("{REVISION_SELECT} WHERE workflow_id = ?1 AND revision = ?2"),
                params![workflow_id, revision],
                row_to_revision,
            )
            .with_context(|| format!("revision {revision} not found"))
    }

    pub fn workflow_revision(&self, name: &str, revision: i64) -> Result<WorkflowRevisionRow> {
        let wf = self.workflow_by_name(name)?;
        self.workflow_revision_row(&wf.id, revision)
    }

    fn latest_workflow_revision(&self, workflow_id: &str) -> Result<WorkflowRevisionRow> {
        self.conn
            .query_row(
                &format!("{REVISION_SELECT} WHERE workflow_id = ?1 ORDER BY revision DESC LIMIT 1"),
                params![workflow_id],
                row_to_revision,
            )
            .context("workflow has no revisions (corrupt state)")
    }

    pub fn workflow_revisions(&self, name: &str) -> Result<Vec<WorkflowRevisionRow>> {
        let wf = self.workflow_by_name(name)?;
        let mut stmt = self.conn.prepare(&format!(
            "{REVISION_SELECT} WHERE workflow_id = ?1 ORDER BY revision"
        ))?;
        let rows = stmt
            .query_map(params![wf.id], row_to_revision)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn workflow_events(&self, name: &str) -> Result<Vec<WorkflowEventRow>> {
        let wf = self.workflow_by_name(name)?;
        let mut stmt = self.conn.prepare(
            "SELECT id, workflow_id, kind, data, at FROM workflow_events
             WHERE workflow_id = ?1 ORDER BY id",
        )?;
        let rows = stmt
            .query_map(params![wf.id], |r| {
                Ok(WorkflowEventRow {
                    id: r.get(0)?,
                    workflow_id: r.get(1)?,
                    kind: r.get(2)?,
                    data: r.get(3)?,
                    at: r.get(4)?,
                })
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }

    pub fn workflow_run(&self, id: &str) -> Result<WorkflowRunRow> {
        self.conn
            .query_row(
                &format!("{WORKFLOW_RUN_SELECT} WHERE r.id = ?1"),
                params![id],
                row_to_workflow_run,
            )
            .with_context(|| format!("workflow run {id} not found"))
    }

    pub fn workflow_runs(&self, name: &str) -> Result<Vec<WorkflowRunRow>> {
        let wf = self.workflow_by_name(name)?;
        let mut stmt = self.conn.prepare(&format!(
            "{WORKFLOW_RUN_SELECT} WHERE r.workflow_id = ?1 ORDER BY r.created_at, r.id"
        ))?;
        let rows = stmt
            .query_map(params![wf.id], row_to_workflow_run)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }
}

/// One workflow's full projection: row + revision metadata + parsed active
/// document. The same shape backs `bb workflow show --json` and
/// `GET /api/workflows/<name>` so the two surfaces cannot drift.
pub fn workflow_view(ledger: &Ledger, name: &str) -> Result<serde_json::Value> {
    let wf = ledger.workflow_by_name(name)?;
    let revisions = ledger
        .workflow_revisions(name)?
        .into_iter()
        .map(|r| {
            serde_json::json!({
                "revision": r.revision,
                "source": r.source,
                "note": r.note,
                "created_at": r.created_at,
            })
        })
        .collect::<Vec<_>>();
    let active_document = match wf.active_revision {
        Some(revision) => Some(revision_document_view(ledger, name, revision)?),
        None => None,
    };
    Ok(serde_json::json!({
        "workflow": wf,
        "revisions": revisions,
        "active_document": active_document,
    }))
}

pub fn revision_view(ledger: &Ledger, name: &str, revision: i64) -> Result<serde_json::Value> {
    let row = ledger.workflow_revision(name, revision)?;
    let document: serde_json::Value = serde_json::from_str(&row.document)?;
    Ok(serde_json::json!({
        "workflow": name,
        "revision": row.revision,
        "source": row.source,
        "note": row.note,
        "created_at": row.created_at,
        "document": document,
    }))
}

fn revision_document_view(ledger: &Ledger, name: &str, revision: i64) -> Result<serde_json::Value> {
    let row = ledger.workflow_revision(name, revision)?;
    Ok(serde_json::from_str(&row.document)?)
}

/// One workflow run plus the exact document it pinned at acceptance — the
/// readback surface proving a run's configuration survives later
/// activations unchanged.
pub fn workflow_run_view(ledger: &Ledger, run_id: &str) -> Result<serde_json::Value> {
    let run = ledger.workflow_run(run_id)?;
    let document = revision_document_view(ledger, &run.workflow, run.revision)?;
    Ok(serde_json::json!({
        "run": run,
        "document": document,
    }))
}

/// Export one revision (default: the active revision, else the latest) as
/// the declarative TOML interchange document. Returns the revision exported
/// so callers can prove which snapshot the file came from.
pub fn export_toml(ledger: &Ledger, name: &str, revision: Option<i64>) -> Result<(i64, String)> {
    let wf = ledger.workflow_by_name(name)?;
    let revision = match revision.or(wf.active_revision) {
        Some(revision) => revision,
        None => ledger
            .workflow_revisions(name)?
            .last()
            .map(|r| r.revision)
            .context("workflow has no revisions (corrupt state)")?,
    };
    let row = ledger.workflow_revision(name, revision)?;
    let doc = WorkflowDoc::from_canonical_json(&row.document)?;
    Ok((revision, doc.to_toml()?))
}

/// Line diff between two stored revisions of one workflow (canonical JSON,
/// so the diff is deterministic). LCS over lines; output rows are
/// `{op: " "|"-"|"+", line}`.
pub fn diff_view(ledger: &Ledger, name: &str, from: i64, to: i64) -> Result<serde_json::Value> {
    let a = ledger.workflow_revision(name, from)?.document;
    let b = ledger.workflow_revision(name, to)?.document;
    let changes = diff_lines(&a, &b)
        .into_iter()
        .filter(|(op, _)| *op != ' ')
        .map(|(op, line)| serde_json::json!({ "op": op.to_string(), "line": line }))
        .collect::<Vec<_>>();
    Ok(serde_json::json!({
        "workflow": name,
        "from": from,
        "to": to,
        "identical": changes.is_empty(),
        "changes": changes,
    }))
}

fn diff_lines(a: &str, b: &str) -> Vec<(char, String)> {
    let a: Vec<&str> = a.lines().collect();
    let b: Vec<&str> = b.lines().collect();
    // LCS table; documents are small config, not transcripts.
    let mut lcs = vec![vec![0usize; b.len() + 1]; a.len() + 1];
    for i in (0..a.len()).rev() {
        for j in (0..b.len()).rev() {
            lcs[i][j] = if a[i] == b[j] {
                lcs[i + 1][j + 1] + 1
            } else {
                lcs[i + 1][j].max(lcs[i][j + 1])
            };
        }
    }
    let (mut i, mut j) = (0, 0);
    let mut out = Vec::new();
    while i < a.len() && j < b.len() {
        if a[i] == b[j] {
            out.push((' ', a[i].to_string()));
            i += 1;
            j += 1;
        } else if lcs[i + 1][j] >= lcs[i][j + 1] {
            out.push(('-', a[i].to_string()));
            i += 1;
        } else {
            out.push(('+', b[j].to_string()));
            j += 1;
        }
    }
    out.extend(a[i..].iter().map(|l| ('-', l.to_string())));
    out.extend(b[j..].iter().map(|l| ('+', l.to_string())));
    out
}
