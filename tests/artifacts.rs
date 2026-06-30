//! Library tests for the shared artifact helpers: multi-attempt precedence,
//! binary/oversized rejection, missing artifacts, path traversal rejection,
//! and a successful REPORT.json read. Constructs attempts directly against a
//! real ledger + tempdirs so the helper is exercised without full dispatch.

use std::fs;
use std::path::Path;

use bitterblossom::{
    artifacts::{self, ReadOutcome},
    ledger::{AttemptStats, IngressRequest, Ledger},
};

fn run_id(ledger: &mut Ledger) -> String {
    ledger
        .ingest(IngressRequest {
            task: "demo",
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: None,
            parent_run_id: None,
        })
        .unwrap()
        .run_id
}

/// Create an attempt row whose artifact_dir is `dir`, finishing it so the
/// helper sees the dir on disk.
fn add_attempt(ledger: &mut Ledger, run_id: &str, n: i64, dir: &Path) {
    let id = ledger
        .create_attempt(run_id, n, "stub", 1, "claude", "stub-model")
        .unwrap();
    let dir_s = dir.to_string_lossy().into_owned();
    ledger
        .finish_attempt(
            id,
            "success",
            None,
            Some(0),
            &AttemptStats::default(),
            Some(&dir_s),
        )
        .unwrap();
}

#[test]
fn read_report_json_from_newest_attempt_that_has_it() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);

    let a1 = dir.path().join("a1");
    let a2 = dir.path().join("a2");
    fs::create_dir_all(&a1).unwrap();
    fs::create_dir_all(&a2).unwrap();
    // Both attempts wrote REPORT.json; the newer one (n=2) must win.
    fs::write(a1.join("REPORT.json"), r#"{"attempt":1}"#).unwrap();
    fs::write(a2.join("REPORT.json"), r#"{"attempt":2}"#).unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);
    add_attempt(&mut ledger, &run, 2, &a2);

    let outcome = artifacts::read(&ledger, &run, "REPORT.json").unwrap();
    match outcome {
        ReadOutcome::Text {
            attempt, content, ..
        } => {
            assert_eq!(attempt, 2);
            assert!(content.contains(r#""attempt":2"#));
        }
        other => panic!("expected text, got {other:?}"),
    }
}

#[test]
fn read_falls_back_to_earlier_attempt_when_newest_lacks_file() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);

    let a1 = dir.path().join("a1");
    let a2 = dir.path().join("a2");
    fs::create_dir_all(&a1).unwrap();
    fs::create_dir_all(&a2).unwrap();
    fs::write(a1.join("REPORT.json"), r#"{"attempt":1}"#).unwrap();
    // newest attempt has no REPORT.json (e.g. a failed retry)
    add_attempt(&mut ledger, &run, 1, &a1);
    add_attempt(&mut ledger, &run, 2, &a2);

    let outcome = artifacts::read(&ledger, &run, "REPORT.json").unwrap();
    match outcome {
        ReadOutcome::Text { attempt, .. } => assert_eq!(attempt, 1),
        other => panic!("expected text from attempt 1, got {other:?}"),
    }
}

#[test]
fn missing_artifact_is_reported_not_panicked() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);
    let a1 = dir.path().join("a1");
    fs::create_dir_all(&a1).unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);

    let outcome = artifacts::read(&ledger, &run, "NOPE.json").unwrap();
    assert!(matches!(outcome, ReadOutcome::Missing { .. }));
}

#[test]
fn path_traversal_is_rejected() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);
    let a1 = dir.path().join("a1");
    fs::create_dir_all(&a1).unwrap();
    fs::write(dir.path().join("secret"), "topsecret").unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);

    for bad in ["../secret", "/etc/passwd", "..", ".", "a/../../b"] {
        let err = artifacts::read(&ledger, &run, bad).unwrap_err();
        assert!(
            err.to_string()
                .contains("must be a non-empty relative path"),
            "traversal {bad:?} not rejected: {err}"
        );
    }
}

