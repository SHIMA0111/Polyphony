# 開発フェーズ計画

## アプリケーション概要

マルチユーザー対応AIチャットアプリケーション。ChatGPTライクなAIチャットとLINE/Slackライクなマルチユーザーチャットルームを統合する。

### 差別化要素

- **マルチプロバイダーAI統合** — OpenAI, Anthropic, Gemini, Ollama, vLLM
- **チームコラボレーション** — ユーザーとAIが共存するチャットルーム
- **ルーム単位のRBAC** — Reader → Guest → Member → Admin → Master

### 構成コンポーネント（4つ）

1. **Web Frontend** — Next.js App Router + Bulletproof Reactアーキテクチャ
2. **Mobile Frontend** — Flutter (iOS/Android), Riverpod状態管理
3. **Main API Server** — Go + Echo, クリーンアーキテクチャ, WebSocket via Redis Pub/Sub
4. **LLM Gateway** — Rust, ヘキサゴナルアーキテクチャ (Ports & Adapters), gRPC/REST

### データ層・インフラ

- **DB**: PostgreSQL単一DB（メッセージは月次パーティショニング）
- **キャッシュ/Pub/Sub**: Redis（セッション、Pub/Sub、レート制限）
- **ストレージ**: S3 + CloudFront（画像、presigned URL）
- **認証**: 初期はSimpleJWT（argon2+JWT）→ Phase 9でOry Kratos → Phase 15でOry Hydra
- **インフラ**: 初期はDocker Compose → Phase 21でAWS（ECS Fargate, Aurora, ElastiCache）

## 方針: 1フェーズ = 1テーマ

本計画では **1フェーズ = 1テーマ** の小さな更新を積み重ねる。全26フェーズで段階的にアプリケーションを構築する。

### 基本原則

- **小さく積み重ねる**: 各フェーズは1〜2週間で完了できる粒度にする。前フェーズの成果物が次フェーズの土台になる
- **インターフェースで差し替え可能にする**: 初期実装はシンプルに、将来の差し替えはインターフェース境界で行う。最初から本番品質を目指さない
- **動くものを常に維持する**: 各フェーズ完了時点でアプリケーションが動作する状態を保つ

---

## Go APIサーバーアーキテクチャ: クリーンアーキテクチャ

Go APIサーバーはクリーンアーキテクチャに従い、依存方向を domain → usecase → interface/infrastructure に厳守する。

```
server/
├── cmd/
│   └── api/
│       └── main.go                    # エントリポイント、DI組み立て
├── internal/
│   ├── domain/                        # ドメイン層（外部依存なし）
│   │   ├── user/
│   │   │   ├── entity.go              # User構造体
│   │   │   ├── repository.go          # UserRepository interface
│   │   │   └── service.go             # ドメインロジック
│   │   ├── room/
│   │   │   ├── entity.go              # Room, RoomMember, RoomRole
│   │   │   ├── repository.go          # RoomRepository interface
│   │   │   └── service.go
│   │   ├── message/
│   │   │   ├── entity.go              # Message, MessageType
│   │   │   ├── repository.go
│   │   │   └── service.go
│   │   ├── auth/
│   │   │   └── service.go             # AuthService interface（★差し替えポイント）
│   │   └── ai/
│   │       ├── entity.go              # AIRequest, AIResponse
│   │       ├── context_builder.go     # コンテキスト構築ロジック
│   │       └── gateway.go             # LLMGateway interface（★差し替えポイント）
│   ├── usecase/                       # ユースケース層
│   │   ├── auth/usecase.go            # Register, Login
│   │   ├── room/usecase.go            # CreateRoom, JoinRoom
│   │   ├── message/usecase.go         # SendMessage, SendAIMessage
│   │   └── ai/usecase.go             # InvokeAI, BuildContext
│   ├── interface/                     # アダプター層（外側）
│   │   ├── handler/                   # Echo HTTPハンドラ
│   │   │   ├── auth_handler.go
│   │   │   ├── room_handler.go
│   │   │   ├── message_handler.go
│   │   │   └── ws_handler.go          # WebSocket（Phase 2で追加）
│   │   ├── middleware/
│   │   │   ├── auth.go                # JWT検証 → Phase 9でKratosセッション検証に差し替え
│   │   │   └── rbac.go                # Phase 4で追加
│   │   ├── repository/
│   │   │   └── postgres/              # PostgreSQL実装（全テーブル）
│   │   └── gateway/
│   │       └── llm_client.go          # LLM Gateway REST/gRPCクライアント
│   └── infrastructure/
│       ├── database/postgres.go       # コネクションプール
│       ├── websocket/                 # Phase 2で追加
│       │   ├── hub.go                 # MessageHub interface + InProcessHub
│       │   ├── client.go
│       │   └── event.go
│       └── config/config.go
├── migrations/                        # golang-migrate
├── proto/                             # Phase 8で追加
└── go.mod
```

