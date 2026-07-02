use std::io::Write as _;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};

use anyhow::{bail, Context, Result};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sha2::{Digest, Sha256};

use crate::ledger;
use crate::spec::{AuthClass, Plane, Task};

pub const MANAGEMENT_KEY_ENV: &str = "OPENROUTER_MANAGEMENT_KEY";
const BASE_URL_ENV: &str = "BB_OPENROUTER_KEYS_BASE_URL";
const DEFAULT_BASE_URL: &str = "https://openrouter.ai/api/v1";
const OPENROUTER_SECRET_ENV: &str = "OPENROUTER_API_KEY";
const PROVIDER: &str = "openrouter";
const SCHEMA_VERSION: u32 = 1;

#[derive(Clone, Serialize, Deserialize)]
pub struct StoredProviderKey {
    pub schema_version: u32,
    pub provider: String,
    pub agent: String,
    pub provider_key_name: String,
    pub name: String,
    pub hash: String,
    pub label: String,
    pub spend_cap_usd: f64,
    pub limit_remaining_usd: Option<f64>,
    pub limit_reset: Option<String>,
    pub usage_usd: f64,
    pub disabled: bool,
    pub created_at: String,
    pub updated_at: Option<String>,
    pub minted_at: String,
    pub revoked_at: Option<String>,
    pub api_key: String,
}

impl StoredProviderKey {
    fn active(&self) -> bool {
        self.provider == PROVIDER
            && self.revoked_at.is_none()
            && !self.disabled
            && !self.api_key.trim().is_empty()
    }

    fn view(&self) -> ProviderKeyView {
        ProviderKeyView {
            schema_version: self.schema_version,
            provider: self.provider.clone(),
            agent: self.agent.clone(),
            provider_key_name: self.provider_key_name.clone(),
            name: self.name.clone(),
            hash: self.hash.clone(),
            label: redact_secret_text(&self.label),
            spend_cap_usd: self.spend_cap_usd,
            limit_remaining_usd: self.limit_remaining_usd,
            limit_reset: self.limit_reset.clone(),
            usage_usd: self.usage_usd,
            disabled: self.disabled,
            revoked: self.revoked_at.is_some(),
            created_at: self.created_at.clone(),
            updated_at: self.updated_at.clone(),
            minted_at: self.minted_at.clone(),
            revoked_at: self.revoked_at.clone(),
            secret_available: self.active(),
            secret_fingerprint: self.active().then(|| fingerprint(&self.api_key)),
        }
    }
}