#[test]
fn symlink_escaping_artifact_root_is_rejected() {
    // safe_relative blocks lexical `..`; a symlink is the other escape
    // vector and must be caught by the canonicalize guard.
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);
    let a1 = dir.path().join("a1");
    fs::create_dir_all(&a1).unwrap();
    fs::write(dir.path().join("outside"), "outside").unwrap();
    #[cfg(unix)]
    std::os::unix::fs::symlink(dir.path().join("outside"), a1.join("escape")).unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);

    #[cfg(unix)]
    {
        let err = artifacts::read(&ledger, &run, "escape").unwrap_err();
        assert!(err.to_string().contains("escapes attempt artifact root"));
    }
}

#[test]
fn binary_artifact_is_refused_not_streamed() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);
    let a1 = dir.path().join("a1");
    fs::create_dir_all(&a1).unwrap();
    fs::write(a1.join("blob.bin"), [0u8, 1, 2, 0, 3]).unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);

    let outcome = artifacts::read(&ledger, &run, "blob.bin").unwrap();
    match outcome {
        ReadOutcome::Binary { size, .. } => assert_eq!(size, 5),
        other => panic!("expected binary, got {other:?}"),
    }
}

#[test]
fn oversized_artifact_is_refused_without_reading_full_file() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);
    let a1 = dir.path().join("a1");
    fs::create_dir_all(&a1).unwrap();
    // 2 MiB of zeros > 1 MiB limit; must be rejected by size, not loaded.
    fs::write(a1.join("big.log"), vec![b' '; (2 << 20) as usize]).unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);

    let outcome = artifacts::read(&ledger, &run, "big.log").unwrap();
    match outcome {
        ReadOutcome::Oversized { size, limit, .. } => {
            assert!(size > limit);
            assert_eq!(limit, 1 << 20);
        }
        other => panic!("expected oversized, got {other:?}"),
    }
}

#[test]
fn list_enumerates_top_level_files_across_attempts() {
    let dir = tempfile::tempdir().unwrap();
    let db = dir.path().join("plane.db");
    let mut ledger = Ledger::open(&db).unwrap();
    let run = run_id(&mut ledger);

    let a1 = dir.path().join("a1");
    let a2 = dir.path().join("a2");
    fs::create_dir_all(a1.join("workspace")).unwrap(); // scratch clone, must be excluded
    fs::create_dir_all(&a2).unwrap();
    fs::write(a1.join("REPORT.json"), "{}").unwrap();
    fs::write(a1.join("harness.pid"), "12345").unwrap(); // internal, excluded
    fs::write(a1.join("result.md"), "ok").unwrap();
    fs::write(a2.join("REPORT.json"), "{}").unwrap();
    add_attempt(&mut ledger, &run, 1, &a1);
    add_attempt(&mut ledger, &run, 2, &a2);

    let entries = artifacts::list(&ledger, &run).unwrap();
    let paths: Vec<(i64, &str)> = entries
        .iter()
        .map(|e| (e.attempt, e.path.as_str()))
        .collect();
    assert!(paths.contains(&(1, "REPORT.json")));
    assert!(paths.contains(&(1, "result.md")));
    assert!(paths.contains(&(2, "REPORT.json")));
    // workspace dir and internal pid must not appear
    assert!(!paths.iter().any(|(_, p)| *p == "workspace"));
    assert!(!paths.iter().any(|(_, p)| *p == "harness.pid"));

    let report = entries
        .iter()
        .find(|e| e.attempt == 2 && e.path == "REPORT.json")
        .unwrap();
    assert_eq!(report.content_type, "application/json");
    assert!(!report.binary);
    assert_eq!(report.size, 2);
}

#[test]
fn list_missing_run_bails() {
    let dir = tempfile::tempdir().unwrap();
    let ledger = Ledger::open(&dir.path().join("plane.db")).unwrap();
    let err = artifacts::list(&ledger, "no-such-run").unwrap_err();
    assert!(err.to_string().contains("not found"));
}