**原則**:
- `domain/`は標準ライブラリ以外に依存しない
- Repository/Service/Gatewayは全てinterfaceとして`domain/`で定義、実装は`interface/`か`infrastructure/`
- ユースケース層はドメインのinterfaceのみ参照（具象型を知らない）
- DI組み立ては`cmd/api/main.go`で行う

---

## フロントエンドアーキテクチャ: Bulletproof React

Web FrontendはBulletproof Reactのアーキテクチャに従う。

```
web/
├── src/
│   ├── app/              # Next.js App Router（ルーティング層のみ）
│   │   ├── layout.tsx    # ルートレイアウト
│   │   ├── (auth)/       # 認証系ルートグループ
│   │   │   ├── login/page.tsx
│   │   │   └── register/page.tsx
│   │   └── (main)/       # メインアプリルートグループ
│   │       ├── layout.tsx
│   │       ├── rooms/page.tsx
│   │       └── rooms/[roomId]/page.tsx
│   ├── components/       # 共有コンポーネント
│   ├── config/           # グローバル設定
│   ├── features/         # 機能ごとのモジュール（★中心）
│   │   ├── auth/         # 認証（api/, components/, hooks/, types/）
│   │   ├── rooms/        # ルーム管理
│   │   ├── messages/     # メッセージ
│   │   ├── ai/           # AI呼び出し
│   │   └── ...
│   ├── hooks/            # 共有フック
│   ├── lib/              # ライブラリ設定済みラッパー
│   ├── stores/           # グローバルステート
│   ├── testing/          # テストユーティリティ
│   ├── types/            # 共有型定義
│   └── utils/            # 共有ユーティリティ
└── public/               # 静的ファイル
```

**Next.js App Router との共存ルール**:
- `app/`ディレクトリはルーティングとレイアウトのみ。ビジネスロジックは置かない
- 各`page.tsx`は対応する`features/`のコンポーネントをimportして表示するだけの薄いラッパー
- Server Components/Server Actionsは`features/`内の`api/`ディレクトリに定義

**原則**:
- feature間の直接importは禁止（共有部品は`components/`, `hooks/`, `lib/`経由）
- 依存方向は一方向: shared → features → app
- barrel file（index.ts）は使わない（tree-shaking最適化のため）

---

## LLM Gatewayアーキテクチャ: ヘキサゴナルアーキテクチャ (Ports & Adapters)

LLM Gateway（Rust）はヘキサゴナルアーキテクチャを採用し、シングルトンファイルへの肥大化を防ぐ。

```
llm-gateway/src/
├── domain/                # ドメイン層（純粋なビジネスロジック）
│   ├── model.rs           # CompletionRequest, CompletionResponse, ModelInfo等
│   ├── error.rs           # ドメインエラー型
│   └── service.rs         # LLM呼び出しのドメインサービス
├── ports/                 # ポート定義（trait）
│   ├── inbound/           # 駆動側ポート（外→内）
│   │   └── completion.rs  # CompletionUseCase trait
│   └── outbound/          # 被駆動側ポート（内→外）
│       ├── provider.rs    # LLMProvider trait（各AIプロバイダー）
│       └── key_store.rs   # APIキー取得 trait
├── adapters/              # アダプター実装
│   ├── inbound/           # 駆動側アダプター
│   │   ├── grpc/          # gRPCサーバー（Phase 8で追加）
│   │   └── rest/          # REST APIハンドラ（axum/actix-web）
│   └── outbound/          # 被駆動側アダプター
│       ├── openai.rs      # OpenAI API adapter
│       ├── anthropic.rs   # Anthropic API adapter（Phase 6）
│       ├── gemini.rs      # Gemini API adapter（Phase 7）
│       └── env_key.rs     # 環境変数からのAPIキー取得
├── config.rs              # アプリケーション設定
└── main.rs                # エントリポイント（DI組み立て）
```

