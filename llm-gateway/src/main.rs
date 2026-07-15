use std::sync::Arc;

use llm_gateway::adapters::inbound::grpc::serve_grpc;
use llm_gateway::adapters::inbound::rest::router::build_router;
use llm_gateway::adapters::outbound::anthropic::AnthropicProvider;
use llm_gateway::adapters::outbound::env_key::EnvKeyStore;
use llm_gateway::adapters::outbound::openai::OpenAIProvider;
use llm_gateway::config::Config;
use llm_gateway::domain::service::CompletionService;
use llm_gateway::ports::inbound::completion::CompletionUseCase;

#[tokio::main]
async fn main() {
    // Initialize structured logging (JSON format)
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let config = Config::from_env();

    tracing::info!(port = config.port, "starting LLM Gateway");

    // Dependency injection assembly
    let key_store = Arc::new(EnvKeyStore);
    let openai_provider = match OpenAIProvider::new(
        key_store.clone(),
        config.http.clone(),
        config.openai.clone(),
    ) {
        Ok(p) => p,
        Err(e) => {
            tracing::error!("failed to initialize OpenAIProvider: {e}");
            std::process::exit(1);
        }
    };

    let anthropic_provider = match AnthropicProvider::new(
        key_store.clone(),
        config.http.clone(),
        config.anthropic.clone(),
    ) {
        Ok(p) => p,
        Err(e) => {
            tracing::error!("failed to initialize AnthropicProvider: {e}");
            std::process::exit(1);
        }
    };

    let service = CompletionService::new(
        vec![Box::new(openai_provider), Box::new(anthropic_provider)],
        key_store,
    );
    // Coerced to the trait object once here so the exact same instance is shared by
    // both the REST router and the gRPC server below — no second `CompletionService`
    // is ever constructed.
    let state: Arc<dyn CompletionUseCase> = Arc::new(service);

    let router = build_router(state.clone());

    let rest_server = async {
        let listener = tokio::net::TcpListener::bind(("0.0.0.0", config.port))
            .await
            .expect("failed to bind TCP listener");

        tracing::info!(port = config.port, "LLM Gateway (REST) listening");

        axum::serve(listener, router)
            .with_graceful_shutdown(shutdown_signal())
            .await
            .expect("REST server error");
    };

    let grpc_addr = std::net::SocketAddr::from(([0, 0, 0, 0], config.grpc_port));
    let grpc_server = async move {
        tracing::info!(port = config.grpc_port, "LLM Gateway (gRPC) listening");

        serve_grpc(state, grpc_addr)
            .await
            .expect("gRPC server error");
    };

    tokio::join!(rest_server, grpc_server);
}

/// Waits for a `Ctrl+C` (SIGINT) or, on Unix, a `SIGTERM` signal, whichever comes
/// first, logging which one triggered shutdown.
///
/// Used as the graceful-shutdown future for `axum::serve(...).with_graceful_shutdown`,
/// so in-flight requests are allowed to complete before the process exits.
///
/// # Errors
/// Panics if installing either signal handler fails (a process-setup error that
/// should not be recovered from).
async fn shutdown_signal() {
    let ctrl_c = async {
        tokio::signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("failed to install SIGTERM handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {
            tracing::info!("received Ctrl+C, starting graceful shutdown");
        }
        _ = terminate => {
            tracing::info!("received SIGTERM, starting graceful shutdown");
        }
    }
}
