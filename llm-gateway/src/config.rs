/// アプリケーション共通設定。環境変数から読み込む。
///
/// プロバイダー固有の設定（APIキー、ベースURL等）はここに含めない。
/// それらは各アダプターが `KeyStore` や自身の環境変数から取得する。
#[derive(Debug, Clone)]
pub struct Config {
    pub port: u16,
}

impl Config {
    /// 環境変数から設定を読み込む。
    ///
    /// # Environment Variables
    /// - `LLM_GATEWAY_PORT` — リッスンポート（デフォルト: 8081）
    pub fn from_env() -> Self {
        let port = std::env::var("LLM_GATEWAY_PORT")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(8081);

        Self { port }
    }
}