**原則**:
- `domain/`と`ports/`は外部クレートに依存しない（`serde`等のみ許容）
- プロバイダー追加は`adapters/outbound/`にファイル追加 + `main.rs`でDI登録のみ
- inboundアダプター（REST/gRPC）は`ports/inbound/`のtraitを呼ぶだけ

---

## フェーズ一覧

| # | テーマ | ゴール |
|---|--------|--------|
| 1 | プロジェクト骨格 + 最小AIチャット | 1ユーザーがAIと会話できる |
| 2 | WebSocket + リアルタイム | メッセージがリアルタイムに表示される |
| 3 | 招待 + メンバー管理 | ルームにユーザーを招待できる |
| 4 | 基本RBAC | reader/member/masterの3ロール |
| 5 | AIコンテキスト制御 | メッセージのAI除外、トークン推定 |
| 6 | マルチプロバイダー (Anthropic) | 2つ目のAIプロバイダー |
| 7 | マルチプロバイダー (Gemini) + モデル選択UI | ユーザーがモデルを選べる |
| 8 | gRPC化 + LLM Gateway堅牢化 | リトライ、ヘルスチェック、モデルメタデータ |
| 9 | Ory Kratos導入 | 本格認証基盤に移行 |
| 10 | Redis + マルチインスタンス | WebSocket Pub/Sub、レート制限 |
| 11 | フルRBAC | guest/admin追加で5段階権限 |
| 12 | 画像アップロード + Vision | S3、マルチモーダルAI |
| 13 | グループ管理 + 一括招待 | グループでまとめて招待 |
| 14 | プライベートAIモード | 自分だけに見えるAI応答 |
| 15 | OAuthソーシャルログイン | Google/GitHub連携 |
| 16 | トークン残高管理 | 消費記録、残高チェック |
| 17 | Stripe課金 | サブスクリプション + オンデマンド購入 |
| 18 | コンテキスト要約 | 古い履歴をAIで要約→キャッシュ |
| 19 | ストリーミングAIレスポンス | トークンが順次表示される |
| 20 | ルームフォーク | 非同期バッチコピー |
| 21 | AWS基盤 (Terraform) | VPC、ECS、Aurora、ElastiCache |
| 22 | CI/CD + ステージング | GitHub Actions、ECR、ECSデプロイ |
| 23 | 監視 + 本番運用 | CloudWatch、WAF、パーティショニング、k6 |
| 24 | Flutter モバイルアプリ | iOS/Android対応 |
| 25 | モバイル課金 + プッシュ通知 | App Store/Google Play連携 |
| 26+ | エンタープライズ（将来） | SSO/SAML、管理ダッシュボード、Ollama/vLLM |

---

## インターフェースによる差し替えポイント

以下の抽象化により、初期実装をシンプルに保ちつつ、後のフェーズで本番品質の実装に差し替える。

| 抽象化 | 初期実装 | 差し替え時期 | 説明 |
|--------|----------|-------------|------|
| `AuthService` | SimpleJWT (argon2+JWT) | Phase 9 (Kratos) | 認証・セッション管理。Phase 1ではargon2でパスワードハッシュ、JWTでトークン発行。Phase 9でOry Kratosのセッション検証に差し替え |
| `MessageHub` | InProcessHub | Phase 10 (Redis) | WebSocketメッセージ配信。Phase 2ではインプロセスHub、Phase 10でRedis Pub/Subに差し替えてマルチインスタンス対応 |
| `LLMClient` | REST client | Phase 8 (gRPC) | LLM Gateway通信。Phase 1ではシンプルなRESTクライアント、Phase 8でgRPCクライアントに差し替え |
| インフラ | Docker Compose | Phase 21 (AWS) | 実行環境。開発中はDocker Compose、Phase 21でTerraformによるAWS基盤（ECS Fargate、Aurora、ElastiCache）に移行 |

