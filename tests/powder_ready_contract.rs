//! Contract pin for Powder's `powder.card_event.v1` webhook envelope
//! (bitterblossom-931 pilot a). Powder signs deliveries with
//! `X-Signature-256: sha256=hex(hmac(signing_secret, raw_body))` and no
//! delivery-id header, so the dispatch dedupe key is the payload's own
//! `event_id` (see tests/ingress.rs `webhook_accepts_powder_ready_event_*`).

use serde_json::Value;

const POWDER_CARD_EVENT_SCHEMA_VERSION: &str = "powder.card_event.v1";

#[test]
fn powder_card_event_fixture_matches_pinned_contract() {
    let schema: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/powder.card_event.v1.schema.json"
    ))
    .unwrap();
    assert_eq!(
        schema["properties"]["schema_version"]["const"],
        POWDER_CARD_EVENT_SCHEMA_VERSION
    );
    assert_schema_requires(
        &schema,
        &[
            "schema_version",
            "event_id",
            "event_type",
            "occurred_at",
            "actor",
            "card",
            "change",
        ],
    );

    let fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/powder.card_event.v1.valid.json"
    ))
    .unwrap();
    assert_powder_card_event(&fixture).unwrap();
}

#[test]
fn powder_card_event_contract_fails_on_field_rename() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/powder.card_event.v1.valid.json"
    ))
    .unwrap();
    let event_type = fixture
        .as_object_mut()
        .unwrap()
        .remove("event_type")
        .unwrap();
    fixture
        .as_object_mut()
        .unwrap()
        .insert("kind".to_string(), event_type);

    let error = assert_powder_card_event(&fixture).unwrap_err();
    assert!(error.contains("event_type"), "{error}");
}

#[test]
fn powder_card_event_contract_rejects_unknown_major_version() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/powder.card_event.v1.valid.json"
    ))
    .unwrap();
    fixture["schema_version"] = Value::String("powder.card_event.v2".to_string());

    let error = assert_powder_card_event(&fixture).unwrap_err();
    assert!(
        error.contains("unsupported schema_version powder.card_event.v2"),
        "{error}"
    );
}

#[test]
fn powder_card_event_contract_rejects_unknown_event_type() {
    let mut fixture: Value = serde_json::from_str(include_str!(
        "fixtures/contracts/powder.card_event.v1.valid.json"
    ))
    .unwrap();
    fixture["event_type"] = Value::String("card-deleted".to_string());

    let error = assert_powder_card_event(&fixture).unwrap_err();
    assert!(
        error.contains("unsupported event_type card-deleted"),
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

fn assert_powder_card_event(value: &Value) -> Result<(), String> {
    let schema_version = required_string(value, "schema_version")?;
    if schema_version != POWDER_CARD_EVENT_SCHEMA_VERSION {
        return Err(format!("unsupported schema_version {schema_version}"));
    }
    required_string(value, "event_id")?;
    let event_type = required_string(value, "event_type")?;
    if !matches!(
        event_type,
        "card-created"
            | "moved-to-ready"
            | "awaiting-input"
            | "claim-expired"
            | "completed"
            | "comment-added"
    ) {
        return Err(format!("unsupported event_type {event_type}"));
    }
    if value.get("occurred_at").and_then(Value::as_i64).is_none() {
        return Err("missing integer occurred_at".into());
    }
    required_string(value, "actor")?;

    let card = required_object(value, "card")?;
    required_string_at(card, "id", "card.id")?;
    required_string_at(card, "status", "card.status")?;
    required_string_at(card, "repo", "card.repo")?;

    required_object(value, "change")?;
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
