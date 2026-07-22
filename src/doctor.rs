use std::io::Write;
use std::path::Path;
use std::process::{Command, Stdio};

use serde::Serialize;

use crate::ledger::{Ledger, LEDGER_SCHEMA_VERSION};
use crate::preflight;
use crate::spec::Plane;

#[derive(Debug, Serialize)]
pub struct DoctorCheck {
    pub name: &'static str,
    pub status: &'static str,
    pub detail: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub remediation: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct DoctorReport {
    pub ok: bool,
    pub checks: Vec<DoctorCheck>,
}

fn check(
    name: &'static str,
    status: &'static str,
    detail: impl Into<String>,
    remediation: Option<String>,
) -> DoctorCheck {
    DoctorCheck {
        name,
        status,
        detail: detail.into(),
        remediation,
    }
}

pub fn run(root: &Path, expect_serve: bool) -> DoctorReport {
    let mut checks = Vec::new();

    let plane = match Plane::load(root) {
        Ok(plane) => plane,
        Err(e) => {
            checks.push(check(
                "config",
                "fail",
                format!("plane config at {} failed to load: {e:#}", root.display()),
                Some(format!(
                    "fix plane.toml/agents/tasks under {}, then rerun `bb doctor --config {}`",
                    root.display(),
                    root.display()
                )),
            ));
            return DoctorReport { ok: false, checks };
        }
    };
    checks.push(ok_check(
        "config",
        format!("plane loaded from {}", plane.root.display()),
    ));
    checks.push(database_check(&plane));
    checks.extend(preflight_checks(&plane));

    let bind = effective_bind(&plane);
    if let Some(bind) = fixed_bind(&bind) {
        checks.push(serve_health_check(&bind, expect_serve));
        checks.push(dashboard_check(&bind, expect_serve));
        if !plane.spec.dev {
            checks.push(retired_dashboard_check());
        }
    } else if expect_serve {
        checks.push(check(
            "serve_api",
            "fail",
            format!("effective [ingress].bind is \"{bind}\" (ephemeral); --expect-serve needs a fixed address"),
            Some("set [ingress].bind to a fixed host:port in plane.toml before requiring a live serve check".into()),
        ));
    }

    DoctorReport {
        ok: checks.iter().all(|c| c.status != "fail"),
        checks,
    }
}

fn ok_check(name: &'static str, detail: impl Into<String>) -> DoctorCheck {
    check(name, "ok", detail, None)
}

fn database_check(plane: &Plane) -> DoctorCheck {
    let db_path = plane.db_path();
    let ledger = match Ledger::open(&db_path) {
        Ok(ledger) => ledger,
        Err(e) => {
            let remediation = format!(
                "check permissions/disk space at {}, or restore from backup if the file is missing",
                db_path.display()
            );
            return check(
                "database",
                "fail",
                format!("cannot open ledger at {}: {e:#}", db_path.display()),
                Some(remediation),
            );
        }
    };
    match ledger.schema_version() {
        Ok(v) if v == LEDGER_SCHEMA_VERSION => ok_check(
            "database",
            format!("ledger reachable at {}, schema v{v}", db_path.display()),
        ),
        Ok(v) => check(
            "database",
            "fail",
            format!("ledger schema v{v} does not match this build's supported v{LEDGER_SCHEMA_VERSION}"),
            Some("rebuild bb to match the ledger's schema version, or run the migration for this bb version".into()),
        ),
        Err(e) => check(
            "database",
            "fail",
            format!("cannot read ledger schema version: {e:#}"),
            Some(format!("ledger file at {} may be corrupt", db_path.display())),
        ),
    }
}

fn preflight_checks(plane: &Plane) -> Vec<DoctorCheck> {
    let report = preflight::run_all(plane);
    if report.findings.is_empty() {
        return vec![ok_check(
            "preflight",
            format!(
                "{} task(s) checked, no missing secrets or unspawnable binaries",
                report.tasks_checked.len()
            ),
        )];
    }
    report
        .findings
        .iter()
        .map(|f| {
            check(
                preflight_check_name(f.kind),
                "fail",
                format!("task '{}': {}", f.task, f.detail),
                f.remediation
                    .clone()
                    .or_else(|| Some(format!("see `bb preflight {} --json` for detail", f.task))),
            )
        })
        .collect()
}

fn preflight_check_name(kind: &str) -> &'static str {
    match kind {
        "missing_secret" | "missing_optional_secret" | "missing_provider_key" => "secrets",
        "unspawnable_binary" => "binaries",
        _ => "preflight",
    }
}

