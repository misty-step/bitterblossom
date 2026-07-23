//! One authenticated mutation service shared by CLI, HTTP, MCP, and local UI.

use anyhow::{Context, Result};

use crate::auth::{
    AuthContext, AuthError, AuthorizationResource, DenialClass, LiveLease, Operation, PrincipalRole,
};
use crate::ingress;
use crate::ledger::{IngressOutcome, IngressRequest, Ledger};
use crate::spec::Plane;
use crate::workflow::{AcceptOutcome, ImportOutcome, WorkflowDoc, WorkflowRow};
use crate::workflow_runtime::{self, TriggerEnvelope};

pub struct WorkflowService<'a> {
    pub plane: &'a Plane,
    pub ledger: &'a Ledger,
    pub auth: AuthContext,
    pub source: &'static str,
}

impl<'a> WorkflowService<'a> {
    pub fn new(plane: &'a Plane, ledger: &'a Ledger, auth: AuthContext) -> Self {
        Self {
            plane,
            ledger,
            auth,
            source: "service",
        }
    }

    pub fn with_source(mut self, source: &'static str) -> Self {
        self.source = source;
        self
    }

    fn live_lease(&self, run_id: Option<&str>) -> Result<Option<LiveLease>> {
        let Some(run_id) = run_id else {
            return Ok(None);
        };
        if let Some(lease) = self.ledger.workflow_run_lease(run_id)? {
            return Ok(Some(lease));
        }
        self.ledger.legacy_run_lease(run_id)
    }

    fn authorize(&self, operation: Operation, resource: AuthorizationResource) -> Result<()> {
        let lease = self.live_lease(resource.run_id.as_deref())?;
        match self.auth.authorize(operation, &resource, lease.as_ref()) {
            Ok(()) => Ok(()),
            Err(error) => {
                // A denial is itself an operator fact. Preserve the stable
                // AuthError class even if writing its receipt fails, but never
                // silently discard the persistence failure.
                if let Err(audit_error) = self
                    .ledger
                    .record_workflow_auth_denial(&resource, operation, &self.auth, &error)
                {
                    return Err(anyhow::Error::from(AuthError::for_operation(
                        error.class,
                        operation,
                        format!(
                            "{}; authorization denial audit failed: {audit_error:#}",
                            error.detail
                        ),
                    )));
                }
                Err(anyhow::Error::from(error))
            }
        }
    }

    fn audit(&self, operation: Operation, resource: &AuthorizationResource) -> Result<()> {
        self.ledger
            .record_workflow_auth_event_with_resource(resource, operation, &self.auth)
    }

    pub fn create_workflow(
        &self,
        doc: &WorkflowDoc,
        note: Option<&str>,
    ) -> Result<(WorkflowRow, i64)> {
        let resource = AuthorizationResource::workflow(&doc.name);
        self.authorize(Operation::CreateWorkflow, resource.clone())?;
        let result = self.ledger.create_workflow(doc, self.source, note)?;
        self.audit(Operation::CreateWorkflow, &resource)?;
        Ok(result)
    }

    pub fn revise_workflow(
        &self,
        name: &str,
        doc: &WorkflowDoc,
        note: Option<&str>,
    ) -> Result<(WorkflowRow, i64)> {
        let resource = AuthorizationResource::workflow(name);
        self.authorize(Operation::ReviseWorkflow, resource.clone())?;
        let result = self.ledger.revise_workflow(name, doc, self.source, note)?;
        self.audit(Operation::ReviseWorkflow, &resource)?;
        Ok(result)
    }

    pub fn import_workflow(
        &self,
        doc: &WorkflowDoc,
        note: Option<&str>,
    ) -> Result<(WorkflowRow, i64, ImportOutcome)> {
        let resource = AuthorizationResource::workflow(&doc.name);
        self.authorize(Operation::ImportWorkflow, resource.clone())?;
        let result = self.ledger.import_workflow(doc, self.source, note)?;
        self.audit(Operation::ImportWorkflow, &resource)?;
        Ok(result)
    }

    pub fn activate_workflow(&self, name: &str, revision: Option<i64>) -> Result<WorkflowRow> {
        let routes = ingress::task_webhook_routes(self.plane);
        self.activate_workflow_with_reserved_routes(name, revision, &routes)
    }

