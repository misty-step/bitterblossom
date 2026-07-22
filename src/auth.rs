//! Authenticated principal and workflow operation policy.
//!
//! Transport credentials establish identity. Semantic payload labels are
//! metadata and can never select a role or widen a grant.

use std::fmt;
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use subtle::ConstantTimeEq;
use time::{format_description::well_known::Rfc3339, OffsetDateTime};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PrincipalRole {
    Admin,
    Operator,
    Worker,
    Controller,
}

impl PrincipalRole {
    pub fn as_str(self) -> &'static str {
        match self {
            Self::Admin => "admin",
            Self::Operator => "operator",
            Self::Worker => "worker",
            Self::Controller => "controller",
        }
    }

    pub fn parse(raw: &str) -> Result<Self, AuthError> {
        match raw.trim().to_ascii_lowercase().as_str() {
            "admin" | "administrator" => Ok(Self::Admin),
            "operator" => Ok(Self::Operator),
            "worker" | "agent" => Ok(Self::Worker),
            "controller" => Ok(Self::Controller),
            _ => Err(AuthError::new(
                DenialClass::Unauthenticated,
                format!("unknown principal role '{raw}'"),
            )),
        }
    }
}

/// Exhaustive workflow mutation vocabulary. Read operations do not enter this
/// matrix; every write-capable face chooses one of these variants.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum Operation {
    CreateWorkflow,
    ReviseWorkflow,
    ImportWorkflow,
    ActivateWorkflow,
    PauseWorkflow,
    ResumeWorkflow,
    ArchiveWorkflow,
    RollbackWorkflow,
    AcceptWorkflowRun,
    ResolveWorkflowRun,
    ExecuteWorkflowRun,
    StopWorkflowRun,
    RaiseAsk,
    AnswerAsk,
    DispatchRun,
    RecordEffect,
    CorrectWorkflow,
}

impl Operation {
    pub const ALL: [Self; 17] = [
        Self::CreateWorkflow,
        Self::ReviseWorkflow,
        Self::ImportWorkflow,
        Self::ActivateWorkflow,
        Self::PauseWorkflow,
        Self::ResumeWorkflow,
        Self::ArchiveWorkflow,
        Self::RollbackWorkflow,
        Self::AcceptWorkflowRun,
        Self::ResolveWorkflowRun,
        Self::ExecuteWorkflowRun,
        Self::StopWorkflowRun,
        Self::RaiseAsk,
        Self::AnswerAsk,
        Self::DispatchRun,
        Self::RecordEffect,
        Self::CorrectWorkflow,
    ];