fn effective_bind(plane: &Plane) -> String {
    std::env::var("BB_INGRESS_BIND")
        .ok()
        .filter(|v| !v.trim().is_empty())
        .map(|v| v.trim().to_string())
        .unwrap_or_else(|| plane.spec.ingress.bind.trim().to_string())
}

fn fixed_bind(bind: &str) -> Option<String> {
    let bind = bind.trim();
    bind.rsplit_once(':')
        .and_then(|(_, port)| (port != "0" && !port.is_empty()).then(|| bind.to_string()))
}

fn retired_dashboard_check() -> DoctorCheck {
    let target = format!("gui/{}/com.misty-step.bb-dashboard", unsafe {
        libc::getuid()
    });
    match Command::new(std::env::var("BB_DOCTOR_LAUNCHCTL_BIN").unwrap_or_else(|_| "launchctl".into()))
        .args(["print", &target]).output()
    {
        Err(e) => check(
            "retired_dashboard",
            "skipped",
            format!("cannot inspect retired launchd label: {e}"),
            Some("run scripts/install-bb-local-primary.sh --retire-legacy-dashboard on macOS".into()),
        ),
        Ok(output) if output.status.success() => check(
            "retired_dashboard",
            "fail",
            "retired launchd label com.misty-step.bb-dashboard is still loaded",
            Some("run scripts/install-bb-local-primary.sh --retire-legacy-dashboard (explicitly unloads and removes ~/Library/LaunchAgents/com.misty-step.bb-dashboard.plist)".into()),
        ),
        Ok(_) => ok_check("retired_dashboard", "retired launchd label com.misty-step.bb-dashboard is not loaded"),
    }
}

fn serve_health_check(bind: &str, expect_serve: bool) -> DoctorCheck {
    match curl_get(&format!("http://{bind}/health")) {
        Ok(body) => ok_check("serve_api", format!("GET /health at {bind} -> {}", body.trim())),
        Err(e) => check(
            "serve_api",
            if expect_serve { "fail" } else { "skipped" },
            format!("GET /health at {bind} failed: {e}"),
            Some(format!("start `bb serve` for this config (or check the process managing it) and confirm it binds {bind}")),
        ),
    }
}

fn dashboard_check(bind: &str, expect_serve: bool) -> DoctorCheck {
    const TITLE_MARKER: &str = "<title>bitterblossom dashboard</title>";
    match curl_get(&format!("http://{bind}/")) {
        Ok(body) if body.contains(TITLE_MARKER) => ok_check("dashboard", format!("GET / at {bind} served the current dashboard")),
        Ok(_) => check(
            "dashboard",
            if expect_serve { "fail" } else { "skipped" },
            format!("GET / at {bind} responded but did not contain the expected dashboard title"),
            Some("the service at this address may be stale or not bb serve at all -- rebuild and restart bb serve".into()),
        ),
        Err(e) => check(
            "dashboard",
            if expect_serve { "fail" } else { "skipped" },
            format!("GET / at {bind} failed: {e}"),
            Some(format!("start `bb serve` for this config (or check the process managing it) and confirm it binds {bind}")),
        ),
    }
}

fn curl_get(url: &str) -> Result<String, String> {
    let config = format!(
        "fail\nsilent\nshow-error\nmax-time = 3\nurl = \"{}\"\n",
        curl_config_escape(url)
    );
    let mut child =
        Command::new(std::env::var("BB_DOCTOR_CURL_BIN").unwrap_or_else(|_| "curl".into()))
            .args(["--config", "-"])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .map_err(|e| format!("cannot spawn curl: {e}"))?;
    if let Some(mut stdin) = child.stdin.take() {
        let _ = stdin.write_all(config.as_bytes());
    }
    let output = child
        .wait_with_output()
        .map_err(|e| format!("cannot wait curl: {e}"))?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
        return Err(if stderr.is_empty() {
            format!("curl exited {}", output.status)
        } else {
            stderr
        });
    }
    Ok(String::from_utf8_lossy(&output.stdout).to_string())
}