    pub fn activate_workflow_with_reserved_routes(
        &self,
        name: &str,
        revision: Option<i64>,
        routes: &[String],
    ) -> Result<WorkflowRow> {
        let resource = AuthorizationResource::workflow(name);
        self.authorize(Operation::ActivateWorkflow, resource.clone())?;
        let result = self
            .ledger
            .activate_workflow_with_reserved_routes(name, revision, routes)?;
        self.audit(Operation::ActivateWorkflow, &resource)?;
        Ok(result)
    }

    pub fn pause_workflow(&self, name: &str, reason: &str) -> Result<WorkflowRow> {
        let resource = AuthorizationResource::workflow(name);
        self.authorize(Operation::PauseWorkflow, resource.clone())?;
        let result = self.ledger.pause_workflow(name, reason)?;
        self.audit(Operation::PauseWorkflow, &resource)?;
        Ok(result)
    }

    pub fn resume_workflow(&self, name: &str) -> Result<WorkflowRow> {
        let resource = AuthorizationResource::workflow(name);
        self.authorize(Operation::ResumeWorkflow, resource.clone())?;
        let routes = ingress::task_webhook_routes(self.plane);
        let result = self
            .ledger
            .resume_workflow_with_reserved_routes(name, &routes)?;
        self.audit(Operation::ResumeWorkflow, &resource)?;
        Ok(result)
    }

    pub fn archive_workflow(&self, name: &str) -> Result<WorkflowRow> {
        let resource = AuthorizationResource::workflow(name);
        self.authorize(Operation::ArchiveWorkflow, resource.clone())?;
        let result = self.ledger.archive_workflow(name)?;
        self.audit(Operation::ArchiveWorkflow, &resource)?;
        Ok(result)
    }

    pub fn rollback_workflow(&self, name: &str, to_revision: i64) -> Result<(WorkflowRow, i64)> {
        let resource = AuthorizationResource::workflow(name);
        self.authorize(Operation::RollbackWorkflow, resource.clone())?;
        let routes = ingress::task_webhook_routes(self.plane);
        let result =
            self.ledger
                .rollback_workflow_with_reserved_routes(name, to_revision, &routes)?;
        self.audit(Operation::RollbackWorkflow, &resource)?;
        Ok(result)
    }

    pub fn accept(&self, envelope: &TriggerEnvelope) -> Result<AcceptOutcome> {
        let resource = AuthorizationResource::workflow(&envelope.workflow);
        self.authorize(Operation::AcceptWorkflowRun, resource.clone())?;
        let result = self.ledger.accept_workflow_run(
            self.plane,
            &envelope.workflow,
            envelope.source.kind(),
            envelope.payload.as_deref(),
            envelope.dedupe_key.as_deref(),
        )?;
        self.audit(Operation::AcceptWorkflowRun, &resource)?;
        Ok(result)
    }

    pub fn resolve_workflow_run(
        &self,
        run_id: &str,
        state: &str,
        reason: &str,
    ) -> Result<crate::workflow_runtime::WorkflowRunStatusRow> {
        let resource = AuthorizationResource::run(run_id);
        self.authorize(Operation::ResolveWorkflowRun, resource.clone())?;
        let result = workflow_runtime::resolve_workflow_run(self.ledger, run_id, state, reason)?;
        self.audit(Operation::ResolveWorkflowRun, &resource)?;
        Ok(result)
    }

