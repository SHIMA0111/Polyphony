# Phase 1: LLM Gateway (Rust) 実装チェックリスト

## 実装ステップ

- [x] Step 1: Cargoプロジェクト初期化 + 依存クレート追加
- [x] Step 2: `config.rs` — 環境変数設定読み込み
- [x] Step 3: `domain/error.rs` — DomainError enum
- [x] Step 4: `domain/model.rs` — ドメインモデル（Role, ChatMessage, CompletionRequest/Response等）
- [x] Step 5: `ports/outbound/key_store.rs` — KeyStore trait
- [x] Step 6: `ports/outbound/provider.rs` — LLMProvider trait
- [x] Step 7: `ports/inbound/completion.rs` — CompletionUseCase trait
- [x] Step 8: `domain/service.rs` — CompletionService（プロバイダー選択ロジック）
- [x] Step 9: `adapters/outbound/env_key.rs` — EnvKeyStore
- [x] Step 10: `adapters/outbound/openai.rs` — OpenAIProvider
- [x] Step 11: `adapters/inbound/rest/` — REST API (handlers, router, DTO)
- [x] Step 12: `main.rs` — エントリポイント、DI組み立て
- [x] Step 13: テスト — ユニットテスト全9件パス

## 検証結果

- [x] `cargo build` — コンパイル成功（警告3件: 未使用のKeyStore関連、将来フェーズで利用予定）
- [x] `cargo test` — 9テスト全パス
- [ ] `cargo run` + `curl /health` — 手動確認待ち
- [ ] `curl /models` — 手動確認待ち
- [ ] OpenAI API経由の補完テスト — APIキー設定後に確認
