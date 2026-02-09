use serde::{Deserialize, Serialize};

/// チャットメッセージのロール。
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum Role {
    System,
    User,
    Assistant,
}

/// チャットメッセージ。ロールと内容のペア。
#[derive(Debug, Clone, Serialize, Deserialize)]
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
#[derive(Debug, Clone, Serialize)]
pub struct CompletionResponse {
    pub id: String,
    pub model: String,
    pub choices: Vec<Choice>,
    pub usage: Usage,
}

/// レスポンス内の選択肢。
#[derive(Debug, Clone, Serialize)]
pub struct Choice {
    pub index: u32,
    pub message: ChatMessage,
    pub finish_reason: String,
}

/// トークン使用量。
#[derive(Debug, Clone, Serialize)]
pub struct Usage {
    pub prompt_tokens: u32,
    pub completion_tokens: u32,
    pub total_tokens: u32,
}

/// 利用可能なモデル情報。
#[derive(Debug, Clone, Serialize)]
pub struct ModelInfo {
    pub id: String,
    pub provider: String,
    pub owned_by: String,
}
