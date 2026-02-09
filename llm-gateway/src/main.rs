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
    // 構造化ログ初期化（JSON形式）
    tracing_subscriber::fmt()
        .json()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
        )
        .init();

    let config = Config::from_env();

    tracing::info!(port = config.port, "starting LLM Gateway");

    // DI組み立て
    let key_store = Arc::new(EnvKeyStore);
    let openai_provider = match OpenAIProvider::new(key_store) {
        Ok(p) => p,
        Err(e) => {
            tracing::error!("failed to initialize OpenAIProvider: {e}");
            std::process::exit(1);
        }
    };

    let service = CompletionService::new(vec![Box::new(openai_provider)]);
    let state = Arc::new(service);

    let router = build_router(state);

    let listener = tokio::net::TcpListener::bind(("0.0.0.0", config.port))
        .await
        .expect("failed to bind TCP listener");

    tracing::info!(port = config.port, "LLM Gateway listening");

    axum::serve(listener, router)
        .await
        .expect("server error");
}
