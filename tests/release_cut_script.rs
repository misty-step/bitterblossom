use std::fs;
use std::path::Path;
use std::process::{Command, Output};

fn init_repo(dir: &Path, version: &str) {
    run(dir, &["init", "-q", "-b", "master"]);
    run(dir, &["config", "user.email", "test@example.com"]);
    run(dir, &["config", "user.name", "Test"]);
    fs::write(
        dir.join("Cargo.toml"),
        format!(
            "[package]\nname = \"bitterblossom\"\nversion = \"{version}\"\nedition = \"2021\"\n"
        ),
    )
    .unwrap();
    fs::write(dir.join("README.md"), "placeholder\n").unwrap();
    run(dir, &["add", "."]);
    run(dir, &["commit", "-q", "-m", "init"]);
}

fn run(dir: &Path, args: &[&str]) {
    let status = Command::new("git")
        .args(args)
        .current_dir(dir)
        .status()
        .unwrap();
    assert!(status.success(), "git {args:?} failed");
}

fn commit(dir: &Path, name: &str, message: &str) {
    fs::write(dir.join(name), message).unwrap();
    run(dir, &["add", name]);
    run(dir, &["commit", "-q", "-m", message]);
}

fn cut_release(dir: &Path, args: &[&str]) -> Output {
    Command::new("python3")
        .arg(format!(
            "{}/scripts/bb-cut-release",
            env!("CARGO_MANIFEST_DIR")
        ))
        .args(["--repo-root", dir.to_str().unwrap()])
        .args(args)
        .output()
        .unwrap()
}

#[test]
fn dry_run_computes_next_version_and_tag_without_side_effects() {
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "0.1.0");
    commit(dir.path(), "a.txt", "feature commit");

    let out = cut_release(dir.path(), &["--bump", "minor", "--dry-run", "--json"]);
    assert!(
        out.status.success(),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
    let report: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(report["current_version"], "0.1.0");
    assert_eq!(report["next_version"], "0.2.0");
    assert_eq!(report["tag"], "v0.2.0");
    assert_eq!(report["dry_run"], true);
    assert_eq!(report["checks"]["clean_tree"], true);
    assert_eq!(report["checks"]["tag_available"], true);

    // No side effects: no tag created, tree still clean.
    let tags = Command::new("git")
        .args(["tag", "-l"])
        .current_dir(dir.path())
        .output()
        .unwrap();
    assert_eq!(String::from_utf8_lossy(&tags.stdout).trim(), "");
}

#[test]
fn bump_kinds_compute_the_right_next_version() {
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "1.2.3");

    for (bump, expected) in [("patch", "1.2.4"), ("minor", "1.3.0"), ("major", "2.0.0")] {
        let out = cut_release(dir.path(), &["--bump", bump, "--dry-run", "--json"]);
        assert!(out.status.success());
        let report: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
        assert_eq!(report["next_version"], expected, "bump={bump}");
    }
}

#[test]
fn dry_run_reports_dirty_tree_as_a_failing_check_but_does_not_refuse() {
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "0.1.0");
    fs::write(dir.path().join("dirty.txt"), "uncommitted").unwrap();

    let out = cut_release(dir.path(), &["--dry-run", "--json"]);
    assert!(out.status.success(), "dry-run itself never refuses");
    let report: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(report["checks"]["clean_tree"], false);
}

#[test]
fn live_refuses_on_dirty_tree_without_touching_git_state() {
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "0.1.0");
    fs::write(dir.path().join("dirty.txt"), "uncommitted").unwrap();

    let out = cut_release(dir.path(), &["--live"]);
    assert!(!out.status.success());
    assert!(
        String::from_utf8_lossy(&out.stderr).contains("clean_tree"),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );

    let tags = Command::new("git")
        .args(["tag", "-l"])
        .current_dir(dir.path())
        .output()
        .unwrap();
    assert_eq!(String::from_utf8_lossy(&tags.stdout).trim(), "");
}