    pub fn as_str(self) -> &'static str {
        match self {
            Self::CreateWorkflow => "create_workflow",
            Self::ReviseWorkflow => "revise_workflow",
            Self::ImportWorkflow => "import_workflow",
            Self::ActivateWorkflow => "activate_workflow",
            Self::PauseWorkflow => "pause_workflow",
            Self::ResumeWorkflow => "resume_workflow",
            Self::ArchiveWorkflow => "archive_workflow",
            Self::RollbackWorkflow => "rollback_workflow",
            Self::AcceptWorkflowRun => "accept_workflow_run",
            Self::ResolveWorkflowRun => "resolve_workflow_run",
            Self::ExecuteWorkflowRun => "execute_workflow_run",
            Self::StopWorkflowRun => "stop_workflow_run",
            Self::RaiseAsk => "raise_ask",
            Self::AnswerAsk => "answer_ask",
            Self::DispatchRun => "dispatch_run",
            Self::RecordEffect => "record_effect",
            Self::CorrectWorkflow => "correct_workflow",
        }
    }

    pub const fn rule(self) -> OperationRule {
        use OperationCapability::{Correction, Execution, Workflow};
        match self {
            Self::CreateWorkflow
            | Self::ReviseWorkflow
            | Self::ImportWorkflow
            | Self::ActivateWorkflow
            | Self::PauseWorkflow
            | Self::ResumeWorkflow
            | Self::ArchiveWorkflow
            | Self::RollbackWorkflow
            | Self::CorrectWorkflow => OperationRule {
                capability: Workflow,
                holder_required: false,
                operator_allowed: true,
                controller_allowed: false,
            },
            Self::AcceptWorkflowRun => OperationRule {
                capability: Execution,
                holder_required: false,
                operator_allowed: true,
                controller_allowed: true,
            },
            Self::ResolveWorkflowRun | Self::StopWorkflowRun => OperationRule {
                capability: Correction,
                holder_required: false,
                operator_allowed: true,
                controller_allowed: true,
            },
            Self::ExecuteWorkflowRun => OperationRule {
                capability: Execution,
                holder_required: true,
                operator_allowed: true,
                controller_allowed: true,
            },
            Self::RecordEffect => OperationRule {
                capability: Execution,
                holder_required: true,
                operator_allowed: false,
                controller_allowed: true,
            },
            Self::RaiseAsk => OperationRule {
                capability: Execution,
                holder_required: true,
                operator_allowed: false,
                controller_allowed: true,
            },
            Self::AnswerAsk => OperationRule {
                capability: Execution,
                holder_required: true,
                operator_allowed: true,
                controller_allowed: true,
            },
            Self::DispatchRun => OperationRule {
                capability: Execution,
                holder_required: false,
                operator_allowed: true,
                controller_allowed: true,
            },
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum OperationCapability {
    Workflow,
    Correction,
    Execution,
}

impl OperationCapability {
    pub const fn allows(self, role: PrincipalRole) -> bool {
        matches!(
            (self, role),
            (_, PrincipalRole::Admin)
                | (Self::Workflow | Self::Correction, PrincipalRole::Operator)
                | (
                    Self::Execution,
                    PrincipalRole::Operator | PrincipalRole::Worker | PrincipalRole::Controller,
                )
        )
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct OperationRule {
    pub capability: OperationCapability,
    pub holder_required: bool,
    pub operator_allowed: bool,
    pub controller_allowed: bool,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum DenialClass {
    Unauthenticated,
    Capability,
    ClaimRequired,
    ClaimExpired,
    IdentityMismatch,
    CrossResource,
}

impl DenialClass {
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Unauthenticated => "unauthenticated",
            Self::Capability => "capability",
            Self::ClaimRequired => "claim_required",
            Self::ClaimExpired => "claim_expired",
            Self::IdentityMismatch => "identity_mismatch",
            Self::CrossResource => "cross_resource",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AuthError {
    pub class: DenialClass,
    pub operation: Option<Operation>,
    pub detail: String,
}

impl AuthError {
    pub fn new(class: DenialClass, detail: impl Into<String>) -> Self {
        Self {
            class,
            operation: None,
            detail: detail.into(),
        }
    }

    pub fn for_operation(
        class: DenialClass,
        operation: Operation,
        detail: impl Into<String>,
    ) -> Self {
        Self {
            class,
            operation: Some(operation),
            detail: detail.into(),
        }
    }

    pub fn denial_class(&self) -> &'static str {
        self.class.as_str()
    }
}

impl fmt::Display for AuthError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.detail)?;
        write!(f, " [denial_class={}]", self.class.as_str())
    }
}
impl std::error::Error for AuthError {}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct AuthContext {
    pub principal: String,
    pub role: PrincipalRole,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub claim_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub run_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub transport: Option<String>,
}

impl AuthContext {
    pub fn new(principal: impl Into<String>, role: PrincipalRole) -> Self {
        Self {
            principal: principal.into(),
            role,
            claim_id: None,
            run_id: None,
            transport: None,
        }
    }

    pub fn worker(
        principal: impl Into<String>,
        run_id: impl Into<String>,
        claim_id: impl Into<String>,
    ) -> Self {
        Self {
            principal: principal.into(),
            role: PrincipalRole::Worker,
            claim_id: Some(claim_id.into()),
            run_id: Some(run_id.into()),
            transport: Some("worker".into()),
        }
    }

    pub fn operator(principal: impl Into<String>) -> Self {
        Self::new(principal, PrincipalRole::Operator)
    }
    pub fn admin(principal: impl Into<String>) -> Self {
        Self::new(principal, PrincipalRole::Admin)
    }
    pub fn controller(principal: impl Into<String>) -> Self {
        Self::new(principal, PrincipalRole::Controller)
    }

    pub fn is_authenticated(&self) -> bool {
        !self.principal.trim().is_empty()
    }

    pub fn with_transport(mut self, transport: impl Into<String>) -> Self {
        self.transport = Some(transport.into());
        self
    }

    pub fn with_run(mut self, run_id: impl Into<String>, claim_id: impl Into<String>) -> Self {
        self.run_id = Some(run_id.into());
        self.claim_id = Some(claim_id.into());
        self
    }

    /// Authenticate the already-validated bearer token. The configured
    /// principal and role are server configuration, never request text.
    pub fn from_http_bearer(
        presented: Option<&str>,
        configured: Option<&str>,
    ) -> Result<Self, AuthError> {
        match configured {
            Some(expected) if presented.is_some_and(|value| constant_time_eq(value, expected)) => {
                let principal =
                    std::env::var("BB_API_PRINCIPAL").unwrap_or_else(|_| "operator".into());
                let role = PrincipalRole::parse(
                    &std::env::var("BB_API_ROLE").unwrap_or_else(|_| "operator".into()),
                )?;
                Ok(Self::new(principal, role).with_transport("http"))
            }
            Some(_) => Err(AuthError::new(
                DenialClass::Unauthenticated,
                "missing or bad bearer token",
            )),
            None => Ok(Self::operator("operator").with_transport("trusted-local-http")),
        }
    }

    /// CLI and stdio MCP are local transports. Explicit environment identity
    /// is accepted only from this trusted local path; no payload label reaches
    /// this constructor. The default is intentionally limited to no-auth mode.
    pub fn trusted_local() -> Result<Self, AuthError> {
        let principal = std::env::var("BB_TRUSTED_PRINCIPAL").ok();
        let role = std::env::var("BB_TRUSTED_ROLE").ok();
        match (principal, role) {
            (Some(principal), Some(role)) => {
                Ok(Self::new(principal, PrincipalRole::parse(&role)?)
                    .with_transport("trusted-local"))
            }
            (None, None) if std::env::var("BB_API_TOKEN").is_err() => {
                Ok(Self::operator("operator").with_transport("trusted-local-default"))
            }
            (None, None) => Err(AuthError::new(
                DenialClass::Unauthenticated,
                "trusted local principal is not configured",
            )),
            _ => Err(AuthError::new(
                DenialClass::Unauthenticated,
                "BB_TRUSTED_PRINCIPAL and BB_TRUSTED_ROLE must be configured together",
            )),
        }
    }

    pub fn authorize(
        &self,
        operation: Operation,
        resource: &AuthorizationResource,
        lease: Option<&LiveLease>,
    ) -> Result<(), AuthError> {
        if !self.is_authenticated() {
            return Err(AuthError::for_operation(
                DenialClass::Unauthenticated,
                operation,
                "authenticated principal required",
            ));
        }
        let rule = operation.rule();
        if !rule.capability.allows(self.role) {
            return Err(AuthError::for_operation(
                DenialClass::Capability,
                operation,
                format!(
                    "{} authority cannot perform {}",
                    self.role.as_str(),
                    operation.as_str()
                ),
            ));
        }
        if self.role == PrincipalRole::Operator && !rule.operator_allowed {
            return Err(AuthError::for_operation(
                DenialClass::Capability,
                operation,
                format!("operator cannot perform {}", operation.as_str()),
            ));
        }
        if self.role == PrincipalRole::Controller && !rule.controller_allowed {
            return Err(AuthError::for_operation(
                DenialClass::Capability,
                operation,
                format!("controller cannot perform {}", operation.as_str()),
            ));
        }
        if self.role == PrincipalRole::Admin
            || (self.role == PrincipalRole::Operator && rule.operator_allowed)
            || (self.role == PrincipalRole::Controller && rule.controller_allowed)
        {
            return Ok(());
        }
        if !rule.holder_required {
            return Ok(());
        }
        let Some(run_id) = resource.run_id.as_deref() else {
            return Err(AuthError::for_operation(
                DenialClass::ClaimRequired,
                operation,
                "current run claim required",
            ));
        };
        let Some(lease) = lease else {
            return Err(AuthError::for_operation(
                DenialClass::ClaimRequired,
                operation,
                "current run lease required",
            ));
        };
        let Some(context_run_id) = self.run_id.as_deref() else {
            return Err(AuthError::for_operation(
                DenialClass::ClaimRequired,
                operation,
                "authenticated worker run claim required",
            ));
        };
        if context_run_id != run_id {
            return Err(AuthError::for_operation(
                DenialClass::CrossResource,
                operation,
                "principal context targets another run",
            ));
        }
        if lease.run_id != run_id {
            return Err(AuthError::for_operation(
                DenialClass::CrossResource,
                operation,
                "operation targets another run",
            ));
        }
        if lease.holder_principal != self.principal {
            return Err(AuthError::for_operation(
                DenialClass::IdentityMismatch,
                operation,
                "principal does not hold the current run lease",
            ));
        }
        if resource.claim_id.as_deref() != Some(lease.claim_id.as_str())
            || self.claim_id.as_deref() != Some(lease.claim_id.as_str())
        {
            return Err(AuthError::for_operation(
                DenialClass::IdentityMismatch,
                operation,
                "claim identity does not match the current run lease",
            ));
        }
        if lease.is_expired() {
            return Err(AuthError::for_operation(
                DenialClass::ClaimExpired,
                operation,
                "current run lease is expired",
            ));
        }
        Ok(())
    }

    pub fn require_semantic_principal(&self, supplied: Option<&str>) -> Result<(), AuthError> {
        match supplied {
            Some(value) if value == self.principal => Ok(()),
            Some(_) => Err(AuthError::new(
                DenialClass::IdentityMismatch,
                "semantic principal does not match authenticated principal",
            )),
            None => Err(AuthError::new(
                DenialClass::IdentityMismatch,
                "semantic principal is required",
            )),
        }
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct AuthorizationResource {
    pub workflow: Option<String>,
    pub run_id: Option<String>,
    pub claim_id: Option<String>,
}

impl AuthorizationResource {
    pub fn workflow(name: impl Into<String>) -> Self {
        Self {
            workflow: Some(name.into()),
            ..Self::default()
        }
    }
    pub fn run(run_id: impl Into<String>) -> Self {
        Self {
            run_id: Some(run_id.into()),
            ..Self::default()
        }
    }
    pub fn run_with_claim(run_id: impl Into<String>, claim_id: impl Into<String>) -> Self {
        Self {
            run_id: Some(run_id.into()),
            claim_id: Some(claim_id.into()),
            ..Self::default()
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LiveLease {
    pub run_id: String,
    pub claim_id: String,
    pub holder_principal: String,
    pub expires_at: String,
}

impl LiveLease {
    pub fn is_expired(&self) -> bool {
        OffsetDateTime::parse(&self.expires_at, &Rfc3339)
            .map(|until| until <= OffsetDateTime::now_utc())
            .unwrap_or(true)
    }
}

pub fn constant_time_eq(left: &str, right: &str) -> bool {
    left.len() == right.len() && left.as_bytes().ct_eq(right.as_bytes()).into()
}

pub fn unix_now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn live(run: &str, principal: &str, claim: &str, expires_at: &str) -> LiveLease {
        LiveLease {
            run_id: run.into(),
            holder_principal: principal.into(),
            claim_id: claim.into(),
            expires_at: expires_at.into(),
        }
    }

    #[test]
    fn operation_matrix_is_exhaustive_and_closed() {
        assert_eq!(Operation::ALL.len(), 17);
        for operation in Operation::ALL {
            let _ = operation.rule();
            assert!(!operation.as_str().is_empty());
        }
        assert!(Operation::CreateWorkflow.rule().operator_allowed);
        assert!(!Operation::RecordEffect.rule().operator_allowed);
        assert!(Operation::ExecuteWorkflowRun.rule().holder_required);
    }

    #[test]
    fn holder_matrix_has_stable_denials_and_no_payload_impersonation() {
        let future = "2999-01-01T00:00:00Z";
        let expired = "2000-01-01T00:00:00Z";
        let resource = AuthorizationResource::run_with_claim("run-1", "claim-1");
        let cases = [
            (
                "holder",
                AuthContext::worker("worker-a", "run-1", "claim-1"),
                live("run-1", "worker-a", "claim-1", future),
                None,
            ),
            (
                "non_holder",
                AuthContext::worker("worker-b", "run-1", "claim-1"),
                live("run-1", "worker-a", "claim-1", future),
                Some(DenialClass::IdentityMismatch),
            ),
            (
                "cross_resource",
                AuthContext::worker("worker-a", "run-2", "claim-1"),
                live("run-1", "worker-a", "claim-1", future),
                Some(DenialClass::CrossResource),
            ),
            (
                "expired",
                AuthContext::worker("worker-a", "run-1", "claim-1"),
                live("run-1", "worker-a", "claim-1", expired),
                Some(DenialClass::ClaimExpired),
            ),
            (
                "missing_claim",
                AuthContext::new("worker-a", PrincipalRole::Worker),
                live("run-1", "worker-a", "claim-1", future),
                Some(DenialClass::ClaimRequired),
            ),
        ];
        for (label, auth, lease, expected) in cases {
            let result = auth.authorize(Operation::RecordEffect, &resource, Some(&lease));
            match expected {
                None => assert!(result.is_ok(), "{label}: {result:?}"),
                Some(class) => assert_eq!(result.unwrap_err().class, class, "{label}"),
            }
        }
        let operator = AuthContext::operator("operator");
        assert!(operator
            .authorize(
                Operation::CorrectWorkflow,
                &AuthorizationResource::workflow("wf"),
                None
            )
            .is_ok());
        assert_eq!(
            operator
                .require_semantic_principal(Some("worker-a"))
                .unwrap_err()
                .class,
            DenialClass::IdentityMismatch
        );
    }
}
