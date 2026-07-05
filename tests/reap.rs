//! Lane-checkout lifecycle (bitterblossom-921): a candidate is reap-eligible
//! only if it is clean, its HEAD is reachable on a remote branch, and it has
//! been idle past the grace period. Every other case must refuse, and the
//! refused tree must be provably untouched.

use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

use bitterblossom::ledger::{IngressRequest, Ledger};
use bitterblossom::reap;
use bitterblossom::spec::Plane;

fn make_plane(root: &Path) -> Plane {
    fs::create_dir_all(root.join("agents")).unwrap();
    fs::create_dir_all(root.join("tasks/demo")).unwrap();
    fs::write(root.join("plane.toml"), "dev = true\n").unwrap();
    fs::write(
        root.join("agents/a.toml"),
        "harness = \"claude\"\nmodel = \"m\"\n",
    )
    .unwrap();
    fs::write(root.join("tasks/demo/card.md"), "card\n").unwrap();
    fs::write(
        root.join("tasks/demo/task.toml"),
        "agent = \"a\"\nsubstrate = \"local\"\n\n[[trigger]]\nkind = \"manual\"\n",
    )
    .unwrap();
    Plane::load(root).unwrap()
}

fn terminal_run(ledger: &mut Ledger, task: &str, checkout_path: &str) -> String {
    let run_id = ledger
        .ingest(IngressRequest {
            task,
            trigger_kind: "manual",
            idempotency_key: None,
            source_event_id: None,
            payload: Some("{}"),
            parent_run_id: None,
        })
        .unwrap()
        .run_id;
    ledger.transition(&run_id, "running", None).unwrap();
    ledger.transition(&run_id, "success", None).unwrap();
    ledger
        .set_run_checkout_path(&run_id, checkout_path)
        .unwrap();
    run_id
}

fn git(dir: &Path, args: &[&str]) -> std::process::Output {
    Command::new("git")
        .args(args)
        .current_dir(dir)
        .output()
        .unwrap()
}

/// A bare "origin" plus a primary checkout with one pushed commit on
/// master. Returns (bare_dir, primary_dir).
fn make_repo_with_remote(base: &Path) -> (PathBuf, PathBuf) {
    let bare = base.join("origin.git");
    let primary = base.join("primary");
    fs::create_dir_all(&bare).unwrap();
    git(&bare, &["init", "--bare", "-q"]);
    fs::create_dir_all(&primary).unwrap();
    git(&primary, &["init", "-q", "-b", "master"]);
    git(&primary, &["config", "user.email", "test@example.com"]);
    git(&primary, &["config", "user.name", "test"]);
    fs::write(primary.join("README.md"), "hello\n").unwrap();
    git(&primary, &["add", "."]);
    git(&primary, &["commit", "-q", "-m", "init"]);
    git(
        &primary,
        &["remote", "add", "origin", bare.to_str().unwrap()],
    );
    git(&primary, &["push", "-q", "origin", "master"]);
    (bare, primary)
}

/// Add a worktree on a fresh branch, commit one file on it, and set the
/// commit's committer timestamp to `age_hours` ago (real git history, just
/// backdated so tests do not need to sleep for hours).
fn add_worktree_with_age(primary: &Path, name: &str, age_hours: f64) -> PathBuf {
    let wt = primary.parent().unwrap().join(name);
    git(
        primary,
        &["worktree", "add", "-q", "-b", name, wt.to_str().unwrap()],
    );
    fs::write(wt.join(format!("{name}.txt")), "work\n").unwrap();
    git(&wt, &["add", "."]);
    let ts = (chrono_now_secs() - (age_hours * 3600.0) as i64).to_string();
    Command::new("git")
        .args(["commit", "-q", "-m", "lane work"])
        .current_dir(&wt)
        .env("GIT_AUTHOR_DATE", &ts)
        .env("GIT_COMMITTER_DATE", &ts)
        .output()
        .unwrap();
    wt
}

fn chrono_now_secs() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64
}

fn push(primary: &Path, branch: &str) {
    git(primary, &["push", "-q", "origin", branch]);
}

fn status_clean(path: &Path) -> bool {
    let out = git(path, &["status", "--porcelain"]);
    out.stdout.is_empty()
}

#[test]
fn discovers_worktrees_and_excludes_the_primary_checkout() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-a", 0.0);

    let found = reap::discover_worktrees(&primary).unwrap();
    assert_eq!(found.len(), 1);
    assert_eq!(found[0].canonicalize().unwrap(), wt.canonicalize().unwrap());
}

