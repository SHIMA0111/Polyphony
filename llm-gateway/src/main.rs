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

    // Dependency injection assembly.
    //
    // Post-review fix: previously all three providers were constructed and registered
    // unconditionally, which meant `GET /ready` (CompletionService::readiness, which
    // checks every *registered* provider's key is resolvable) could never succeed
    // unless every one of OPENAI_API_KEY/ANTHROPIC_API_KEY/GEMINI_API_KEY was set --
    // even for deployments that only intend to use a subset of providers. A provider is
    // now only constructed and registered when its `{PROVIDER}_API_KEY` env var is
    // present and non-empty; skipped providers are logged at info level and simply
    // absent from `list_models`/dispatch, not treated as an error. Only having zero
    // providers register at all is an error (nothing could ever be served), matching
    // this gateway's fail-fast-at-startup posture.
    let key_store = Arc::new(EnvKeyStore);
    let mut providers: Vec<Box<dyn llm_gateway::ports::outbound::provider::LLMProvider>> =
        Vec::new();

    if has_api_key("OPENAI_API_KEY") {
        match OpenAIProvider::new(key_store.clone(), config.http.clone(), config.openai.clone()) {
            Ok(p) => providers.push(Box::new(p)),
            Err(e) => {
                tracing::error!(provider = "openai", error = %e, "failed to initialize provider");
                std::process::exit(1);
            }
        }
    } else {
        tracing::info!(provider = "openai", "OPENAI_API_KEY not set, skipping provider registration");
    }

    if has_api_key("ANTHROPIC_API_KEY") {
        match AnthropicProvider::new(
            key_store.clone(),
            config.http.clone(),
            config.anthropic.clone(),
        ) {
            Ok(p) => providers.push(Box::new(p)),
            Err(e) => {
                tracing::error!(provider = "anthropic", error = %e, "failed to initialize provider");
                std::process::exit(1);
            }
        }
    } else {
        tracing::info!(provider = "anthropic", "ANTHROPIC_API_KEY not set, skipping provider registration");
    }

    if has_api_key("GEMINI_API_KEY") {
        match GeminiProvider::new(key_store.clone(), config.http.clone(), config.gemini.clone()) {
            Ok(p) => providers.push(Box::new(p)),
            Err(e) => {
                tracing::error!(provider = "gemini", error = %e, "failed to initialize provider");
                std::process::exit(1);
            }
        }
    } else {
        tracing::info!(provider = "gemini", "GEMINI_API_KEY not set, skipping provider registration");
    }

    if providers.is_empty() {
        tracing::error!(
            "no LLM providers registered: set at least one of OPENAI_API_KEY, \
             ANTHROPIC_API_KEY, or GEMINI_API_KEY"
        );
        std::process::exit(1);
    }

    let service = CompletionService::new(providers, key_store);
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

        serve_grpc(state, grpc_addr, shutdown_signal())
            .await
            .expect("gRPC server error");
    };

    tokio::join!(rest_server, grpc_server);
}

/// Reports whether the named environment variable is set to a non-empty value.
///
/// Used to decide whether a given provider's API key is present before constructing
/// and registering that provider (see the provider-assembly block in `main`) --
/// treating an empty string the same as "unset" avoids silently registering a
/// provider with a blank key that would only fail later, at request or readiness
/// time.
fn has_api_key(var_name: &str) -> bool {
    std::env::var(var_name).is_ok_and(|v| !v.is_empty())
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
