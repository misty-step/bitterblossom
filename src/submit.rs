use anyhow::{bail, Context, Result};
use rusqlite::{params, OptionalExtension};
use serde::{Deserialize, Serialize};

use crate::ledger::{new_id, now, Ledger};
use crate::spec::Plane;
pub const SUBMISSION_STATES: &[&str] = &["open", "clear", "blocked", "escalated", "abandoned"];

pub const VERDICTS: &[&str] = &["pass", "blocking", "advisory"];
pub const SEVERITIES: &[&str] = &["blocking", "serious", "minor"];

#[derive(Debug, Serialize)]
pub struct SubmissionRow {
    pub id: String,
    pub change_key: String,
    pub rev: String,
    pub round: i64,
    pub state: String,
    pub context: Option<String>,
    pub prior_report_json: Option<String>,
    pub report_json: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Finding {
    pub severity: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub line: Option<i64>,
    pub claim: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub evidence: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub fingerprint: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct VerdictDoc {
    pub verdict: String,
    #[serde(default)]
    pub findings: Vec<Finding>,
}

#[derive(Debug, Serialize)]
pub struct VerdictRow {
    pub kind: String,
    pub run_id: String,
    pub verdict: String,
    pub findings: Vec<Finding>,
}

#[derive(Debug, Serialize)]
pub struct RejectionRow {
    pub fingerprint: String,
    pub reason: String,
}

#[derive(Debug, Serialize)]
pub struct SubmissionBundle {
    pub submission: SubmissionRow,
    pub verdicts: Vec<VerdictRow>,
    pub rejections: Vec<RejectionRow>,
}

type StormRun = (String, String, Option<f64>, Option<String>);

pub fn fingerprint(kind: &str, file: Option<&str>, claim: &str) -> String {
    use sha2::{Digest, Sha256};
    let mut h = Sha256::new();
    h.update(format!("{kind}|{}|{claim}", file.unwrap_or("")));
    format!("{:x}", h.finalize())[..16].to_string()
}
pub fn parse_verdict(kind: &str, raw: &str) -> Result<VerdictDoc> {
    let end = raw.rfind('}').context("no JSON object in verdict output")?;
    let mut parsed: Option<VerdictDoc> = None;
    let mut idx = 0;
    while let Some(off) = raw[idx..end].find('{') {
        let start = idx + off;
        if let Ok(d) = serde_json::from_str::<VerdictDoc>(&raw[start..=end]) {
            parsed = Some(d);
            break;
        }
        idx = start + 1;
    }
    let mut doc = parsed.context("verdict JSON malformed")?;
    if !VERDICTS.contains(&doc.verdict.as_str()) {
        bail!(
            "verdict '{}' not one of {}",
            doc.verdict,
            VERDICTS.join("|")
        );
    }
    for f in &mut doc.findings {
        if !SEVERITIES.contains(&f.severity.as_str()) {
            bail!(
                "finding severity '{}' not one of {}",
                f.severity,
                SEVERITIES.join("|")
            );
        }
        if f.fingerprint.is_none() {
            f.fingerprint = Some(fingerprint(kind, f.file.as_deref(), &f.claim));
        }
    }
    Ok(doc)
}

impl Ledger {
    pub fn open_submission(
        &mut self,
        change_key: &str,
        rev: &str,
        context: Option<&str>,
    ) -> Result<SubmissionRow> {
        let tx = self
            .conn
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)?;
        let prev: Option<(String, i64, Option<String>)> = tx
            .query_row(
                "SELECT state, round, report_json FROM submissions
                 WHERE change_key = ?1 ORDER BY rowid DESC LIMIT 1",
                params![change_key],
                |r| Ok((r.get(0)?, r.get(1)?, r.get(2)?)),
            )
            .optional()?;
        let (round, prior_report) = match prev {
            Some((state, _, _)) if state == "open" => {
                bail!("change '{change_key}' already has an open submission")
            }
            Some((state, round, report)) if state == "blocked" => (round + 1, report),
            _ => (1, None),
        };
        let id = new_id();
        let ts = now();
        tx.execute(
            "INSERT INTO submissions (id, change_key, rev, round, state, context,
               prior_report_json, created_at, updated_at)
             VALUES (?1, ?2, ?3, ?4, 'open', ?5, ?6, ?7, ?7)",
            params![id, change_key, rev, round, context, prior_report, ts],
        )?;
        tx.commit()?;
        self.submission(&id)
    }

