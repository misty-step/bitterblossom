use serde_json::Value;

const BB_NOTIFICATION_EVENT_SCHEMA_VERSION: &str = "bb.notification_event.v1";

#[test]
fn bb_notification_event_fixture_matches_owned_contract() {
    let schema: Value = serde_json::from_str(include_str!(
        "../docs/schemas/bb.notification_event.v1.schema.json"
    ))
    .unwrap();
    assert_eq!(
        schema["properties"]["schema_version"]["const"],
        BB_NOTIFICATION_EVENT_SCHEMA_VERSION
    );
    assert_schema_requires(&schema, &["schema_version", "event"]);

    let fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.notification_event.v1.valid.json"
    ))
    .unwrap();
    assert_bb_notification_event(&fixture).unwrap();
}

#[test]
fn bb_notification_event_contract_rejects_unknown_major_version() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/bb.notification_event.v1.valid.json"
    ))
    .unwrap();
    fixture["schema_version"] = Value::String("bb.notification_event.v2".to_string());

    let error = assert_bb_notification_event(&fixture).unwrap_err();
    assert!(
        error.contains("unsupported schema_version bb.notification_event.v2"),
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

fn assert_bb_notification_event(value: &Value) -> Result<(), String> {
    let schema_version = required_string(value, "schema_version")?;
    if schema_version != BB_NOTIFICATION_EVENT_SCHEMA_VERSION {
        return Err(format!("unsupported schema_version {schema_version}"));
    }
    required_string(value, "event")?;
    Ok(())
}

fn required_string<'a>(value: &'a Value, key: &str) -> Result<&'a str, String> {
    match value.get(key).and_then(Value::as_str) {
        Some(value) if !value.is_empty() => Ok(value),
        _ => Err(format!("missing string {key}")),
    }
}