---

## 各フェーズ詳細

### Phase 1: プロジェクト骨格 + 最小AIチャット

**ゴール**: 1ユーザーがブラウザからAIと会話できる最小限のアプリケーション

#### Go API

- クリーンアーキテクチャ骨格（domain/usecase/interface/infrastructure）
- `AuthService`インターフェース + SimpleJWT実装（argon2 + JWT）
- ユーザー登録・ログインAPI
- ルームCRUD API
- メッセージ永続化 + カーソルページネーション
- AI呼び出し（LLM Gateway RESTクライアント → 同期レスポンス → DB保存。LLM失敗時は `status=failed` のプレースホルダーAIメッセージを保存）
- AI応答再生成API（`POST /rooms/:roomId/messages/:messageId/regenerate` — 指定humanメッセージ直後のAIメッセージを常にUPDATEで上書き。新規作成パスなし。sequence/created_atを保持するため順序が崩れない）

#### LLM Gateway (Rust)

- ヘキサゴナルアーキテクチャ骨格
- `domain/` — CompletionRequest/Response型、ドメインエラー
- `ports/outbound/provider.rs` — `LLMProvider` trait定義
- `ports/inbound/completion.rs` — `CompletionUseCase` trait定義
- `adapters/outbound/openai.rs` — OpenAIアダプター
- `adapters/inbound/rest/` — REST APIハンドラ（`/completions`, `/models`, `/health`）
- `main.rs` — DI組み立て

#### Web Frontend (Next.js)

- Next.js App Router + Bulletproof Reactディレクトリ構造
- SSR（Server Components）+ Server Actions
- `features/auth/` — ログイン・登録ページ
- `features/rooms/` — ルーム一覧
- `features/messages/` — チャット画面（ポーリングまたは手動リロード、リアルタイムなし）

#### DB変更

- `users` — ユーザー基本情報、パスワードハッシュ
- `rooms` — ルーム情報
- `room_members` — ルームメンバーシップ
- `messages` — メッセージ本文、送信者、タイプ（human/ai）、ステータス（completed/failed）
- `room_sequences` — ルーム内メッセージ連番

#### インフラ

- Docker Compose（PostgreSQL + Go API + Rust LLM Gateway + Next.js）

#### 除外

- WebSocket（Phase 2）
- RBAC（Phase 4）
- マルチプロバイダー（Phase 6〜7）
- 画像（Phase 12）
- 課金（Phase 16〜17）
- Redis（Phase 10）
- S3（Phase 12）

---

### Phase 2: WebSocket + リアルタイム

**ゴール**: メッセージ送信がリアルタイムに全メンバーへ配信される

#### スコープ

- Go API: WebSocketエンドポイント、`MessageHub`インターフェース + `InProcessHub`実装
- Web Frontend: WebSocket接続、メッセージのリアルタイム受信・表示
- イベント駆動: メッセージ送信 → Hub → 接続中クライアントへブロードキャスト

#### DB変更

なし

#### 除外

- Redis Pub/Sub（Phase 10）
- 複数インスタンス対応（Phase 10）

---

### Phase 3: 招待 + メンバー管理

**ゴール**: ルームオーナーがユーザーを招待でき、メンバー一覧を表示できる

#### スコープ

- Go API: 招待API、メンバー一覧API、招待承認/拒否
- Go API: オーナー移譲API（`PATCH /rooms/:roomId/owner`）— 現ownerが別メンバーにownershipを移譲。課金主体（`token_balances`）の移行にも必要
- Go API: メンバー退室API（`DELETE /rooms/:roomId/members/:userId`）— `room_members`のON DELETE RESTRICTにより、退室はアプリ側で明示的に処理
- Web Frontend: メンバー管理画面、招待フォーム、オーナー移譲UI

- 招待リンクまたはユーザー名検索による招待

#### DB変更

- `room_invitations` — 招待情報

#### 除外

- ロール管理（Phase 4）
- グループ一括招待（Phase 13）

---

### Phase 4: 基本RBAC

**ゴール**: reader/member/masterの3段階ロールで権限制御