    pub fn submission(&self, id: &str) -> Result<SubmissionRow> {
        self.conn
            .query_row(
                &format!("{SUBMISSION_SELECT} WHERE id = ?1"),
                params![id],
                row_to_submission,
            )
            .with_context(|| format!("submission {id} not found"))
    }
    pub fn latest_submission(&self, change_key: &str) -> Result<Option<SubmissionRow>> {
        Ok(self
            .conn
            .query_row(
                &format!(
                    "{SUBMISSION_SELECT} WHERE change_key = ?1
                     ORDER BY rowid DESC LIMIT 1"
                ),
                params![change_key],
                row_to_submission,
            )
            .optional()?)
    }
    pub fn settle_submission(&self, id: &str, state: &str, report_json: &str) -> Result<bool> {
        if !SUBMISSION_STATES.contains(&state) || state == "open" {
            bail!("illegal submission state '{state}'");
        }
        let n = self.conn.execute(
            "UPDATE submissions SET state = ?2, report_json = ?3, updated_at = ?4
             WHERE id = ?1 AND state = 'open'",
            params![id, state, report_json, now()],
        )?;
        Ok(n == 1)
    }

    pub fn record_verdict(
        &self,
        submission_id: &str,
        run_id: &str,
        kind: &str,
        doc: &VerdictDoc,
    ) -> Result<()> {
        self.submission(submission_id)?;
        self.conn.execute(
            "INSERT INTO verdicts (submission_id, run_id, kind, verdict, findings_json, created_at)
             VALUES (?1, ?2, ?3, ?4, ?5, ?6)
             ON CONFLICT(submission_id, kind, run_id) DO UPDATE SET
               verdict = ?4, findings_json = ?5, created_at = ?6",
            params![
                submission_id,
                run_id,
                kind,
                doc.verdict,
                serde_json::to_string(&doc.findings)?,
                now()
            ],
        )?;
        Ok(())
    }

