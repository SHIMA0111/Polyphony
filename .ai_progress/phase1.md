# Phase 1: プロジェクト骨格 + 最小AIチャット

**ゴール**: 1ユーザーがブラウザからAIと会話できる最小限のアプリケーション

---

## LLM Gateway (Rust)

- [ ] Cargoプロジェクト初期化（`llm-gateway/`）
- [ ] `domain/model.rs` — CompletionRequest, CompletionResponse, ModelInfo型
- [ ] `domain/error.rs` — ドメインエラー型
- [ ] `domain/service.rs` — LLM呼び出しドメインサービス
- [ ] `ports/inbound/completion.rs` — CompletionUseCase trait
- [ ] `ports/outbound/provider.rs` — LLMProvider trait
- [ ] `ports/outbound/key_store.rs` — APIキー取得 trait
- [ ] `adapters/outbound/openai.rs` — OpenAI APIアダプター
- [ ] `adapters/outbound/env_key.rs` — 環境変数からのAPIキー取得
- [ ] `adapters/inbound/rest/` — REST APIハンドラ（axum）
  - [ ] `POST /completions` — チャット補完
  - [ ] `GET /models` — 利用可能モデル一覧
  - [ ] `GET /health` — ヘルスチェック
- [ ] `config.rs` — アプリケーション設定
- [ ] `main.rs` — DI組み立て + サーバー起動
- [ ] ユニットテスト（モックプロバイダー）
- [ ] 動作確認（curl等でローカルテスト）

## Go API Server

- [ ] Goモジュール初期化（`server/`）
- [ ] `cmd/api/main.go` — エントリポイント、DI組み立て
- [ ] `internal/infrastructure/config/config.go` — 設定読み込み
- [ ] `internal/infrastructure/database/postgres.go` — DBコネクション
- [ ] **domain層**
  - [ ] `domain/user/entity.go` — User構造体
  - [ ] `domain/user/repository.go` — UserRepository interface
  - [ ] `domain/auth/service.go` — AuthService interface
  - [ ] `domain/room/entity.go` — Room, RoomMember構造体
  - [ ] `domain/room/repository.go` — RoomRepository interface
  - [ ] `domain/message/entity.go` — Message, MessageType構造体
  - [ ] `domain/message/repository.go` — MessageRepository interface
  - [ ] `domain/ai/entity.go` — AIRequest, AIResponse構造体
  - [ ] `domain/ai/gateway.go` — LLMGateway interface
- [ ] **usecase層**
  - [ ] `usecase/auth/usecase.go` — Register, Login
  - [ ] `usecase/room/usecase.go` — CreateRoom, GetRoom, ListRooms
  - [ ] `usecase/message/usecase.go` — SendMessage, ListMessages（カーソルページネーション）, SendAIMessage
- [ ] **interface層**
  - [ ] `interface/handler/auth_handler.go` — POST /auth/register, POST /auth/login
  - [ ] `interface/handler/room_handler.go` — CRUD API
  - [ ] `interface/handler/message_handler.go` — メッセージ送信・取得API
  - [ ] `interface/middleware/auth.go` — JWT検証ミドルウェア
  - [ ] `interface/repository/postgres/user_repository.go`
  - [ ] `interface/repository/postgres/room_repository.go`
  - [ ] `interface/repository/postgres/message_repository.go`
  - [ ] `interface/gateway/llm_client.go` — LLM Gateway RESTクライアント
- [ ] SimpleJWT実装（argon2 + JWT発行/検証）
- [ ] ユニットテスト（モックRepository/Gateway）
- [ ] 動作確認

## Web Frontend (Next.js)

- [ ] Next.jsプロジェクト初期化（`web/`）、Bulletproof Reactディレクトリ構造
- [ ] `src/lib/api.ts` — APIクライアント設定
- [ ] `src/features/auth/` — ログイン・登録
  - [ ] `components/LoginForm.tsx`
  - [ ] `components/RegisterForm.tsx`
  - [ ] `api/actions.ts` — Server Actions
- [ ] `src/features/rooms/` — ルーム一覧・作成
  - [ ] `components/RoomList.tsx`
  - [ ] `components/CreateRoomForm.tsx`
  - [ ] `api/actions.ts`
- [ ] `src/features/messages/` — チャット画面
  - [ ] `components/MessageList.tsx`
  - [ ] `components/MessageInput.tsx`
  - [ ] `api/actions.ts`
- [ ] `src/app/` — ルーティング
  - [ ] `(auth)/login/page.tsx`
  - [ ] `(auth)/register/page.tsx`
  - [ ] `(main)/rooms/page.tsx`
  - [ ] `(main)/rooms/[roomId]/page.tsx`
- [ ] 動作確認

## DB マイグレーション

- [ ] `migrations/` — golang-migrate初期マイグレーション
  - [ ] `users`テーブル
  - [ ] `rooms`テーブル
  - [ ] `room_members`テーブル
  - [ ] `messages`テーブル
  - [ ] `room_sequences`テーブル

## インフラ

- [ ] `docker-compose.yml`（PostgreSQL + Go API + Rust LLM Gateway + Next.js）
- [ ] 各サービスのDockerfile
- [ ] `.env.example`

## 結合テスト

- [ ] Docker Compose起動 → 登録 → ログイン → ルーム作成 → メッセージ送信 → AI応答取得の一連フロー確認