#[derive(Debug, Clone, Serialize)]
pub struct ProviderKeyView {
    pub schema_version: u32,
    pub provider: String,
    pub agent: String,
    pub provider_key_name: String,
    pub name: String,
    pub hash: String,
    pub label: String,
    pub spend_cap_usd: f64,
    pub limit_remaining_usd: Option<f64>,
    pub limit_reset: Option<String>,
    pub usage_usd: f64,
    pub disabled: bool,
    pub revoked: bool,
    pub created_at: String,
    pub updated_at: Option<String>,
    pub minted_at: String,
    pub revoked_at: Option<String>,
    pub secret_available: bool,
    pub secret_fingerprint: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct KeyOperationReport {
    pub operation: String,
    pub keys: Vec<ProviderKeyView>,
}

#[derive(Debug, Clone, Serialize)]
pub struct RemoteKeyView {
    pub provider: String,
    pub name: String,
    pub hash: String,
    pub label: String,
    pub limit: Option<f64>,
    pub limit_remaining: Option<f64>,
    pub limit_reset: Option<String>,
    pub usage: f64,
    pub disabled: bool,
    pub created_at: String,
    pub updated_at: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
struct OpenRouterKeyEnvelope {
    data: OpenRouterKeyData,
    key: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
struct OpenRouterKeyList {
    data: Vec<OpenRouterKeyData>,
}

#[derive(Debug, Clone, Deserialize)]
struct OpenRouterKeyData {
    hash: String,
    name: String,
    #[serde(default)]
    label: String,
    #[serde(default)]
    limit: Option<f64>,
    #[serde(default)]
    limit_remaining: Option<f64>,
    #[serde(default)]
    limit_reset: Option<String>,
    #[serde(default)]
    usage: f64,
    #[serde(default)]
    disabled: bool,
    #[serde(default)]
    created_at: String,
    #[serde(default)]
    updated_at: Option<String>,
}

impl OpenRouterKeyData {
    fn remote_view(&self) -> RemoteKeyView {
        RemoteKeyView {
            provider: PROVIDER.into(),
            name: self.name.clone(),
            hash: self.hash.clone(),
            label: redact_secret_text(&self.label),
            limit: self.limit,
            limit_remaining: self.limit_remaining,
            limit_reset: self.limit_reset.clone(),
            usage: self.usage,
            disabled: self.disabled,
            created_at: self.created_at.clone(),
            updated_at: self.updated_at.clone(),
        }
    }
}

pub fn eligible_agents(plane: &Plane) -> Vec<String> {
    plane
        .agents
        .iter()
        .filter_map(|(name, agent)| {
            if agent.provider() == PROVIDER && agent.policy.provider_key_name.is_some() {
                Some(name.clone())
            } else {
                None
            }
        })
        .collect()
}

pub fn mint_agent(plane: &Plane, agent_name: &str) -> Result<ProviderKeyView> {
    let target = key_target(plane, agent_name)?;
    if let Some(existing) = read_stored_key(plane, agent_name)? {
        if existing.active() {
            bail!(
                "agent '{agent_name}' already has an active {PROVIDER} key {}; use `bb keys rotate {agent_name}`",
                existing.hash
            );
        }
    }
    let client = OpenRouterClient::from_env()?;
    let name = key_display_name(plane, agent_name, &target.provider_key_name);
    let created = client.create_key(&name, target.spend_cap_usd)?;
    let raw_key = created
        .key
        .context("OpenRouter create response did not include the one-time plaintext key")?;
    let record = stored_from_create(agent_name, &target, created.data, raw_key);
    write_stored_key(plane, &record)?;
    Ok(record.view())
}

pub fn rotate_agent(plane: &Plane, agent_name: &str) -> Result<KeyOperationReport> {
    let old = read_stored_key(plane, agent_name)?;
    if let Some(old) = &old {
        if !old.active() {
            bail!("agent '{agent_name}' has no active key to rotate");
        }
    } else {
        bail!("agent '{agent_name}' has no stored key to rotate; use `bb keys mint {agent_name}`");
    }
    let old = old.expect("checked");
    let target = key_target(plane, agent_name)?;
    let client = OpenRouterClient::from_env()?;
    let name = key_display_name(plane, agent_name, &target.provider_key_name);
    let created = client.create_key(&name, target.spend_cap_usd)?;
    let raw_key = created
        .key
        .context("OpenRouter create response did not include the one-time plaintext key")?;
    let new_record = stored_from_create(agent_name, &target, created.data, raw_key);
    write_stored_key(plane, &new_record)?;
    client.delete_key(&old.hash)?;
    Ok(KeyOperationReport {
        operation: "rotate".into(),
        keys: vec![new_record.view()],
    })
}

pub fn revoke_agent(plane: &Plane, agent_name: &str) -> Result<ProviderKeyView> {
    let mut record = read_stored_key(plane, agent_name)?
        .with_context(|| format!("agent '{agent_name}' has no stored provider key"))?;
    if !record.active() {
        bail!("agent '{agent_name}' has no active key to revoke");
    }
    OpenRouterClient::from_env()?.delete_key(&record.hash)?;
    record.disabled = true;
    record.revoked_at = Some(ledger::now());
    record.api_key.clear();
    write_stored_key(plane, &record)?;
    Ok(record.view())
}

pub fn list_local(plane: &Plane) -> Result<Vec<ProviderKeyView>> {
    let dir = store_dir(plane);
    if !dir.is_dir() {
        return Ok(Vec::new());
    }
    let mut rows = Vec::new();
    for entry in std::fs::read_dir(dir)? {
        let path = entry?.path();
        if path.extension().and_then(|e| e.to_str()) != Some("json") {
            continue;
        }
        let record: StoredProviderKey = serde_json::from_str(&std::fs::read_to_string(&path)?)
            .with_context(|| format!("parse provider key record {}", path.display()))?;
        rows.push(record.view());
    }
    rows.sort_by(|a, b| a.agent.cmp(&b.agent));
    Ok(rows)
}

pub fn list_remote(include_disabled: bool) -> Result<Vec<RemoteKeyView>> {
    let rows = OpenRouterClient::from_env()?.list_keys(include_disabled)?;
    Ok(rows.into_iter().map(|r| r.remote_view()).collect())
}

pub fn resolve_secret_for_task(
    plane: &Plane,
    task: &Task,
    secret_name: &str,
) -> Result<Option<String>> {
    if secret_name != OPENROUTER_SECRET_ENV
        || task.agent.provider() != PROVIDER
        || task.agent.policy.provider_key_name.is_none()
    {
        return Ok(None);
    }
    let target = key_target_for_agent(&task.agent_name, &task.agent)?;
    let record = read_stored_key(plane, &task.agent_name)?.with_context(|| {
        format!(
            "agent '{}' has policy.provider_key_name '{}' but no stored {PROVIDER} key; run `bb --config {} keys mint {}`",
            task.agent_name,
            target.provider_key_name,
            plane.root.display(),
            task.agent_name
        )
    })?;
    if record.provider_key_name != target.provider_key_name {
        bail!(
            "agent '{}' stored key is for provider_key_name '{}' but policy wants '{}'; rotate the key",
            task.agent_name,
            record.provider_key_name,
            target.provider_key_name
        );
    }
    if (record.spend_cap_usd - target.spend_cap_usd).abs() > f64::EPSILON {
        bail!(
            "agent '{}' stored key cap ${:.4} does not match policy cap ${:.4}; rotate the key",
            task.agent_name,
            record.spend_cap_usd,
            target.spend_cap_usd
        );
    }
    if !record.active() {
        bail!(
            "agent '{}' stored {PROVIDER} key is revoked or disabled",
            task.agent_name
        );
    }
    Ok(Some(record.api_key))
}

fn stored_from_create(
    agent_name: &str,
    target: &KeyTarget,
    data: OpenRouterKeyData,
    api_key: String,
) -> StoredProviderKey {
    StoredProviderKey {
        schema_version: SCHEMA_VERSION,
        provider: PROVIDER.into(),
        agent: agent_name.into(),
        provider_key_name: target.provider_key_name.clone(),
        name: data.name,
        hash: data.hash,
        label: data.label,
        spend_cap_usd: data.limit.unwrap_or(target.spend_cap_usd),
        limit_remaining_usd: data.limit_remaining,
        limit_reset: data.limit_reset,
        usage_usd: data.usage,
        disabled: data.disabled,
        created_at: data.created_at,
        updated_at: data.updated_at,
        minted_at: ledger::now(),
        revoked_at: None,
        api_key,
    }
}

struct KeyTarget {
    provider_key_name: String,
    spend_cap_usd: f64,
}

fn key_target(plane: &Plane, agent_name: &str) -> Result<KeyTarget> {
    let agent = plane
        .agents
        .get(agent_name)
        .with_context(|| format!("unknown agent '{agent_name}'"))?;
    key_target_for_agent(agent_name, agent)
}

fn key_target_for_agent(agent_name: &str, agent: &crate::spec::AgentSpec) -> Result<KeyTarget> {
    if agent.provider() != PROVIDER {
        bail!(
            "agent '{agent_name}' provider is '{}'; only {PROVIDER} keys are supported in this slice",
            agent.provider()
        );
    }
    if agent.auth_class()? != AuthClass::Api {
        bail!("agent '{agent_name}' is not API-auth; scoped provider keys are for API-auth agents");
    }
    let provider_key_name = agent
        .policy
        .provider_key_name
        .clone()
        .with_context(|| format!("agent '{agent_name}' has no policy.provider_key_name"))?;
    let spend_cap_usd = agent
        .policy
        .provider_spend_cap_usd
        .with_context(|| format!("agent '{agent_name}' has no policy.provider_spend_cap_usd"))?;
    if !spend_cap_usd.is_finite() || spend_cap_usd <= 0.0 {
        bail!(
            "agent '{agent_name}' policy.provider_spend_cap_usd must be a positive finite number"
        );
    }
    Ok(KeyTarget {
        provider_key_name,
        spend_cap_usd,
    })
}

fn key_display_name(plane: &Plane, agent_name: &str, provider_key_name: &str) -> String {
    let plane_name = plane
        .root
        .file_name()
        .and_then(|s| s.to_str())
        .unwrap_or("plane");
    format!(
        "bb:{plane_name}:{agent_name}:{provider_key_name}:{}",
        ledger::now()
    )
}

fn read_stored_key(plane: &Plane, agent_name: &str) -> Result<Option<StoredProviderKey>> {
    let path = key_path(plane, agent_name);
    if !path.exists() {
        return Ok(None);
    }
    let raw = std::fs::read_to_string(&path)
        .with_context(|| format!("read provider key record {}", path.display()))?;
    Ok(Some(serde_json::from_str(&raw).with_context(|| {
        format!("parse provider key record {}", path.display())
    })?))
}

fn write_stored_key(plane: &Plane, record: &StoredProviderKey) -> Result<()> {
    let dir = store_dir(plane);
    std::fs::create_dir_all(&dir)?;
    set_mode(&dir, 0o700)?;
    let path = key_path(plane, &record.agent);
    let content = serde_json::to_vec_pretty(record)?;
    let mut file = std::fs::OpenOptions::new()
        .create(true)
        .truncate(true)
        .write(true)
        .open(&path)
        .with_context(|| format!("write provider key record {}", path.display()))?;
    file.write_all(&content)?;
    file.sync_all()?;
    set_mode(&path, 0o600)?;
    Ok(())
}

fn store_dir(plane: &Plane) -> PathBuf {
    plane.root.join(".bb/provider-keys/openrouter")
}

fn key_path(plane: &Plane, agent_name: &str) -> PathBuf {
    store_dir(plane).join(format!("{}.json", path_segment(agent_name)))
}

fn path_segment(value: &str) -> String {
    value
        .chars()
        .map(|c| {
            if c.is_ascii_alphanumeric() || c == '-' || c == '_' || c == '.' {
                c
            } else {
                '_'
            }
        })
        .collect()
}

fn set_mode(path: &Path, mode: u32) -> Result<()> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::set_permissions(path, std::fs::Permissions::from_mode(mode))?;
    }
    #[cfg(not(unix))]
    let _ = (path, mode);
    Ok(())
}

struct OpenRouterClient {
    base_url: String,
    management_key: String,
}

impl OpenRouterClient {
    fn from_env() -> Result<Self> {
        let management_key = std::env::var(MANAGEMENT_KEY_ENV)
            .with_context(|| format!("{MANAGEMENT_KEY_ENV} is not set"))?;
        if management_key.trim().is_empty() {
            bail!("{MANAGEMENT_KEY_ENV} is blank");
        }
        let base_url = std::env::var(BASE_URL_ENV).unwrap_or_else(|_| DEFAULT_BASE_URL.into());
        Ok(Self {
            base_url: base_url.trim_end_matches('/').to_string(),
            management_key,
        })
    }

