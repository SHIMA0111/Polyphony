use std::future::Future;
use std::pin::Pin;

use crate::domain::error::DomainError;
use crate::domain::model::{CompletionRequest, CompletionResponse, ModelInfo};

/// 補完ユースケースポート。
///
/// インバウンドアダプター（REST, gRPC等）がドメインサービスを呼び出すためのインターフェース。
pub trait CompletionUseCase: Send + Sync {
    /// チャット補完を実行する。
    ///
    /// # Arguments
    /// * `req` — 補完リクエスト
    ///
    /// # Errors
    /// モデル未発見、プロバイダーエラー等で `DomainError` を返す。
    fn complete(
        &self,
        req: CompletionRequest,
    ) -> Pin<Box<dyn Future<Output = Result<CompletionResponse, DomainError>> + Send + '_>>;

    /// 全プロバイダーの利用可能モデル一覧を返す。
    fn list_models(&self) -> Vec<ModelInfo>;
}
