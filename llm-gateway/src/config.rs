/// アプリケーション設定。環境変数から読み込む。
#[derive(Debug, Clone)]
pub struct Config {
    pub port: u16,
    pub openai_api_key: String,
    pub openai_base_url: String,
}

impl Config {
    /// 環境変数から設定を読み込む。
    ///
    /// # Environment Variables
    /// - `LLM_GATEWAY_PORT` — リッスンポート（デフォルト: 8081）
    /// - `OPENAI_API_KEY` — OpenAI APIキー（必須）
    /// - `OPENAI_BASE_URL` — OpenAI APIベースURL（デフォルト: https://api.openai.com）
    pub fn from_env() -> Self {
        let port = std::env::var("LLM_GATEWAY_PORT")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(8081);

        let openai_api_key =
            std::env::var("OPENAI_API_KEY").unwrap_or_default();

        let openai_base_url = std::env::var("OPENAI_BASE_URL")
            .unwrap_or_else(|_| "https://api.openai.com".to_string());

        Self {
            port,
            openai_api_key,
            openai_base_url,
        }
    }
}