    fn create_key(&self, name: &str, limit: f64) -> Result<OpenRouterKeyEnvelope> {
        let body = serde_json::json!({
            "name": name,
            "limit": limit,
            "include_byok_in_limit": false,
        });
        let response = self.request_json("POST", "/keys", Some(&body))?;
        serde_json::from_str(&response).context("parse OpenRouter create key response")
    }

    fn list_keys(&self, include_disabled: bool) -> Result<Vec<OpenRouterKeyData>> {
        let path = if include_disabled {
            "/keys?include_disabled=true"
        } else {
            "/keys"
        };
        let response = self.request_json("GET", path, None)?;
        let list: OpenRouterKeyList =
            serde_json::from_str(&response).context("parse OpenRouter key list response")?;
        Ok(list.data)
    }

    fn delete_key(&self, hash: &str) -> Result<()> {
        let path = format!("/keys/{hash}");
        let response = self.request_json("DELETE", &path, None)?;
        if response.trim().is_empty() {
            return Ok(());
        }
        let value: Value = serde_json::from_str(&response).unwrap_or(Value::Null);
        if value
            .get("success")
            .and_then(Value::as_bool)
            .unwrap_or(true)
        {
            Ok(())
        } else {
            bail!("OpenRouter delete returned success=false")
        }
    }