fn curl_config_escape(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn write_local_plane(root: &Path) {
        fs::create_dir_all(root.join("agents")).unwrap();
        fs::create_dir_all(root.join("tasks/demo")).unwrap();
        fs::write(
            root.join("plane.toml"),
            "dev = true\n[ingress]\nbind = \"127.0.0.1:0\"\n",
        )
        .unwrap();
        let stub = root.join("stub.sh");
        fs::write(&stub, "#!/bin/sh\ncat >/dev/null\necho ok\n").unwrap();
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let mut perms = fs::metadata(&stub).unwrap().permissions();
            perms.set_mode(0o755);
            fs::set_permissions(&stub, perms).unwrap();
        }
        fs::write(
            root.join("agents/stub.toml"),
            format!(
                "harness = \"command\"\nmodel = \"\"\nbin = \"{}\"\n",
                stub.display()
            ),
        )
        .unwrap();
        fs::write(root.join("tasks/demo/card.md"), "demo\n").unwrap();
        fs::write(
            root.join("tasks/demo/task.toml"),
            "agent = \"stub\"\nsubstrate = \"local\"\n[[trigger]]\nkind = \"manual\"\n",
        )
        .unwrap();
    }

    #[test]
    fn doctor_reports_ok_on_a_healthy_local_plane() {
        let dir = tempfile::tempdir().unwrap();
        write_local_plane(dir.path());

        let report = run(dir.path(), false);

        assert!(report.ok, "{report:?}");
        assert!(report
            .checks
            .iter()
            .any(|c| c.name == "config" && c.status == "ok"));
        assert!(report
            .checks
            .iter()
            .any(|c| c.name == "database" && c.status == "ok"));
        assert!(report
            .checks
            .iter()
            .any(|c| c.name == "preflight" && c.status == "ok"));
        assert!(!report
            .checks
            .iter()
            .any(|c| c.name == "serve_api" || c.name == "dashboard"));
    }

    #[test]
    fn doctor_fails_on_invalid_config_and_stops_there() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("plane.toml"), "not = [valid").unwrap();

        let report = run(dir.path(), false);

        assert!(!report.ok);
        assert_eq!(report.checks.len(), 1, "{report:?}");
        assert_eq!(report.checks[0].name, "config");
        assert_eq!(report.checks[0].status, "fail");
        assert!(report.checks[0].remediation.is_some());
    }

    #[test]
    fn doctor_fails_on_missing_required_secret() {
        let dir = tempfile::tempdir().unwrap();
        write_local_plane(dir.path());
        let stub = dir.path().join("stub.sh");
        fs::write(
            dir.path().join("agents/stub.toml"),
            format!(
                "harness = \"command\"\nmodel = \"\"\nbin = \"{}\"\nsecrets = [\"BB_DOCTOR_TEST_MISSING_SECRET_XYZ\"]\n",
                stub.display()
            ),
        )
        .unwrap();

        let report = run(dir.path(), false);

        assert!(!report.ok);
        let secret_check = report
            .checks
            .iter()
            .find(|c| c.name == "secrets")
            .expect("missing secret must surface as a doctor check");
        assert_eq!(secret_check.status, "fail");
        assert!(secret_check
            .detail
            .contains("BB_DOCTOR_TEST_MISSING_SECRET_XYZ"));
    }

    #[test]
    fn doctor_fails_on_unspawnable_binary() {
        let dir = tempfile::tempdir().unwrap();
        write_local_plane(dir.path());
        fs::write(
            dir.path().join("agents/stub.toml"),
            "harness = \"command\"\nmodel = \"\"\nbin = \"/definitely/not/a/real/binary-xyz\"\n",
        )
        .unwrap();

        let report = run(dir.path(), false);

        assert!(!report.ok);
        let binary_check = report
            .checks
            .iter()
            .find(|c| c.name == "binaries")
            .expect("unspawnable binary must surface as a doctor check");
        assert_eq!(binary_check.status, "fail");
    }

    #[test]
    fn doctor_fails_loudly_on_dead_serve_api_when_expected() {
        let dir = tempfile::tempdir().unwrap();
        write_local_plane(dir.path());
        fs::write(
            dir.path().join("plane.toml"),
            "dev = true\n[ingress]\nbind = \"127.0.0.1:18799\"\n",
        )
        .unwrap();

        let report = run(dir.path(), true);

        assert!(!report.ok);
        let serve_check = report
            .checks
            .iter()
            .find(|c| c.name == "serve_api")
            .unwrap();
        assert_eq!(serve_check.status, "fail");
        assert!(serve_check.remediation.is_some());
        let dashboard_check = report
            .checks
            .iter()
            .find(|c| c.name == "dashboard")
            .unwrap();
        assert_eq!(dashboard_check.status, "fail");
    }

    #[test]
    fn doctor_skips_rather_than_fails_dead_serve_api_when_not_expected() {
        let dir = tempfile::tempdir().unwrap();
        write_local_plane(dir.path());
        fs::write(
            dir.path().join("plane.toml"),
            "dev = true\n[ingress]\nbind = \"127.0.0.1:18798\"\n",
        )
        .unwrap();

        let report = run(dir.path(), false);

        assert!(report.ok, "{report:?}");
        let serve_check = report
            .checks
            .iter()
            .find(|c| c.name == "serve_api")
            .unwrap();
        assert_eq!(serve_check.status, "skipped");
    }
}