#[test]
fn live_refuses_when_the_tag_already_exists() {
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "0.1.0");
    run(dir.path(), &["tag", "v0.1.1"]);

    let out = cut_release(dir.path(), &["--bump", "patch", "--live"]);
    assert!(!out.status.success());
    assert!(
        String::from_utf8_lossy(&out.stderr).contains("tag_available"),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
}

fn write_fake_gh(gh_dir: &Path) -> std::path::PathBuf {
    let fake_gh = gh_dir.join("fake-gh.sh");
    fs::write(
        &fake_gh,
        "#!/bin/sh\necho \"$@\" > \"$(dirname \"$0\")/gh-args.txt\"\n",
    )
    .unwrap();
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&fake_gh, fs::Permissions::from_mode(0o755)).unwrap();
    }
    fake_gh
}

#[test]
fn live_pushes_and_creates_the_release_when_every_check_passes() {
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "0.1.0");
    commit(dir.path(), "a.txt", "a feature");

    // A real local bare repo stands in for the remote so `git push` runs
    // for real without any network access; only the GitHub-facing boundary
    // (`gh`) is stubbed, matching how the rest of this repo's script tests
    // stub the external CLI boundary rather than the mechanism under test.
    let remote_dir = tempfile::tempdir().unwrap();
    run(remote_dir.path(), &["init", "-q", "--bare"]);
    run(
        dir.path(),
        &[
            "remote",
            "add",
            "origin",
            remote_dir.path().to_str().unwrap(),
        ],
    );

    let gh_dir = tempfile::tempdir().unwrap();
    let fake_gh = write_fake_gh(gh_dir.path());

    let out = cut_release(
        dir.path(),
        &[
            "--bump",
            "patch",
            "--live",
            "--gh-bin",
            fake_gh.to_str().unwrap(),
            "--json",
        ],
    );
    assert!(
        out.status.success(),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );
    let report: serde_json::Value = serde_json::from_slice(&out.stdout).unwrap();
    assert_eq!(report["tag"], "v0.1.1");
    assert_eq!(report["dry_run"], false);

    let tags = Command::new("git")
        .args(["tag", "-l"])
        .current_dir(dir.path())
        .output()
        .unwrap();
    assert_eq!(String::from_utf8_lossy(&tags.stdout).trim(), "v0.1.1");

    // Actually pushed, not just tagged locally.
    let remote_tags = Command::new("git")
        .args(["tag", "-l"])
        .current_dir(remote_dir.path())
        .output()
        .unwrap();
    assert_eq!(
        String::from_utf8_lossy(&remote_tags.stdout).trim(),
        "v0.1.1"
    );

    let gh_args = fs::read_to_string(gh_dir.path().join("gh-args.txt")).unwrap();
    assert!(gh_args.contains("release"));
    assert!(gh_args.contains("v0.1.1"));
}

#[test]
fn no_push_stops_before_touching_the_remote_or_github() {
    // A fixed bug: `--no-push` used to still call `gh release create`,
    // which (with no matching pushed tag) silently publishes a release
    // against the tip of the default branch instead of the intended commit.
    // --no-push must mean nothing remote happens at all.
    let dir = tempfile::tempdir().unwrap();
    init_repo(dir.path(), "0.1.0");
    commit(dir.path(), "a.txt", "a feature");

    let gh_dir = tempfile::tempdir().unwrap();
    let fake_gh = write_fake_gh(gh_dir.path());

    let out = cut_release(
        dir.path(),
        &[
            "--bump",
            "patch",
            "--live",
            "--no-push",
            "--gh-bin",
            fake_gh.to_str().unwrap(),
            "--json",
        ],
    );
    assert!(
        out.status.success(),
        "stderr:\n{}",
        String::from_utf8_lossy(&out.stderr)
    );

    let tags = Command::new("git")
        .args(["tag", "-l"])
        .current_dir(dir.path())
        .output()
        .unwrap();
    assert_eq!(
        String::from_utf8_lossy(&tags.stdout).trim(),
        "v0.1.1",
        "the local tag itself should still be created"
    );

    assert!(
        !gh_dir.path().join("gh-args.txt").exists(),
        "gh must never be invoked under --no-push"
    );
}