    fn request_json(&self, method: &str, path: &str, body: Option<&Value>) -> Result<String> {
        let url = format!("{}{}", self.base_url, path);
        let body_path = if let Some(body) = body {
            let path = std::env::temp_dir().join(format!(
                "bb-openrouter-body-{}-{}.json",
                std::process::id(),
                ledger::new_id()
            ));
            std::fs::write(&path, serde_json::to_vec(body)?)?;
            Some(path)
        } else {
            None
        };
        let mut config = format!(
            "url = \"{}\"\nrequest = \"{}\"\nheader = \"Authorization: Bearer {}\"\nnoproxy = \"*\"\nconnect-timeout = 20\nmax-time = 90\n",
            curl_value(&url),
            curl_value(method),
            curl_value(&self.management_key)
        );
        if let Some(path) = &body_path {
            config.push_str("header = \"Content-Type: application/json\"\n");
            config.push_str(&format!(
                "data-binary = \"@{}\"\n",
                curl_value(&path.display().to_string())
            ));
        }

        let mut child = Command::new("curl")
            .args([
                "--silent",
                "--show-error",
                "--fail-with-body",
                "--config",
                "-",
            ])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .context("spawn curl for OpenRouter key management request")?;
        {
            let mut stdin = child.stdin.take().context("curl stdin")?;
            stdin.write_all(config.as_bytes())?;
        }
        let output = child.wait_with_output()?;
        if let Some(path) = body_path {
            let _ = std::fs::remove_file(path);
        }
        if !output.status.success() {
            let stderr = redact_secret_text(&String::from_utf8_lossy(&output.stderr));
            let stdout = redact_secret_text(&String::from_utf8_lossy(&output.stdout));
            bail!(
                "OpenRouter key management request failed (status={}): {}{}{}",
                output.status,
                stderr.trim(),
                if stderr.trim().is_empty() || stdout.trim().is_empty() {
                    ""
                } else {
                    " "
                },
                stdout.trim()
            );
        }
        String::from_utf8(output.stdout).context("OpenRouter response was not UTF-8")
    }
}

fn curl_value(value: &str) -> String {
    value.replace('\\', "\\\\").replace('"', "\\\"")
}

fn fingerprint(secret: &str) -> String {
    let digest = Sha256::digest(secret.as_bytes());
    format!(
        "sha256:{}",
        digest[..6]
            .iter()
            .map(|b| format!("{b:02x}"))
            .collect::<String>()
    )
}

fn redact_secret_text(input: &str) -> String {
    let marker = "sk-or-v1-";
    let mut out = String::with_capacity(input.len());
    let mut rest = input;
    while let Some(idx) = rest.find(marker) {
        out.push_str(&rest[..idx]);
        out.push_str("sk-or-v1-[redacted]");
        let after_marker = idx + marker.len();
        let tail = &rest[after_marker..];
        let consumed = tail
            .char_indices()
            .find_map(|(i, c)| (!(c.is_ascii_alphanumeric() || c == '-' || c == '_')).then_some(i))
            .unwrap_or(tail.len());
        rest = &tail[consumed..];
    }
    out.push_str(rest);
    out
}
