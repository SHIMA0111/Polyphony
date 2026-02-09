/// チャットメッセージのロール。
///
/// アプリケーション内部の概念的なロールを表す。
/// 各LLMプロバイダー固有のロール文字列（OpenAIの"system"/"developer"、Geminiの"model"等）
/// への変換はアダプター層で行う。
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Role {
    /// システム指示・コンテキスト設定
    System,
    /// ユーザー入力
    User,
    /// AI応答
    Assistant,
}

/// チャットメッセージ。ロールと内容のペア。
#[derive(Debug, Clone)]
pub struct ChatMessage {
    pub role: Role,
    pub content: String,
}

/// LLM補完リクエスト。プロバイダー非依存のドメインモデル。
#[derive(Debug, Clone)]
pub struct CompletionRequest {
    pub model: String,
    pub messages: Vec<ChatMessage>,
    pub temperature: Option<f32>,
    pub max_tokens: Option<u32>,
}

/// LLM補完レスポンス。プロバイダー非依存のドメインモデル。
#[derive(Debug, Clone)]
pub struct CompletionResponse {
    pub id: String,
    pub model: String,
    pub choices: Vec<Choice>,
    pub usage: Usage,
}

/// レスポンス内の選択肢。
#[derive(Debug, Clone)]
pub struct Choice {
    pub index: u32,
    pub message: ChatMessage,
    pub finish_reason: String,
}

/// トークン使用量。
#[derive(Debug, Clone)]
pub struct Usage {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

/// 利用可能なモデル情報。
#[derive(Debug, Clone)]
pub struct ModelInfo {
    pub id: String,
    pub provider: String,
    pub owned_by: String,
}
