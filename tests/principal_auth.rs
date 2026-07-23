//! Cross-face principal authorization parity.
//!
//! CLI, HTTP/UI, MCP, and internal service callers all converge on the
//! AuthContext/Operation contract. Keep the denial matrix table-driven so a
//! new transport cannot silently invent a role or claim rule.

use bitterblossom::auth::{AuthContext, AuthorizationResource, DenialClass, LiveLease, Operation};

fn lease(run_id: &str, principal: &str, claim_id: &str, expires_at: &str) -> LiveLease {
    LiveLease {
        run_id: run_id.into(),
        holder_principal: principal.into(),
        claim_id: claim_id.into(),
        expires_at: expires_at.into(),
    }
}

#[test]
fn cross_face_execute_matrix_has_one_stable_contract() {
    let resource = AuthorizationResource::run_with_claim("run-1", "claim-1");
    let cases = [
        (
            "cli-holder",
            AuthContext::worker("worker-a", "run-1", "claim-1"),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            None,
        ),
        (
            "http-non-holder",
            AuthContext::worker("worker-b", "run-1", "claim-1"),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            Some(DenialClass::IdentityMismatch),
        ),
        (
            "mcp-impersonation",
            AuthContext::worker("worker-a", "run-2", "claim-1"),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            Some(DenialClass::CrossResource),
        ),
        (
            "ui-expired",
            AuthContext::worker("worker-a", "run-1", "claim-1"),
            lease("run-1", "worker-a", "claim-1", "2000-01-01T00:00:00Z"),
            Some(DenialClass::ClaimExpired),
        ),
        (
            "service-missing-claim",
            AuthContext::new("worker-a", bitterblossom::auth::PrincipalRole::Worker),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            Some(DenialClass::ClaimRequired),
        ),
        (
            "cli-operator-bypass",
            AuthContext::operator("operator"),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            None,
        ),
        (
            "http-admin-bypass",
            AuthContext::admin("admin"),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            None,
        ),
        (
            "mcp-controller-bypass",
            AuthContext::controller("bb-controller"),
            lease("run-1", "worker-a", "claim-1", "2999-01-01T00:00:00Z"),
            None,
        ),
    ];

    for (face, auth, live_lease, expected) in cases {
        let result = auth.authorize(Operation::ExecuteWorkflowRun, &resource, Some(&live_lease));
        match expected {
            None => assert!(result.is_ok(), "{face}: {result:?}"),
            Some(class) => {
                let error = result.expect_err(face);
                assert_eq!(error.class, class, "{face}");
                assert_eq!(error.denial_class(), class.as_str(), "{face}");
            }
        }
    }
}

#[test]
fn denied_matrix_is_non_mutating_by_construction() {
    let auth = AuthContext::worker("worker-b", "run-1", "claim-1");
    let resource = AuthorizationResource::run_with_claim("run-1", "claim-1");
    let before = (resource.clone(), auth.clone());
    let error = auth
        .authorize(
            Operation::ExecuteWorkflowRun,
            &resource,
            Some(&lease(
                "run-1",
                "worker-a",
                "claim-1",
                "2999-01-01T00:00:00Z",
            )),
        )
        .unwrap_err();
    assert_eq!(error.denial_class(), "identity_mismatch");
    assert_eq!((resource, auth), before);
}
