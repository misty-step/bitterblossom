use anyhow::Result;
use serde::Serialize;

use crate::dispatch::{attempt_dir, attempt_marker};
use crate::ledger::{phase_reached, Ledger};
use crate::spec::Plane;
use crate::substrate::{self, ProbeResult};

#[derive(Serialize)]
pub struct RecoveryReport {
    pub run_id: String,
    pub task: String,
    pub attempt_phase: Option<String>,
    pub probe: Option<String>,
    pub disposition: String,
}

pub fn recover_inherited_runs(plane: &Plane, ledger: &mut Ledger) -> Result<Vec<RecoveryReport>> {
    let mut reports = Vec::new();
    for run in ledger.runs_in_state("running")? {
        let attempts = ledger.attempts(&run.id)?;
        let latest = attempts.last();
        let release_lease = |ledger: &Ledger| -> Result<()> {
            if let Ok(task) = plane.task(&run.task) {
                ledger.release_host_lease(&task.host(), &run.id)?;
            }
            Ok(())
        };

        let report = match latest {
            None => {
                let dl = ledger.record_dead_letter(
                    &run.id,
                    &run.task,
                    ledger.run_payload(&run.id)?.as_deref(),
                    "plane died before any attempt started",
                )?;
                ledger.transition(
                    &run.id,
                    "failure",
                    Some(&format!("dead_letter:{dl} recovered at boot, pre-attempt")),
                )?;
                release_lease(ledger)?;
                RecoveryReport {
                    run_id: run.id.clone(),
                    task: run.task.clone(),
                    attempt_phase: None,
                    probe: None,
                    disposition: format!("dead_letter:{dl}"),
                }
            }
            Some(attempt) if !phase_reached(&attempt.phase, "executing") => {
                let dl = ledger.record_dead_letter(
                    &run.id,
                    &run.task,
                    ledger.run_payload(&run.id)?.as_deref(),
                    &format!("plane died pre-execute (phase {})", attempt.phase),
                )?;
                ledger.transition(
                    &run.id,
                    "failure",
                    Some(&format!("dead_letter:{dl} recovered at boot, pre-execute")),
                )?;
                release_lease(ledger)?;
                RecoveryReport {
                    run_id: run.id.clone(),
                    task: run.task.clone(),
                    attempt_phase: Some(attempt.phase.clone()),
                    probe: None,
                    disposition: format!("dead_letter:{dl}"),
                }
            }
            Some(attempt) => {
                let probe = match plane.task(&run.task) {
                    Ok(task) => match substrate::for_task(&task.spec.substrate) {
                        Ok(sub) => sub.probe(
                            &task.host(),
                            &attempt_dir(plane, &run.id, attempt.n),
                            &attempt_marker(attempt.id),
                        ),
                        Err(e) => ProbeResult::Unknown(e.to_string()),
                    },
                    Err(e) => ProbeResult::Unknown(e.to_string()),
                };
                let probe_desc = match &probe {
                    ProbeResult::Alive => "alive".to_string(),
                    ProbeResult::Dead => "dead".to_string(),
                    ProbeResult::Unknown(why) => format!("unknown: {why}"),
                };
                ledger.record_event(&run.id, "boot_probe", Some(&probe_desc))?;

                let disposition = if probe == ProbeResult::Alive {
                    "still_running".to_string()
                } else {
                    if probe == ProbeResult::Dead {
                        release_lease(ledger)?;
                    }
                    ledger.transition(
                        &run.id,
                        "awaiting_recovery",
                        Some(&format!(
                            "plane died at phase {}; probe: {probe_desc}; resolve with \
                             `bb runs resolve {} success|failure`",
                            attempt.phase, run.id
                        )),
                    )?;
                    "awaiting_recovery".to_string()
                };
                RecoveryReport {
                    run_id: run.id.clone(),
                    task: run.task.clone(),
                    attempt_phase: Some(attempt.phase.clone()),
                    probe: Some(probe_desc),
                    disposition,
                }
            }
        };
        reports.push(report);
    }
    Ok(reports)
}
