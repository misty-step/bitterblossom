//! One authenticated mutation service shared by CLI, HTTP, MCP, and local UI.

use anyhow::Result;

use crate::auth::{AuthContext, AuthError, AuthorizationResource, Operation};
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

    fn authorize(&self, operation: Operation, resource: AuthorizationResource) -> Result<()> {
        let lease = resource
            .run_id
            .as_deref()
            .map(|run_id| self.ledger.workflow_run_lease(run_id))
            .transpose()?
            .flatten();
        self.auth
            .authorize(operation, &resource, lease.as_ref())
            .map_err(anyhow::Error::from)
    }

    fn audit(&self, operation: Operation, resource: &AuthorizationResource) -> Result<()> {
        self.ledger.record_workflow_auth_event(
            resource.workflow.as_deref(),
            resource.run_id.as_deref(),
            operation,
            &self.auth,
        )
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
        let resource = AuthorizationResource::run(run_id);
        self.authorize(Operation::ExecuteWorkflowRun, resource.clone())?;
        let result = workflow_runtime::execute_run(self.plane, self.ledger, run_id)?;
        self.audit(Operation::ExecuteWorkflowRun, &resource)?;
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
        // Legacy task dispatch has no workflow resource; authorization still ran before ingest.
        Ok(result)
    }

    pub fn record_effect(&self, run_id: &str, claim_id: &str) -> Result<()> {
        let resource = AuthorizationResource::run_with_claim(run_id, claim_id);
        self.authorize(Operation::RecordEffect, resource.clone())?;
        self.ledger
            .record_auth_event(None, Some(run_id), Operation::RecordEffect, &self.auth)?;
        Ok(())
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
            let resource = AuthorizationResource {
                run_id: Some(ask.run_id.clone()),
                claim_id: auth.claim_id.clone(),
                ..AuthorizationResource::default()
            };
            service.authorize(Operation::AnswerAsk, resource.clone())?;
            service.require_answer_identity(Some(answered_by))?;
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