    pub fn execute_workflow_run(&self, run_id: &str) -> Result<serde_json::Value> {
        // Read the live lease before authorization. A worker can only execute
        // the exact run/claim it was assigned; controller/operator/admin may
        // admit a queued run under their own authenticated identity.
        let mut lease = self.live_lease(Some(run_id))?;
        let mut resource = AuthorizationResource {
            run_id: Some(run_id.to_string()),
            claim_id: lease.as_ref().map(|value| value.claim_id.clone()),
            ..AuthorizationResource::default()
        };
        self.authorize(Operation::ExecuteWorkflowRun, resource.clone())?;

        if let Some(current) = lease.as_ref() {
            let worker_holds_exact_claim = self.auth.role == PrincipalRole::Worker
                && current.holder_principal == self.auth.principal
                && self.auth.run_id.as_deref() == Some(run_id)
                && self.auth.claim_id.as_deref() == Some(current.claim_id.as_str());
            if !worker_holds_exact_claim {
                let denial = AuthError::for_operation(
                    DenialClass::IdentityMismatch,
                    Operation::ExecuteWorkflowRun,
                    "run is already claimed by an authenticated execution holder",
                );
                self.ledger.record_workflow_auth_denial(
                    &resource,
                    Operation::ExecuteWorkflowRun,
                    &self.auth,
                    &denial,
                )?;
                return Err(denial.into());
            }
        } else {
            // Workers never self-assign a queued run. The explicit worker
            // entrypoint below performs assignment with its authenticated
            // principal and claim, then comes back through this door.
            if self.auth.role == PrincipalRole::Worker {
                let denial = AuthError::for_operation(
                    DenialClass::ClaimRequired,
                    Operation::ExecuteWorkflowRun,
                    "current run lease required",
                );
                self.ledger.record_workflow_auth_denial(
                    &resource,
                    Operation::ExecuteWorkflowRun,
                    &self.auth,
                    &denial,
                )?;
                return Err(denial.into());
            }
            let claim_id = self
                .auth
                .claim_id
                .clone()
                .unwrap_or_else(|| format!("controller:{}:{}", self.auth.principal, run_id));
            if !self
                .ledger
                .claim_workflow_run_for(run_id, &self.auth.principal, &claim_id, 3600)?
            {
                let state = self
                    .ledger
                    .workflow_run_status(run_id)?
                    .map(|status| status.state)
                    .unwrap_or_else(|| "unknown".to_string());
                return Err(anyhow::anyhow!(
                    "workflow run {run_id} is {state}; only queued runs execute"
                ));
            }
            lease = self.live_lease(Some(run_id))?;
            resource.claim_id = lease.as_ref().map(|value| value.claim_id.clone());
            // Re-read and re-authorize the exact claim assigned above.
            self.authorize(Operation::ExecuteWorkflowRun, resource.clone())?;
        }

        let lease = lease.context("workflow execution claim disappeared")?;
        let result = workflow_runtime::execute_run_as(
            self.plane,
            self.ledger,
            run_id,
            &lease.holder_principal,
            &lease.claim_id,
        )?;
        let audit_auth = self
            .auth
            .clone()
            .with_run(run_id.to_string(), lease.claim_id.clone());
        self.ledger.record_workflow_auth_event_with_resource(
            &resource,
            Operation::ExecuteWorkflowRun,
            &audit_auth,
        )?;
        Ok(result)
    }

    pub fn stop_workflow_run(
        &self,
        run_id: &str,
        reason: &str,
    ) -> Result<crate::workflow_runtime::WorkflowRunStatusRow> {
        let resource = AuthorizationResource::run(run_id);
        self.authorize(Operation::StopWorkflowRun, resource.clone())?;
        let result = self.ledger.request_workflow_run_stop(run_id, reason)?;
        self.audit(Operation::StopWorkflowRun, &resource)?;
        Ok(result)
    }

    pub fn dispatch_for(
        plane: &'a Plane,
        ledger: &'a mut Ledger,
        auth: AuthContext,
        req: IngressRequest<'_>,
    ) -> Result<IngressOutcome> {
        let service = WorkflowService::new(plane, &*ledger, auth.clone());
        let resource = AuthorizationResource::default();
        service.authorize(Operation::DispatchRun, resource.clone())?;
        drop(service);
        let result = ledger.ingest(req)?;
        // Legacy task dispatch has no workflow resource, but its service allow
        // is still a durable auth receipt in the global auth_events table.
        WorkflowService::new(plane, &*ledger, auth)
            .with_source("dispatch")
            .audit(Operation::DispatchRun, &resource)?;
        Ok(result)
    }

