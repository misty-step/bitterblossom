use serde_json::Value;

const CANARY_INCIDENT_EVENT_SCHEMA_VERSION: &str = "canary.incident_event.v1";
const CANARY_TRIAGE_REPORT_SCHEMA_VERSION: &str = "bb.canary_incident_response.report.v1";

#[test]
fn canary_incident_event_fixture_matches_pinned_contract() {
    let schema: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/canary.incident_event.v1.schema.json"
    ))
    .unwrap();
    assert_eq!(
        schema["properties"]["schema_version"]["const"],
        CANARY_INCIDENT_EVENT_SCHEMA_VERSION
    );
    assert_schema_requires(
        &schema,
        &["schema_version", "event", "subject", "signal", "replay"],
    );

    let fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/canary.incident_event.v1.valid.json"
    ))
    .unwrap();
    assert_canary_incident_event(&fixture).unwrap();
}

#[test]
fn canary_incident_event_contract_fails_on_field_rename() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/canary.incident_event.v1.valid.json"
    ))
    .unwrap();
    let subject = fixture["subject"].as_object_mut().unwrap();
    let id = subject.remove("id").unwrap();
    subject.insert("incident_id".to_string(), id);

    let error = assert_canary_incident_event(&fixture).unwrap_err();
    assert!(error.contains("subject.id"), "{error}");
}

#[test]
fn canary_incident_event_contract_rejects_unknown_major_version() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/canary.incident_event.v1.valid.json"
    ))
    .unwrap();
    fixture["schema_version"] = Value::String("canary.incident_event.v2".to_string());

    let error = assert_canary_incident_event(&fixture).unwrap_err();
    assert!(
        error.contains("unsupported schema_version canary.incident_event.v2"),
        "{error}"
    );
}

#[test]
fn low_severity_canary_incident_drill_pins_normalization_contract() {
    let drill: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/canary.low_severity_incident_drill.v1.json"
    ))
    .unwrap();
    assert_eq!(
        drill["schema_version"],
        "bb.canary_low_severity_incident_drill.v1"
    );
    assert_eq!(
        drill["normalization"]["expected_incident_severity"],
        "medium"
    );
    assert_eq!(
        drill["normalization"]["derivation"],
        "active_correlated_signal_count"
    );
    assert_eq!(drill["normalization"]["high_threshold"], 3);
    assert_eq!(
        drill["normalization"]["fallback_incident_severity"],
        "medium"
    );
    assert_eq!(
        drill["normalization"]["propagates_originating_signal_severity_to_incident"],
        false
    );

    let timeline = drill["timeline"].as_array().unwrap();
    let error_event = timeline
        .iter()
        .find(|event| event["event"] == "error.new_class")
        .expect("error.new_class event");
    assert_eq!(error_event["severity"], "low");
    let opened_event = timeline
        .iter()
        .find(|event| event["event"] == "incident.opened")
        .expect("incident.opened event");
    assert_eq!(opened_event["severity"], "medium");

    assert_eq!(drill["incident_detail"]["incident"]["severity"], "medium");

    let webhook = &drill["webhook"];
    assert_canary_incident_event(webhook).unwrap();
    assert_eq!(webhook["signal"]["severity"], "low");
    assert_eq!(webhook["incident"]["severity"], "medium");
}

#[test]
fn canary_triage_report_fixture_is_report_only_and_actionable() {
    let report: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.canary_triage_report.v1.valid.json"
    ))
    .unwrap();
    assert_canary_triage_report(&report).unwrap();
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

fn assert_canary_incident_event(value: &Value) -> Result<(), String> {
    let schema_version = required_string(value, "schema_version")?;
    if schema_version != CANARY_INCIDENT_EVENT_SCHEMA_VERSION {
        return Err(format!("unsupported schema_version {schema_version}"));
    }
    let event = required_string(value, "event")?;
    if !matches!(
        event,
        "incident.opened" | "incident.updated" | "incident.resolved"
    ) {
        return Err(format!("unsupported event {event}"));
    }
    let subject = required_object(value, "subject")?;
    expect_string(subject, "type", "subject.type", "incident")?;
    required_string_at(subject, "id", "subject.id")?;
    required_string_at(subject, "service", "subject.service")?;

    let signal = required_object(value, "signal")?;
    required_string_at(signal, "kind", "signal.kind")?;
    required_string_at(signal, "fingerprint", "signal.fingerprint")?;
    required_string_at(signal, "severity", "signal.severity")?;
    required_string_at(signal, "observed_at", "signal.observed_at")?;

    let replay = required_object(value, "replay")?;
    required_string_at(replay, "timeline_url", "replay.timeline_url")?;
    required_string_at(replay, "report_url", "replay.report_url")?;
    required_string_at(replay, "incident_url", "replay.incident_url")?;
    Ok(())
}

fn assert_canary_triage_report(value: &Value) -> Result<(), String> {
    let schema = required_string(value, "schema")?;
    if schema != CANARY_TRIAGE_REPORT_SCHEMA_VERSION {
        return Err(format!("unsupported schema {schema}"));
    }
    let subject = required_object(value, "canary_subject")?;
    required_string_at(subject, "id", "canary_subject.id")?;
    required_string_at(subject, "service", "canary_subject.service")?;
    required_string_at(subject, "environment", "canary_subject.environment")?;
    required_string_at(subject, "severity", "canary_subject.severity")?;
    required_string_at(subject, "fingerprint", "canary_subject.fingerprint")?;
    required_string_at(subject, "observed_at", "canary_subject.observed_at")?;
    required_string(value, "delivery_id")?;
    required_string(value, "bb_run_id")?;
    required_string(value, "service")?;

    let repo = required_object(value, "repo")?;
    required_string_at(repo, "slug", "repo.slug")?;
    required_string_at(repo, "mapping_source", "repo.mapping_source")?;
    expect_bool(repo, "auto_merge", "repo.auto_merge", false)?;
    expect_bool(repo, "auto_deploy", "repo.auto_deploy", false)?;

    require_nonempty_array(value, "evidence")?;
    require_nonempty_array(value, "hypotheses")?;
    require_nonempty_array(value, "recommended_actions")?;
    require_nonempty_array(value, "residual_uncertainty")?;

    let constraints = required_object(value, "constraints")?;
    expect_bool(constraints, "report_only", "constraints.report_only", true)?;
    match constraints.get("mutations_performed") {
        Some(Value::Array(items)) if items.is_empty() => Ok(()),
        Some(other) => Err(format!(
            "constraints.mutations_performed must be empty array, got {other}"
        )),
        None => Err("missing constraints.mutations_performed".into()),
    }?;

    let actions = value["recommended_actions"].as_array().unwrap();
    if !actions.iter().any(|action| {
        action
            .get("command")
            .and_then(Value::as_str)
            .is_some_and(|command| command.contains("bb --config plane run canary-triage"))
    }) {
        return Err("recommended_actions must include a concrete BB replay command".into());
    }
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

fn require_nonempty_array(value: &Value, key: &str) -> Result<(), String> {
    match value.get(key) {
        Some(Value::Array(items)) if !items.is_empty() => Ok(()),
        Some(other) => Err(format!("{key} must be a non-empty array, got {other}")),
        None => Err(format!("missing array {key}")),
    }
}
