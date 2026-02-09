use crate::domain::error::DomainError;

/// APIキー取得ポート。
///
/// プロバイダー名からAPIキーを取得する。環境変数、Vault等のバックエンドに差し替え可能。
pub trait KeyStore: Send + Sync {
    /// 指定プロバイダーのAPIキーを取得する。
    ///
    /// # Arguments
    /// * `provider` — プロバイダー名（例: "openai"）
    ///
    /// # Errors
    /// キーが見つからない場合 `DomainError::KeyNotFound` を返す。
    fn get_key(&self, provider: &str) -> Result<String, DomainError>;
}
