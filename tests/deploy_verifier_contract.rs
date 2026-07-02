use serde_json::Value;

const DEPLOY_VERIFIER_EVENT_SCHEMA_VERSION: &str = "bb.deploy_prod_verifier_event.v1";

#[test]
fn deploy_verifier_event_fixture_matches_pinned_contract() {
    let schema: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.deploy_prod_verifier_event.v1.schema.json"
    ))
    .unwrap();
    assert_eq!(
        schema["properties"]["schema_version"]["const"],
        DEPLOY_VERIFIER_EVENT_SCHEMA_VERSION
    );
    assert_schema_requires(
        &schema,
        &[
            "schema_version",
            "event",
            "subject",
            "evidence",
            "requested_checks",
            "constraints",
        ],
    );

    let fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.deploy_prod_verifier_event.v1.valid.json"
    ))
    .unwrap();
    assert_deploy_verifier_event(&fixture).unwrap();
}

#[test]
fn deploy_verifier_event_contract_fails_on_field_rename() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.deploy_prod_verifier_event.v1.valid.json"
    ))
    .unwrap();
    let subject = fixture["subject"].as_object_mut().unwrap();
    let revision = subject.remove("revision").unwrap();
    subject.insert("sha".to_string(), revision);

    let error = assert_deploy_verifier_event(&fixture).unwrap_err();
    assert!(error.contains("subject.revision"), "{error}");
}

#[test]
fn deploy_verifier_event_contract_rejects_unknown_major_version() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.deploy_prod_verifier_event.v1.valid.json"
    ))
    .unwrap();
    fixture["schema_version"] = Value::String("bb.deploy_prod_verifier_event.v2".to_string());

    let error = assert_deploy_verifier_event(&fixture).unwrap_err();
    assert!(
        error.contains("unsupported schema_version bb.deploy_prod_verifier_event.v2"),
        "{error}"
    );
}

fn assert_schema_requires(schema: &Value, fields: &[&str]) {
    let required = schema["required"].as_array().unwrap();
    for field in fields {
        assert!(
            required.iter().any(|required| required == field),
            "schema required list missing {field}: {schema}"
        );
    }
}

fn assert_deploy_verifier_event(value: &Value) -> Result<(), String> {
    let schema_version = required_string(value, "schema_version")?;
    if schema_version != DEPLOY_VERIFIER_EVENT_SCHEMA_VERSION {
        return Err(format!("unsupported schema_version {schema_version}"));
    }
    let event = required_string(value, "event")?;
    if !matches!(event, "deploy_smoke.failed" | "production_incident.opened") {
        return Err(format!("unsupported event {event}"));
    }

    let subject = required_object(value, "subject")?;
    expect_string(subject, "type", "subject.type", "deploy")?;
    required_string_at(subject, "service", "subject.service")?;
    required_string_at(subject, "environment", "subject.environment")?;
    required_string_at(subject, "repo", "subject.repo")?;
    required_string_at(subject, "revision", "subject.revision")?;
    required_nonempty_array_at(subject, "target_urls", "subject.target_urls")?;

    required_nonempty_array(value, "evidence")?;
    required_nonempty_array(value, "requested_checks")?;

    let constraints = required_object(value, "constraints")?;
    expect_bool(constraints, "report_only", "constraints.report_only", true)?;
    expect_bool(
        constraints,
        "no_code_edits",
        "constraints.no_code_edits",
        true,
    )?;
    expect_bool(constraints, "no_deploys", "constraints.no_deploys", true)?;
    Ok(())
}

fn required_object<'a>(value: &'a Value, key: &str) -> Result<&'a Value, String> {
    match value.get(key) {
        Some(value @ Value::Object(_)) => Ok(value),
        _ => Err(format!("missing object {key}")),
    }
}

fn required_string<'a>(value: &'a Value, key: &str) -> Result<&'a str, String> {
    required_string_at(value, key, key)
}

fn required_string_at<'a>(value: &'a Value, key: &str, label: &str) -> Result<&'a str, String> {
    match value.get(key).and_then(Value::as_str) {
        Some(value) if !value.is_empty() => Ok(value),
        _ => Err(format!("missing string {label}")),
    }
}

fn required_nonempty_array(value: &Value, key: &str) -> Result<(), String> {
    required_nonempty_array_at(value, key, key)
}

fn required_nonempty_array_at(value: &Value, key: &str, label: &str) -> Result<(), String> {
    match value.get(key).and_then(Value::as_array) {
        Some(values) if !values.is_empty() => Ok(()),
        _ => Err(format!("missing non-empty array {label}")),
    }
}

fn expect_string(value: &Value, key: &str, label: &str, expected: &str) -> Result<(), String> {
    let actual = required_string_at(value, key, label)?;
    if actual == expected {
        Ok(())
    } else {
        Err(format!("expected {label}={expected}, got {actual}"))
    }
}

fn expect_bool(value: &Value, key: &str, label: &str, expected: bool) -> Result<(), String> {
    match value.get(key).and_then(Value::as_bool) {
        Some(actual) if actual == expected => Ok(()),
        Some(actual) => Err(format!("expected {label}={expected}, got {actual}")),
        None => Err(format!("missing bool {label}")),
    }
}
