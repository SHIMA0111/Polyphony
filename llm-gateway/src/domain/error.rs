use std::fmt;

/// ドメイン層のエラー型。
///
/// 全てのビジネスロジックエラーを表現する。アダプター層でHTTPステータスコード等に変換される。
#[derive(Debug)]
pub enum DomainError {
    /// LLMプロバイダーからのエラー応答
    ProviderError(String),
    /// リクエストパラメータが不正
    InvalidRequest(String),
    /// 指定されたモデルが見つからない
    ModelNotFound(String),
    /// APIキーが見つからない
    KeyNotFound(String),
    /// プロバイダーへのリクエストがタイムアウト
    Timeout,
}

impl fmt::Display for DomainError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::ProviderError(msg) => write!(f, "provider error: {msg}"),
            Self::InvalidRequest(msg) => write!(f, "invalid request: {msg}"),
            Self::ModelNotFound(model) => write!(f, "model not found: {model}"),
            Self::KeyNotFound(provider) => {
                write!(f, "API key not found for provider: {provider}")
            }
            Self::Timeout => write!(f, "request timed out"),
        }
    }
}

impl std::error::Error for DomainError {}
