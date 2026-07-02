use serde_json::Value;

const CANARY_INCIDENT_EVENT_SCHEMA_VERSION: &str = "canary.incident_event.v1";

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