    #[allow(clippy::too_many_arguments)]
    pub fn raise_ask_for(
        plane: &'a Plane,
        ledger: &'a mut Ledger,
        auth: AuthContext,
        id: &str,
        run_id: &str,
        task: &str,
        kind: &str,
        question: &str,
        context: Option<&str>,
        blocking: bool,
        window_seconds: i64,
    ) -> Result<crate::ledger::AskRow> {
        let service = WorkflowService::new(plane, &*ledger, auth.clone());
        let lease = service.ledger.legacy_run_lease(run_id)?;
        let resource = AuthorizationResource {
            run_id: Some(run_id.to_string()),
            claim_id: lease.as_ref().map(|value| value.claim_id.clone()),
            ..AuthorizationResource::default()
        };
        service.authorize(Operation::RaiseAsk, resource.clone())?;
        let ask = ledger.raise_ask(
            id,
            run_id,
            task,
            kind,
            question,
            context,
            blocking,
            window_seconds,
        )?;
        service.audit(Operation::RaiseAsk, &resource)?;
        Ok(ask)
    }

    pub fn answer_ask_for(
        plane: &'a Plane,
        ledger: &'a mut Ledger,
        auth: AuthContext,
        id: &str,
        answer: &str,
        answered_by: &str,
    ) -> Result<(crate::ledger::AskRow, Option<String>)> {
        let (ask, resource) = {
            let service = WorkflowService::new(plane, &*ledger, auth.clone());
            let ask = service.ledger.ask(id)?;
            let lease = service.ledger.legacy_run_lease(&ask.run_id)?;
            let resource = AuthorizationResource {
                run_id: Some(ask.run_id.clone()),
                claim_id: lease.as_ref().map(|value| value.claim_id.clone()),
                ..AuthorizationResource::default()
            };
            service.authorize(Operation::AnswerAsk, resource.clone())?;
            if let Err(error) = service.require_answer_identity(Some(answered_by)) {
                if let Some(auth_error) = error.downcast_ref::<AuthError>() {
                    service.ledger.record_workflow_auth_denial(
                        &resource,
                        Operation::AnswerAsk,
                        &service.auth,
                        auth_error,
                    )?;
                }
                return Err(error);
            }
            (ask, resource)
        };
        let run = ledger.run(&ask.run_id)?;
        let result = if run.state == "parked_on_ask" {
            let packet = match crate::artifacts::read(
                ledger,
                &ask.run_id,
                crate::dispatch::ASK_PACKET_FILENAME,
            )? {
                crate::artifacts::ReadOutcome::Text { content, .. } => Some(content),
                _ => None,
            };
            let resume_payload = serde_json::json!({
                "ask": {"id": ask.id, "kind": ask.kind, "question": ask.question, "context": ask.context},
                "answer": answer,
                "answered_by": answered_by,
                "packet": packet,
            }).to_string();
            let resume_key = format!("resume:{id}");
            let (answered, outcome) = ledger.answer_ask_and_resume(
                id,
                answer,
                answered_by,
                IngressRequest {
                    task: &ask.task,
                    trigger_kind: "resume",
                    idempotency_key: Some(&resume_key),
                    source_event_id: None,
                    payload: Some(&resume_payload),
                    parent_run_id: Some(&ask.run_id),
                },
            )?;
            (answered, Some(outcome.run_id))
        } else {
            (ledger.answer_ask(id, answer, answered_by)?, None)
        };
        WorkflowService::new(plane, &*ledger, auth).audit(Operation::AnswerAsk, &resource)?;
        Ok(result)
    }

    pub fn require_answer_identity(&self, answered_by: Option<&str>) -> Result<()> {
        self.auth
            .require_semantic_principal(answered_by)
            .map_err(anyhow::Error::from)
    }
}

pub fn auth_context_for_local() -> Result<AuthContext> {
    AuthContext::trusted_local().map_err(anyhow::Error::from)
}

pub fn auth_context_for_controller() -> AuthContext {
    AuthContext::controller("bb-controller").with_transport("internal")
}

pub fn auth_context_for_http(presented_bearer: Option<&str>) -> Result<AuthContext> {
    AuthContext::from_http_bearer(
        presented_bearer,
        std::env::var("BB_API_TOKEN").ok().as_deref(),
    )
    .map_err(anyhow::Error::from)
}

pub fn auth_context_for_worker(principal: &str, run_id: &str, claim_id: &str) -> AuthContext {
    AuthContext::worker(principal, run_id, claim_id)
}

pub fn auth_error(error: &anyhow::Error) -> Option<&AuthError> {
    error.downcast_ref::<AuthError>()
}