    pub fn verdicts(&self, submission_id: &str) -> Result<Vec<VerdictRow>> {
        let mut stmt = self.conn.prepare(
            "SELECT kind, run_id, verdict, findings_json FROM verdicts
             WHERE submission_id = ?1 ORDER BY kind",
        )?;
        let rows = stmt
            .query_map(params![submission_id], |r| {
                Ok((
                    r.get::<_, String>(0)?,
                    r.get::<_, String>(1)?,
                    r.get::<_, String>(2)?,
                    r.get::<_, String>(3)?,
                ))
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        rows.into_iter()
            .map(|(kind, run_id, verdict, fj)| {
                Ok(VerdictRow {
                    kind,
                    run_id,
                    verdict,
                    findings: serde_json::from_str(&fj).context("stored findings_json")?,
                })
            })
            .collect()
    }

    pub fn list_submissions(&self, limit: i64) -> Result<Vec<SubmissionBundle>> {
        let mut stmt = self.conn.prepare(&format!(
            "{SUBMISSION_SELECT} ORDER BY created_at DESC LIMIT ?1"
        ))?;
        let submissions = stmt
            .query_map(params![limit], row_to_submission)?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        submissions
            .into_iter()
            .map(|submission| {
                let rejections = self
                    .rejections(&submission.change_key)?
                    .into_iter()
                    .map(|(fingerprint, reason)| RejectionRow {
                        fingerprint,
                        reason,
                    })
                    .collect();
                Ok(SubmissionBundle {
                    verdicts: self.verdicts(&submission.id)?,
                    rejections,
                    submission,
                })
            })
            .collect()
    }

    pub fn known_fingerprints(
        &self,
        sub: &SubmissionRow,
    ) -> Result<std::collections::BTreeSet<String>> {
        let mut known = std::collections::BTreeSet::new();
        if let Some(report) = &sub.prior_report_json {
            let v: serde_json::Value = serde_json::from_str(report)?;
            collect_fingerprints(&v, &mut known);
        }
        for verdict in self.verdicts(&sub.id)? {
            for f in verdict.findings {
                if let Some(fp) = f.fingerprint {
                    known.insert(fp);
                }
            }
        }
        Ok(known)
    }

    pub fn reject_finding(&self, change_key: &str, fp: &str, reason: &str) -> Result<()> {
        self.conn.execute(
            "INSERT INTO rejections (change_key, fingerprint, reason, created_at)
             VALUES (?1, ?2, ?3, ?4)
             ON CONFLICT(change_key, fingerprint) DO UPDATE SET reason = ?3",
            params![change_key, fp, reason, now()],
        )?;
        Ok(())
    }

    pub fn rejections(&self, change_key: &str) -> Result<Vec<(String, String)>> {
        let mut stmt = self.conn.prepare(
            "SELECT fingerprint, reason FROM rejections WHERE change_key = ?1 ORDER BY created_at",
        )?;
        let rows = stmt
            .query_map(params![change_key], |r| Ok((r.get(0)?, r.get(1)?)))?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        Ok(rows)
    }
    fn arbiter_sustains(&self, change_key: &str, arbiter_kind: &str, fp: &str) -> Result<bool> {
        let mut stmt = self.conn.prepare(
            "SELECT v.verdict, v.findings_json FROM verdicts v
             JOIN submissions s ON s.id = v.submission_id
             WHERE s.change_key = ?1 AND v.kind = ?2",
        )?;
        let rows = stmt
            .query_map(params![change_key, arbiter_kind], |r| {
                Ok((r.get::<_, String>(0)?, r.get::<_, String>(1)?))
            })?
            .collect::<rusqlite::Result<Vec<_>>>()?;
        for (verdict, fj) in rows {
            if verdict != "pass" {
                continue;
            }
            let findings: Vec<Finding> = serde_json::from_str(&fj)?;
            if findings
                .iter()
                .any(|f| f.fingerprint.as_deref() == Some(fp))
            {
                return Ok(true);
            }
        }
        Ok(false)
    }

    fn storm_run(&self, task: &str, key: &str) -> Result<Option<StormRun>> {
        Ok(self
            .conn
            .query_row(
                "SELECT id, state, cost_usd, state_reason FROM runs
                 WHERE task = ?1 AND idempotency_key = ?2",
                params![task, key],
                |r| Ok((r.get(0)?, r.get(1)?, r.get(2)?, r.get(3)?)),
            )
            .optional()?)
    }
}

fn collect_fingerprints(v: &serde_json::Value, out: &mut std::collections::BTreeSet<String>) {
    match v {
        serde_json::Value::Object(map) => {
            if let Some(serde_json::Value::String(fp)) = map.get("fingerprint") {
                out.insert(fp.clone());
            }
            map.values().for_each(|v| collect_fingerprints(v, out));
        }
        serde_json::Value::Array(items) => {
            items.iter().for_each(|v| collect_fingerprints(v, out));
        }
        _ => {}
    }
}
pub fn enforce_fingerprints(
    doc: &mut VerdictDoc,
    kind: &str,
    known: &std::collections::BTreeSet<String>,
) {
    for f in &mut doc.findings {
        if let Some(fp) = &f.fingerprint {
            if !known.contains(fp) {
                f.fingerprint = Some(fingerprint(kind, f.file.as_deref(), &f.claim));
            }
        }
    }
}

const SUBMISSION_SELECT: &str = "SELECT id, change_key, rev, round, state, context,
  prior_report_json, report_json, created_at, updated_at FROM submissions";

fn row_to_submission(r: &rusqlite::Row<'_>) -> rusqlite::Result<SubmissionRow> {
    Ok(SubmissionRow {
        id: r.get(0)?,
        change_key: r.get(1)?,
        rev: r.get(2)?,
        round: r.get(3)?,
        state: r.get(4)?,
        context: r.get(5)?,
        prior_report_json: r.get(6)?,
        report_json: r.get(7)?,
        created_at: r.get(8)?,
        updated_at: r.get(9)?,
    })
}

#[derive(Serialize)]
pub struct MemberStatus {
    pub kind: String,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub run_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cost_usd: Option<f64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub safe_next_command: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub safe_next_reason: Option<String>,
}

#[derive(Serialize)]
pub struct GateReport {
    pub submission: String,
    pub change_key: String,
    pub rev: String,
    pub round: i64,
    pub max_rounds: u32,
    pub decision: String,
    pub members: Vec<MemberStatus>,
    pub blocking: Vec<Finding>,
    pub advisory: Vec<Finding>,
    pub rejected: Vec<(Finding, String)>,
}
pub fn evaluate(plane: &Plane, ledger: &Ledger, submission_id: &str) -> Result<GateReport> {
    let gate = plane
        .spec
        .gate
        .as_ref()
        .context("plane.toml has no [gate] section")?;
    let sub = ledger.submission(submission_id)?;
    let rejections = ledger.rejections(&sub.change_key)?;

    let verdicts = ledger.verdicts(submission_id)?;
    let mut members = Vec::new();
    let mut counted: Vec<&VerdictRow> = Vec::new();
    let mut pending = false;
    let mut infra_failure = false;
    for kind in &gate.required {
        let task = plane
            .tasks
            .values()
            .find(|t| t.spec.verdict.as_deref() == Some(kind.as_str()))
            .with_context(|| format!("no task declares verdict = \"{kind}\""))?;
        let key = format!("storm:{submission_id}:{kind}");
        let run = ledger.storm_run(&task.name, &key)?;
        let (status, run_id, cost, safe_next_command, safe_next_reason) = match run {
            Some((id, state, cost, reason)) => {
                let canonical = verdicts
                    .iter()
                    .find(|v| v.kind == *kind && v.run_id == id && state == "success");
                match canonical {
                    Some(v) => {
                        counted.push(v);
                        (format!("verdict:{}", v.verdict), Some(id), cost, None, None)
                    }
                    None => {
                        if state == "failure" {
                            infra_failure = true;
                            let why = reason.clone().unwrap_or_else(|| "run failed".into());
                            let cmd = format!(
                                "bb --config {:?} submit open --change {} --rev {} --json",
                                plane.root, sub.change_key, sub.rev
                            );
                            let why = format!("canonical {kind} run {id} failed: {why}");
                            (format!("run:{state}"), Some(id), cost, Some(cmd), Some(why))
                        } else {
                            pending = true;
                            (format!("run:{state}"), Some(id), cost, None, None)
                        }
                    }
                }
            }
            None => {
                pending = true;
                ("not_started".to_string(), None, None, None, None)
            }
        };
        members.push(MemberStatus {
            kind: kind.clone(),
            status,
            run_id,
            cost_usd: cost,
            safe_next_command,
            safe_next_reason,
        });
    }

    let mut blocking = Vec::new();
    let mut advisory = Vec::new();
    let mut rejected = Vec::new();
    for v in counted {
        for f in &v.findings {
            let fp = f.fingerprint.as_deref().unwrap_or("");
            let rejection = rejections.iter().find(|(r, _)| r == fp);
            if f.severity == "blocking" {
                match rejection {
                    Some((_, reason))
                        if ledger.arbiter_sustains(&sub.change_key, &gate.arbiter, fp)? =>
                    {
                        rejected.push((f.clone(), reason.clone()));
                    }
                    _ => blocking.push(f.clone()),
                }
            } else {
                match rejection {
                    Some((_, reason)) => rejected.push((f.clone(), reason.clone())),
                    None => advisory.push(f.clone()),
                }
            }
        }
    }

    let decision = if infra_failure {
        "escalated"
    } else if pending {
        "pending"
    } else if blocking.is_empty() {
        "clear"
    } else if sub.round >= gate.max_rounds as i64 {
        "escalated"
    } else {
        "blocked"
    };

    let report = GateReport {
        submission: sub.id.clone(),
        change_key: sub.change_key.clone(),
        rev: sub.rev.clone(),
        round: sub.round,
        max_rounds: gate.max_rounds,
        decision: decision.to_string(),
        members,
        blocking,
        advisory,
        rejected,
    };

    if sub.state == "open" && decision != "pending" {
        let json = serde_json::to_string(&report)?;
        let settled = ledger.settle_submission(&sub.id, decision, &json)?;
        if settled && decision == "escalated" {
            crate::notify::notify(
                plane,
                "submission_escalated",
                &serde_json::json!({
                    "submission": sub.id, "change": sub.change_key,
                    "round": sub.round, "blocking": report.blocking.len(),
                }),
            );
        }
        // A blocked gate fires `gate.blocked` carrying every blocking finding
        // (fingerprint/file/line/claim) — the contract the fix-prompt reflex
        // consumes to mint a bounded builder packet. Report-only, like escalate.
        if settled && decision == "blocked" {
            crate::notify::notify(
                plane,
                "gate.blocked",
                &serde_json::json!({
                    "submission": sub.id, "change": sub.change_key,
                    "round": sub.round, "blocking": report.blocking,
                }),
            );
        }
    }
    Ok(report)
}