#### スコープ

- Go API: RBACミドルウェア、ロール変更API
- ルーム作成者が自動的にmaster
- reader: メッセージ閲覧のみ（送信不可）
- member: メッセージ送信 + AI呼び出し可能
- master: メンバー管理、ルーム設定変更
- Web Frontend: ロール表示、master向け管理UI

#### DB変更

- `room_members`テーブルにroleカラム追加（既存データはmember）

#### 除外

- guest/adminロール（Phase 11）

---

### Phase 5: AIコンテキスト制御

**ゴール**: メッセージ単位でAI除外フラグを設定でき、トークン数を推定できる

#### スコープ

- Go API: メッセージにAI除外フラグ、コンテキスト構築ロジック（除外フラグ、削除済み、カットオフ日時を考慮）
- LLM Gateway: トークン推定エンドポイント
- Web Frontend: メッセージのAI除外トグル、推定トークン数表示

#### DB変更

- `messages`テーブルに`exclude_from_ai`カラム追加
- `rooms`テーブルに`ai_context_cutoff_at`カラム追加

#### 除外

- 履歴要約（Phase 18）
- プライベートAIモード（Phase 14）

---

### Phase 6: マルチプロバイダー (Anthropic)

**ゴール**: OpenAIに加えてAnthropicのモデルを利用できる

#### スコープ

- LLM Gateway: `adapters/outbound/anthropic.rs` — Anthropic APIアダプター
- Go API: ルーム設定でプロバイダー/モデル指定
- Web Frontend: モデル設定UI（ルーム設定内）

#### DB変更

- `rooms`テーブルに`ai_provider`, `ai_model`カラム追加

#### 除外

- Gemini（Phase 7）
- モデル選択のリッチUI（Phase 7）

---

### Phase 7: マルチプロバイダー (Gemini) + モデル選択UI

**ゴール**: 3プロバイダー対応、ユーザーが直感的にモデルを選べるUI

#### スコープ

- LLM Gateway: `adapters/outbound/gemini.rs` — Gemini APIアダプター
- LLM Gateway: `/models`エンドポイントで利用可能モデル一覧返却
- Web Frontend: モデル選択ドロップダウン（プロバイダー + モデル名）

#### DB変更

なし

#### 除外

- Ollama/vLLM（Phase 26+）

---

### Phase 8: gRPC化 + LLM Gateway堅牢化

**ゴール**: Go APIとLLM Gateway間をgRPC通信にし、リトライ・ヘルスチェックを追加

#### スコープ

- LLM Gateway: gRPCサーバー追加（`adapters/inbound/grpc/`）
- Go API: gRPCクライアントに差し替え（`LLMClient`インターフェースの実装差し替え）
- proto定義（`server/proto/`）
- リトライポリシー（指数バックオフ）
- ヘルスチェックエンドポイント
- モデルメタデータ（トークン上限、料金情報）

#### DB変更

なし

#### 除外

- ストリーミング（Phase 19）

---

### Phase 9: Ory Kratos導入

**ゴール**: 認証基盤をSimpleJWTからOry Kratosに移行

#### スコープ

- Ory Kratosコンテナ追加（Docker Compose）
- Go API: `AuthService`実装をKratosセッション検証に差し替え
- ミドルウェア: JWTヘッダ検証 → Kratosセッションcookie検証
- Web Frontend: ログイン/登録フローをKratos UIに合わせて修正
- 既存ユーザーデータ移行スクリプト

#### DB変更

- Kratos独自のDBスキーマ（Kratos管理）
- `users`テーブルに`kratos_identity_id`カラム追加

#### 除外

- OAuth/ソーシャルログイン（Phase 15）
- Ory Hydra（Phase 15）

---

### Phase 10: Redis + マルチインスタンス

**ゴール**: Redis導入によりWebSocket Pub/Sub対応、レート制限、セッションキャッシュ

#### スコープ

- Redisコンテナ追加（Docker Compose）
- Go API: `MessageHub`実装をRedis Pub/Subに差し替え
- レート制限ミドルウェア（Redis Token Bucket）
- セッションキャッシュ（Kratosセッション検証結果のキャッシュ）

#### DB変更

なし

#### 除外

