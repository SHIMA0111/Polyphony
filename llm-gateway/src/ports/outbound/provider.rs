use std::future::Future;
use std::pin::Pin;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionRequest, CompletionResponse, ModelInfo};

/// LLMプロバイダーポート。
///
/// 各LLMプロバイダー（OpenAI, Anthropic, Gemini等）のアダプターが実装するトレイト。
/// プロバイダー追加時はこのトレイトを実装するだけでよい。
pub trait LLMProvider: Send + Sync {
    /// チャット補完リクエストを実行する。
    ///
    /// # Arguments
    /// * `req` — 補完リクエスト
    ///
    /// # Errors
    /// プロバイダーエラー、タイムアウト等で `DomainError` を返す。
    fn complete(
        &self,
        req: &CompletionRequest,
    ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>>;

    /// このプロバイダーが提供するモデル一覧を返す。
    fn models(&self) -> Vec<ModelInfo>;

    /// プロバイダー名を返す（例: "openai"）。
    fn provider_name(&self) -> &str;
}