#[test]
fn dirty_worktree_is_refused_and_left_untouched() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-dirty", 100.0);
    push(&primary, "lane-dirty");
    fs::write(wt.join("uncommitted.txt"), "oops\n").unwrap();

    let c = reap::evaluate(&wt, "discovered", None, 6.0);
    assert!(!c.eligible, "dirty tree must never be eligible");
    assert!(c.reason.contains("dirty"), "reason was: {}", c.reason);
    assert!(wt.exists(), "refused candidate must not be deleted");
    assert!(!status_clean(&wt), "the dirty file must still be there");
}

#[test]
fn unpushed_worktree_is_refused_even_when_clean() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-unpushed", 100.0);
    // deliberately never pushed

    let c = reap::evaluate(&wt, "discovered", None, 6.0);
    assert!(!c.eligible, "unpushed commit must never be eligible");
    assert!(c.reason.contains("unpushed"), "reason was: {}", c.reason);
    assert!(wt.exists());
}

#[test]
fn too_recent_worktree_is_refused_despite_being_clean_and_pushed() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-fresh", 0.5);
    push(&primary, "lane-fresh");

    let c = reap::evaluate(&wt, "discovered", None, 6.0);
    assert!(
        !c.eligible,
        "a lane finished 30 minutes ago is not fair game yet"
    );
    assert!(c.reason.contains("too recent"), "reason was: {}", c.reason);
    assert!(wt.exists());
}

#[test]
fn clean_pushed_aged_worktree_is_eligible_and_apply_removes_it() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-done", 100.0);
    push(&primary, "lane-done");

    let dry = reap::evaluate(&wt, "discovered", None, 6.0);
    assert!(dry.eligible, "reason was: {}", dry.reason);
    assert!(wt.exists(), "evaluate() alone must never delete anything");

    let ledger = Ledger::open(&base.path().join("empty.db")).unwrap();
    let candidates = reap::sweep(&ledger, std::slice::from_ref(&primary), 6.0, true, &[]).unwrap();
    let done = candidates
        .iter()
        .find(|c| c.path.contains("lane-done"))
        .unwrap();
    assert!(done.removed, "should have been removed: {}", done.reason);
    assert!(!wt.exists(), "the worktree directory should be gone");
}

#[test]
fn excluded_candidate_is_never_evaluated_or_touched_even_when_eligible() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-out-of-scope", 100.0);
    push(&primary, "lane-out-of-scope");

    let ledger = Ledger::open(&base.path().join("empty.db")).unwrap();
    let excludes = vec!["lane-out-of-scope".to_string()];
    let candidates = reap::sweep(
        &ledger,
        std::slice::from_ref(&primary),
        6.0,
        true,
        &excludes,
    )
    .unwrap();
    assert!(
        !candidates
            .iter()
            .any(|c| c.path.contains("lane-out-of-scope")),
        "an excluded path must not appear in the report at all"
    );
    assert!(
        wt.exists(),
        "excluded candidate must survive even with --apply"
    );
}

#[test]
fn dry_run_never_mutates_even_when_everything_is_eligible() {
    let base = tempfile::tempdir().unwrap();
    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-dryrun", 100.0);
    push(&primary, "lane-dryrun");

    let ledger = Ledger::open(&base.path().join("empty.db")).unwrap();
    let candidates = reap::sweep(&ledger, std::slice::from_ref(&primary), 6.0, false, &[]).unwrap();
    let found = candidates
        .iter()
        .find(|c| c.path.contains("lane-dryrun"))
        .unwrap();
    assert!(found.eligible);
    assert!(!found.removed, "dry run (apply=false) must never remove");
    assert!(wt.exists());
}

#[test]
fn registered_checkout_path_refusal_is_recorded_as_a_run_event() {
    let base = tempfile::tempdir().unwrap();
    let plane_dir = base.path().join("plane");
    fs::create_dir_all(&plane_dir).unwrap();
    let plane = make_plane(&plane_dir);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-registered-dirty", 100.0);
    push(&primary, "lane-registered-dirty");
    fs::write(wt.join("late-edit.txt"), "still working\n").unwrap();

    let run_id = terminal_run(&mut ledger, "demo", wt.to_str().unwrap());
    let candidates = reap::sweep(&ledger, &[], 6.0, true, &[]).unwrap();
    let found = candidates
        .iter()
        .find(|c| c.run_id.as_deref() == Some(run_id.as_str()))
        .unwrap();
    assert!(!found.eligible);
    assert!(wt.exists(), "dirty registered checkout must survive");

    let events = ledger.events(&run_id).unwrap();
    assert!(
        events.iter().any(|e| e.kind == "checkout_reap_refused"),
        "refusal must be visible on the run, not silently dropped"
    );
}