- Redis Cluster構成（Phase 21）

---

### Phase 11: フルRBAC

**ゴール**: reader/guest/member/admin/masterの5段階権限

#### スコープ

- Go API: guestロール（メッセージ送信可、AI呼び出し不可）、adminロール（メンバー管理可、ルーム削除不可）
- RBACミドルウェア更新
- Web Frontend: ロール選択UI更新、権限に応じたUI表示制御

#### DB変更

- `room_members`テーブルのroleカラムに新しい値追加

#### 除外

なし

---

### Phase 12: 画像アップロード + Vision

**ゴール**: 画像をアップロードしてAIに画像認識させられる

#### スコープ

- S3（MinIO for local）+ presigned URL
- Go API: 画像アップロードAPI、presigned URL発行
- LLM Gateway: マルチモーダルリクエスト対応（Vision API）
- Web Frontend: 画像アップロードUI、画像プレビュー

#### DB変更

- `message_attachments` — ファイルメタ情報（S3キー、MIME type、サイズ）

#### 除外

- CloudFront（Phase 21）
- 動画・PDF等の他のファイル形式

---

### Phase 13: グループ管理 + 一括招待

**ゴール**: ユーザーグループを作成し、グループ単位でルームに招待できる

#### スコープ

- Go API: グループCRUD、グループメンバー管理、グループ単位招待
- Web Frontend: グループ管理画面、グループ招待UI

#### DB変更

- `groups` — グループ情報
- `group_members` — グループメンバーシップ

#### 除外

なし

---

### Phase 14: プライベートAIモード

**ゴール**: AI応答を自分だけに表示するモードを選択できる

#### スコープ

- Go API: メッセージに`visibility`フラグ（public/private）
- AIリクエスト時にprivateモード指定
- private応答は送信者にのみWebSocket配信
- Web Frontend: AIリクエスト時のプライベートモードトグル

#### DB変更

- `messages`テーブルに`visibility`カラム追加

#### 除外

なし

---

### Phase 15: OAuthソーシャルログイン

**ゴール**: Google/GitHubアカウントでログインできる

#### スコープ

- Ory Hydra導入（OAuth2/OIDCプロバイダー）
- Kratos設定: Google/GitHub OIDCプロバイダー追加
- Web Frontend: ソーシャルログインボタン

#### DB変更

- Kratos管理（外部プロバイダーリンク）

#### 除外

- SSO/SAML（Phase 26+）

---

### Phase 16: トークン残高管理

**ゴール**: AI利用のトークン消費を記録し、残高を管理できる

#### スコープ

- Go API: トークン消費記録、残高チェック（AIリクエスト前）、残高不足エラー
- ルームmasterが残高を持つ（ルーム課金モデル）
- Web Frontend: 残高表示、消費履歴

#### DB変更

- `token_balances` — ユーザー別残高
- `token_transactions` — トークン消費・チャージ履歴

#### 除外

- 決済連携（Phase 17）
- 初期残高付与の自動化

---

### Phase 17: Stripe課金

**ゴール**: Stripeでサブスクリプション購入・オンデマンドトークン購入ができる

#### スコープ

- Go API: Stripe Webhook処理、サブスクリプション管理、一回限り購入
- サブスクリプションプラン（月次トークン付与）
- Web Frontend: 料金プラン選択、Stripe Checkout連携、課金履歴

#### DB変更

- `subscriptions` — サブスクリプション情報
- `payment_history` — 決済履歴

#### 除外

- App Store/Google Play課金（Phase 25）

---

### Phase 18: コンテキスト要約

**ゴール**: 長い会話履歴をAIで要約し、コンテキストウィンドウに収める

#### スコープ

- Go API: コンテキスト構築時にトークン上限超過を検知
- 超過時: 古いメッセージをAIで要約 → `message_context_summaries`にキャッシュ
- 要約結果をコンテキストの先頭に挿入、個別メッセージは最新分のみ
- Web Frontend: 要約が使われていることの表示

#### DB変更

- `message_context_summaries` — 要約キャッシュ（ルーム、対象期間、要約テキスト、トークン数）

#### 除外

なし

---

### Phase 19: ストリーミングAIレスポンス

