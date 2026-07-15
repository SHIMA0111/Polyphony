mod adapters;
mod config;
mod domain;
mod ports;

use std::sync::Arc;

use adapters::inbound::rest::router::build_router;
use adapters::outbound::env_key::EnvKeyStore;
use adapters::outbound::openai::OpenAIProvider;
use config::Config;
use domain::service::CompletionService;

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

    let service = CompletionService::new(vec![Box::new(openai_provider)], key_store);
    let state = Arc::new(service);

    let router = build_router(state);

    let listener = tokio::net::TcpListener::bind(("0.0.0.0", config.port))
        .await
        .expect("failed to bind TCP listener");

    tracing::info!(port = config.port, "LLM Gateway listening");

    axum::serve(listener, router)
        .with_graceful_shutdown(shutdown_signal())
        .await
        .expect("server error");
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
