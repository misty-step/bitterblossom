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
    pub probe_state: Option<String>,
    pub probe_reason: Option<String>,
    pub lease_disposition: Option<String>,
    pub operator_action: Option<String>,
    pub disposition: String,
}

pub fn recover_inherited_runs(plane: &Plane, ledger: &mut Ledger) -> Result<Vec<RecoveryReport>> {
    let mut reports = Vec::new();
    for run in ledger.runs_in_state("running")? {
        let attempts = ledger.attempts(&run.id)?;
        let latest = attempts.last();
        let release_lease = |ledger: &Ledger| -> Result<()> {
            // Release by run identity, not the current task host. A renamed or
            // removed task declaration must not strand a lease forever.
            ledger.release_host_leases_for_run(&run.id)?;
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
                    probe_state: None,
                    probe_reason: None,
                    lease_disposition: Some("released".to_string()),
                    operator_action: Some("replay_dead_letter_after_fixing_cause".to_string()),
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
                    probe_state: None,
                    probe_reason: None,
                    lease_disposition: Some("released".to_string()),
                    operator_action: Some("replay_dead_letter_after_fixing_cause".to_string()),
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
                let probe_desc = probe.description();
                ledger.record_event(&run.id, "boot_probe", Some(&probe_desc))?;

                let (disposition, lease_disposition, operator_action) =
                    if probe == ProbeResult::Alive {
                        (
                            "still_running".to_string(),
                            "retained".to_string(),
                            "observe_running_run".to_string(),
                        )
                    } else {
                        let lease_disposition = if probe == ProbeResult::Dead {
                            release_lease(ledger)?;
                            "released"
                        } else {
                            "retained"
                        };
                        ledger.transition(
                            &run.id,
                            "awaiting_recovery",
                            Some(&format!(
                                "plane died at phase {}; probe: {probe_desc}; resolve with \
                             `bb runs resolve {} success|failure`",
                                attempt.phase, run.id
                            )),
                        )?;
                        (
                            "awaiting_recovery".to_string(),
                            lease_disposition.to_string(),
                            "inspect_side_effects_then_resolve".to_string(),
                        )
                    };
                RecoveryReport {
                    run_id: run.id.clone(),
                    task: run.task.clone(),
                    attempt_phase: Some(attempt.phase.clone()),
                    probe: Some(probe_desc),
                    probe_state: Some(probe.state().to_string()),
                    probe_reason: probe.reason().map(ToString::to_string),
                    lease_disposition: Some(lease_disposition),
                    operator_action: Some(operator_action),
                    disposition,
                }
            }
        };
        reports.push(report);
    }
    Ok(reports)
}
