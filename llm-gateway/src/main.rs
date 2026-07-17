use std::sync::Arc;

use llm_gateway::adapters::inbound::grpc::serve_grpc;
use llm_gateway::adapters::inbound::rest::router::build_router;
use llm_gateway::adapters::outbound::anthropic::AnthropicProvider;
use llm_gateway::adapters::outbound::env_key::EnvKeyStore;
use llm_gateway::adapters::outbound::gemini::GeminiProvider;
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

    let gemini_provider = match GeminiProvider::new(
        key_store.clone(),
        config.http.clone(),
        config.gemini.clone(),
    ) {
        Ok(p) => p,
        Err(e) => {
            tracing::error!("failed to initialize GeminiProvider: {e}");
            std::process::exit(1);
        }
    };

    let service = CompletionService::new(
        vec![
            Box::new(openai_provider),
            Box::new(anthropic_provider),
            Box::new(gemini_provider),
        ],
        key_store,
    );
    // Coerced to the trait object once here so the exact same instance is shared by
    // both the REST router and the gRPC server below — no second `CompletionService`
    // is ever constructed.
    let state: Arc<dyn CompletionUseCase> = Arc::new(service);

    let router = build_router(state.clone());

    // `shutdown_signal()` resolves at most once, but both the REST and gRPC servers
    // each need their own graceful-shutdown future. A `watch` channel lets any number
    // of clones observe the same one-shot signal: the sender task awaits it once and
    // flips the shared value, and each receiver (already subscribed before the flip)
    // wakes on that change regardless of exactly when the signal fires.
    let (shutdown_tx, shutdown_rx) = tokio::sync::watch::channel(false);
    let signal_task = async move {
        shutdown_signal().await;
        // No listeners left is not an error here — both servers may have already
        // exited for other reasons.
        let _ = shutdown_tx.send(true);
    };

    let rest_shutdown_rx = shutdown_rx.clone();
    let rest_server = async {
        let listener = tokio::net::TcpListener::bind(("0.0.0.0", config.port))
            .await
            .expect("failed to bind TCP listener");

        tracing::info!(port = config.port, "LLM Gateway (REST) listening");

        axum::serve(listener, router)
            .with_graceful_shutdown(wait_for_shutdown(rest_shutdown_rx))
            .await
            .expect("REST server error");
    };

    let grpc_addr = std::net::SocketAddr::from(([0, 0, 0, 0], config.grpc_port));
    let grpc_server = async move {
        tracing::info!(port = config.grpc_port, "LLM Gateway (gRPC) listening");

        serve_grpc(state, grpc_addr, wait_for_shutdown(shutdown_rx))
            .await
            .expect("gRPC server error");
    };

    tokio::join!(signal_task, rest_server, grpc_server);
}

/// Waits until `rx` observes a `true` value, i.e. until the shared shutdown signal has
/// fired.
///
/// Used to derive independent graceful-shutdown futures for the REST and gRPC servers
/// from a single `shutdown_signal()` call, since that underlying signal can only be
/// awaited once.
async fn wait_for_shutdown(mut rx: tokio::sync::watch::Receiver<bool>) {
    // `wait_for` also checks the currently held value first, so a signal that already
    // fired before this receiver started waiting is not missed.
    let _ = rx.wait_for(|shutdown| *shutdown).await;
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