#[test]
fn registered_checkout_path_is_removed_when_safe() {
    let base = tempfile::tempdir().unwrap();
    let plane_dir = base.path().join("plane");
    fs::create_dir_all(&plane_dir).unwrap();
    let plane = make_plane(&plane_dir);
    let mut ledger = Ledger::open(&plane.db_path()).unwrap();

    let (_bare, primary) = make_repo_with_remote(base.path());
    let wt = add_worktree_with_age(&primary, "lane-registered-done", 100.0);
    push(&primary, "lane-registered-done");

    let run_id = terminal_run(&mut ledger, "demo", wt.to_str().unwrap());
    let candidates = reap::sweep(&ledger, &[], 6.0, true, &[]).unwrap();
    let found = candidates
        .iter()
        .find(|c| c.run_id.as_deref() == Some(run_id.as_str()))
        .unwrap();
    assert!(
        found.eligible && found.removed,
        "reason was: {}",
        found.reason
    );
    assert!(!wt.exists());

    let events = ledger.events(&run_id).unwrap();
    assert!(events.iter().any(|e| e.kind == "checkout_reaped"));
}

#[test]
fn set_checkout_path_cli_round_trips_through_the_ledger() {
    let base = tempfile::tempdir().unwrap();
    let plane_dir = base.path().join("plane");
    fs::create_dir_all(&plane_dir).unwrap();
    let plane = make_plane(&plane_dir);

    // Build the run directly (as recovery.rs's own tests do) rather than
    // through `bb run`, which would dispatch a real `claude` harness
    // process this test environment does not have. Only the new
    // `set-checkout-path` surface is under test here.
    let run_id = {
        let mut ledger = Ledger::open(&plane.db_path()).unwrap();
        ledger
            .ingest(IngressRequest {
                task: "demo",
                trigger_kind: "manual",
                idempotency_key: None,
                source_event_id: None,
                payload: Some("{}"),
                parent_run_id: None,
            })
            .unwrap()
            .run_id
    };

    let checkout = base.path().join("some-lane-checkout");
    fs::create_dir_all(&checkout).unwrap();

    let set = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            plane_dir.to_str().unwrap(),
            "runs",
            "set-checkout-path",
            &run_id,
            checkout.to_str().unwrap(),
        ])
        .output()
        .unwrap();
    assert!(
        set.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&set.stderr)
    );

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let run = ledger.run(&run_id).unwrap();
    assert_eq!(
        run.checkout_path.as_deref(),
        Some(checkout.canonicalize().unwrap().to_str().unwrap())
    );
}

#[test]
fn set_checkout_path_cli_refuses_a_non_directory() {
    let base = tempfile::tempdir().unwrap();
    let plane_dir = base.path().join("plane");
    fs::create_dir_all(&plane_dir).unwrap();
    let plane = make_plane(&plane_dir);
    let run_id = {
        let mut ledger = Ledger::open(&plane.db_path()).unwrap();
        ledger
            .ingest(IngressRequest {
                task: "demo",
                trigger_kind: "manual",
                idempotency_key: None,
                source_event_id: None,
                payload: Some("{}"),
                parent_run_id: None,
            })
            .unwrap()
            .run_id
    };

    let not_a_dir = base.path().join("nope.txt");
    fs::write(&not_a_dir, "not a directory\n").unwrap();

    let set = Command::new(env!("CARGO_BIN_EXE_bb"))
        .args([
            "--config",
            plane_dir.to_str().unwrap(),
            "runs",
            "set-checkout-path",
            &run_id,
            not_a_dir.to_str().unwrap(),
        ])
        .output()
        .unwrap();
    assert!(!set.status.success());

    let ledger = Ledger::open(&plane.db_path()).unwrap();
    let run = ledger.run(&run_id).unwrap();
    assert_eq!(
        run.checkout_path, None,
        "must not record a non-directory path"
    );
}