**ゴール**: AIの応答がトークン単位で逐次表示される

#### スコープ

- LLM Gateway: Server-Sent Events（SSE）またはgRPCストリーミング対応
- Go API: ストリーミングレスポンスをWebSocket経由でクライアントに転送
- Web Frontend: トークン逐次表示、タイピングインジケーター

#### DB変更

なし（ストリーミング完了後に既存のメッセージ保存フロー）

#### 除外

なし

---

### Phase 20: ルームフォーク

**ゴール**: 既存ルームの会話を別ルームにコピーして分岐できる

#### スコープ

- Go API: 非同期バッチジョブ（1000メッセージ/バッチでコピー）
- 連番再採番
- コピー完了まで`is_archived=true`で新規投稿をブロック
- Web Frontend: フォークボタン、進行状況表示

#### DB変更

- `rooms`テーブルに`forked_from_room_id`, `is_archived`カラム追加
- `room_fork_jobs` — フォークジョブ進行状況

#### 除外

なし

---

### Phase 21: AWS基盤 (Terraform)

**ゴール**: AWS上に本番環境を構築

#### スコープ

- Terraform: VPC、サブネット、セキュリティグループ
- ECS Fargate（Go API、LLM Gateway、Next.js、Kratos）
- Aurora Serverless v2（PostgreSQL互換）
- ElastiCache（Redis）
- S3 + CloudFront（画像配信）
- ALB + ACM（HTTPS）

#### DB変更

なし

#### 除外

- CI/CD（Phase 22）
- 監視（Phase 23）

---

### Phase 22: CI/CD + ステージング

**ゴール**: mainブランチへのマージで自動的にステージング環境へデプロイ

#### スコープ

- GitHub Actions: テスト → ビルド → ECRプッシュ → ECSデプロイ
- ステージング環境（Terraformワークスペース分離）
- DB マイグレーション自動実行
- E2Eテスト（Playwright）をステージングで実行

#### DB変更

なし

#### 除外

- 本番デプロイフロー（Phase 23）

---

### Phase 23: 監視 + 本番運用

**ゴール**: 本番運用に必要な監視・セキュリティ・パフォーマンス対策

#### スコープ

- CloudWatch Logs + メトリクス + アラーム
- AWS WAF（レート制限、SQLi/XSS防御）
- PostgreSQLメッセージテーブルの月次パーティショニング
- k6 負荷テスト
- OpenTelemetryトレース（X-Ray連携）
- 本番デプロイフロー（承認ゲート付き）

#### DB変更

- `messages`テーブルのパーティショニング設定

#### 除外

なし

---

### Phase 24: Flutter モバイルアプリ

**ゴール**: iOS/AndroidネイティブアプリでWebと同等の機能を利用できる

#### スコープ

- Flutter: Riverpod状態管理
- 認証（Kratosセッション）
- ルーム一覧、チャット画面、WebSocket接続
- 画像アップロード（カメラ/ギャラリー）

#### DB変更

なし

#### 除外

- App Store/Google Play課金（Phase 25）
- プッシュ通知（Phase 25）

---

### Phase 25: モバイル課金 + プッシュ通知

**ゴール**: アプリ内課金とプッシュ通知に対応

#### スコープ

- Flutter: App Store/Google Play課金連携（RevenueCat等）
- Go API: App Store Server Notifications / Google Play RTDN Webhook
- `token_balances`への残高反映（Stripe/App Store/Google Play共通）
- Firebase Cloud Messaging（FCM）によるプッシュ通知
- Go API: 通知送信ロジック（新メッセージ、招待、AI応答完了）

#### DB変更

- `device_tokens` — FCMデバイストークン
- `payment_history`テーブルにpayment_rail（stripe/app_store/google_play）追加

#### 除外

なし

---

### Phase 26+: エンタープライズ（将来）

**ゴール**: 大規模組織向け機能

#### スコープ（候補）

- SSO/SAML対応（Ory Hydra拡張）
- 管理ダッシュボード（利用状況、ユーザー管理、監査ログ）
- Ollama/vLLM対応（オンプレミスLLM）
- メッセージエクスポート
- データリテンションポリシー

#### DB変更

未定

#### 除外

未定
