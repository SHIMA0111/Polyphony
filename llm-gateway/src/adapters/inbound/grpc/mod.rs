//! gRPC inbound adapter.
//!
//! Serves the same `CompletionUseCase` domain port the REST inbound adapter
//! (`adapters::inbound::rest`) uses, over `tonic`/gRPC instead of `axum`/HTTP+JSON.

pub mod completion_service;
pub mod convert;
pub mod models_service;
pub mod pb;
pub mod server;

pub use server::serve_grpc;
