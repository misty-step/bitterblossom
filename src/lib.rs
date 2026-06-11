//! Bitterblossom v3: the event plane spine. Tasks, agents, and triggers
//! are files; runs are durable ledger rows; substrates execute. See
//! docs/spine.md for the operator contract.

pub mod budget;
pub mod dispatch;
pub mod harness;
pub mod ingress;
pub mod ledger;
pub mod notify;
pub mod recovery;
pub mod serve;
pub mod spec;
pub mod submit;
pub mod substrate;
