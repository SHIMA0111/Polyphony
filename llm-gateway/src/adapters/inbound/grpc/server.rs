//! gRPC server bootstrap.
//!
//! Serves `CompletionService`, `ModelsService`, and the standard
//! `grpc.health.v1.Health` service from a single `tonic` transport, all backed by the
//! same `Arc<dyn CompletionUseCase>` instance the REST router uses.

use std::future::Future;
use std::net::SocketAddr;
use std::sync::Arc;

use crate::ports::inbound::completion::CompletionUseCase;

use super::completion_service::GrpcCompletionService;
use super::models_service::GrpcModelsService;
use super::pb::completion_service_server::CompletionServiceServer;
use super::pb::models_service_server::ModelsServiceServer;

/// Starts the gRPC server and serves it on `addr` until `shutdown` resolves.
///
/// Mirrors the REST `GET /health` liveness-only philosophy: both gRPC services are
/// marked `Serving` unconditionally at startup (no deep dependency checks, e.g.
/// probing the configured LLM providers) — this only proves the gRPC server itself is
/// reachable and serving.
///
/// # Arguments
/// * `state` — Shared domain service implementing `CompletionUseCase`, the same
///   instance injected into the REST router.
/// * `addr` — Socket address to bind and listen on.
/// * `shutdown` — Future that resolves when a shutdown signal is received. Mirrors the
///   REST server's `axum::serve(...).with_graceful_shutdown` behavior, letting
///   in-flight RPCs complete before the transport stops accepting new connections.
///
/// # Errors
/// Returns `tonic::transport::Error` if the server fails to bind `addr` or the
/// transport encounters a fatal error while serving.
pub async fn serve_grpc(
    state: Arc<dyn CompletionUseCase>,
    addr: SocketAddr,
    shutdown: impl Future<Output = ()> + Send + 'static,
) -> Result<(), tonic::transport::Error> {
    let (health_reporter, health_service) = tonic_health::server::health_reporter();

    health_reporter
        .set_serving::<CompletionServiceServer<GrpcCompletionService>>()
        .await;
    health_reporter
        .set_serving::<ModelsServiceServer<GrpcModelsService>>()
        .await;

    let completion_svc = CompletionServiceServer::new(GrpcCompletionService::new(state.clone()));
    let models_svc = ModelsServiceServer::new(GrpcModelsService::new(state));

    tonic::transport::Server::builder()
        .add_service(health_service)
        .add_service(completion_svc)
        .add_service(models_svc)
        .serve_with_shutdown(addr, shutdown)
        .await
}
