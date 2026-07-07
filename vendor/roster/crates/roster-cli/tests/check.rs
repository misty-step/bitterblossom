use assert_cmd::Command;
use predicates::prelude::*;
use std::{fs, path::Path};
use tempfile::TempDir;

fn workspace_root() -> std::path::PathBuf {
    std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .parent()
        .and_then(|path| path.parent())
        .expect("workspace root")
        .to_path_buf()
}

fn write(dir: &Path, rel: &str, contents: &str) {
    let path = dir.join(rel);
    fs::create_dir_all(path.parent().expect("parent")).expect("mkdir");
    fs::write(path, contents).expect("write fixture file");
}

fn git(dir: &Path, args: &[&str]) {
    let status = std::process::Command::new("git")
        .args(args)
        .current_dir(dir)
        .status()
        .expect("run git");
    assert!(status.success(), "git {args:?} failed");
}

#[test]
fn check_fails_on_broken_frontmatter_and_dead_reference() {
    let tmp = TempDir::new().expect("tempdir");
    let root = tmp.path();

    write(
        root,
        "primitives/skills/skills-index.yaml",
        "schema_version: roster.skills_index.v1\nskills: []\n",
    );
    write(
        root,
        "primitives/skills/broken/SKILL.md",
        "---\ndescription: only a description, no name\n---\nbody\n",
    );
    write(
        root,
        "primitives/skills/dead-ref/SKILL.md",
        "---\nname: dead-ref\ndescription: cites a path that does not exist\nargument-hint: \"[x]\"\n---\n\nSee `primitives/shared/references/does-not-exist.md` for detail.\n",
    );

    git(root, &["init", "-q"]);
    git(
        root,
        &["config", "user.email", "roster-check-test@example.com"],
    );
    git(root, &["config", "user.name", "roster-check-test"]);
    git(root, &["add", "-A"]);
    git(root, &["commit", "-q", "-m", "fixture"]);

    Command::cargo_bin("roster")
        .expect("roster binary")
        .args(["--root", root.to_str().expect("utf8 path"), "check"])
        .assert()
        .failure()
        .stdout(predicate::str::contains("missing/empty name"))
        .stdout(predicate::str::contains("dead reference"));
}

#[test]
fn check_passes_on_the_real_repo() {
    Command::cargo_bin("roster")
        .expect("roster binary")
        .current_dir(workspace_root())
        .arg("check")
        .assert()
        .success();
}
