# Polyphony

> [!WARNING]
> このプロジェクトは開発中であり、本番環境での使用はまだ準備が整っていません。

PolyphonyはLLMをネイティブ統合したチャットアプリケーションです。

ユーザーはPolyphony上で他のユーザーとチャットを行いながら、会話履歴を元にLLMとの対話を行うこともできます。複数のユーザーとLLMが同時に一つのルームでチャットでき、ルームごとにロールを設定してユーザーを招待できます。AIを呼び出せないチャット専用のGuest、投稿はできないが内容を閲覧できるReaderなど、柔軟なアクセス制御を備えています。

LLMを活用してチームの思考を拡張する、次世代チャットアプリケーションです。

[English](README.md)

## 特徴

このリポジトリは [`phases.md`](phases.md) に定義された「Web complete version」計画のフェーズ2〜20を完全に実装し、
E2Eテストで検証済みです（フェーズごとの状態は [`phases.md` の Phase Overview](phases.md#phase-overview)、当初計画からの
意図的な差分は [Deviations from this plan](phases.md#deviations-from-this-plan) セクションを参照）。

- **マルチプロバイダーAI統合** — OpenAI・Anthropic・Gemini、ルーム単位のモデル選択、トークン/コンテキストメタデータ
  （コンテキストウィンドウ、100万トークンあたりの料金、Vision対応）、画像/Vision入力
- **リアルタイムチームコラボレーション** — WebSocketによるルーム配信（複数インスタンス対応のRedis Pub/Sub）、
  トークン単位でのストリーミングAI応答
- **ルーム単位のRBAC** — 5段階ロール（Reader → Guest → Member → Admin → Master）、招待、メンバー/グループ管理、
  オーナー権限移譲
- **本番グレードの認証** — Ory Kratosによるセッション管理（メール/パスワード + ローカル`dex`モックによるOIDC
  ソーシャルログイン）、ファーストパーティOAuth2/OIDCプロバイダーとしてのOry Hydra、Redisによるレート制限・
  セッションキャッシュ
- **AIコンテキスト制御** — メッセージ単位のAI除外フラグ、論理削除、コンテキストカットオフ日時、プライベートAI
  モード（送信者のみに応答を表示）、コンテキストがモデルのウィンドウを超えた場合の自動履歴要約
- **トークン課金** — ルーム単位のトークン残高管理、利用履歴、Stripe（テストモード）によるサブスクリプション/
  従量課金トークン購入
- **ルームフォーク** — ルームの全履歴を新しい独立ルームへ非同期バッチコピー

## アーキテクチャ

| コンポーネント | 技術スタック |
|--------------|------------|
| Web Frontend | Next.js App Router, Bulletproof React |
| Mobile | Flutter, Riverpod |
| API Server | Go, Echo, WebSocket, Redis Pub/Sub |
| LLM Gateway | Rust, axum, ヘキサゴナルアーキテクチャ |

**データ層**: PostgreSQL / Redis / S3（ローカルはMinIO） + CloudFront
**認証**: Ory Kratos（セッション、OIDCソーシャルログイン） + Ory Hydra（OAuth2/OIDCプロバイダー）、`AUTH_MODE`で
切り替え可能（`simple_jwt`もサポート）
**インフラ**: Docker Compose → AWS (ECS Fargate, Aurora, ElastiCache)

> [!NOTE]
> **Mobile** の行およびAWS/Terraform/CI-CDへのインフラ移行は `phases.md` のフェーズ21〜25に対応しており、
> プロジェクトの方針決定により**このリポジトリの実装スコープから意図的に除外**されています —
> このコードベースには `mobile/` ディレクトリもTerraformも存在しません。フェーズごとの状態は
> [`phases.md` の Phase Overview](phases.md#phase-overview) を参照してください。

## ディレクトリ構成

```text
server/            — Go APIサーバー（クリーンアーキテクチャ）
llm-gateway/       — Rust LLM Gateway（Ports & Adapters）
web/               — Next.js Web Frontend
web/e2e/           — Playwright E2Eハーネス（スモーク + 機能別spec + リグレッションスイート）
ory/               — Ory Kratos/Hydra/dex 設定（認証、OIDC、モックソーシャルログイン）
llm-stub/          — E2Eテストプロファイルで使用する疑似OpenAI互換LLMスタブ
server/migrations/ — DBマイグレーション（Atlas）
docker-compose.yml — ローカル開発環境（+ E2E用の分離された `test` プロファイル）
Taskfile.yml       — タスクランナー（go-task）
phases.md          — 開発フェーズ計画（26フェーズ。フェーズ2〜20をこのリポジトリで実装）
docs/tasks/        — このコードベースの実装元となったステップ単位のビルド計画（step1〜60）
.ai_progress/      — フェーズ別実装チェックリスト
```

`mobile/`（Flutter、フェーズ24〜25）はこのリポジトリには存在しません — 上記の注記を参照してください。

## 開発

### 前提条件

- Rust 1.93+
- Go 1.24+
- Bun 1.2+
- Flutter 3+
- Docker / Docker Compose
- PostgreSQL 17 / Redis 7
- [go-task](https://taskfile.dev/)（任意、`task` コマンド用）

### クイックスタート（Docker Compose）

```bash
# 全サービス起動（PostgreSQL, Redis, Go API, LLM Gateway, Web Frontend）
task up
# または: docker compose up -d

# ログ確認
task logs
```

### 個別サービス

```bash
# LLM Gateway
cd llm-gateway
cargo build && cargo test
OPENAI_API_KEY=sk-... cargo run
# → http://localhost:8081/health

# Go API Server
cd server
go run ./cmd/api
# → http://localhost:8080

# Web Frontend
cd web
bun install && bun run dev
# → http://localhost:3000
```

### E2Eスイートの実行（Playwright）

通常の開発スタックとは別に、分離されたDocker Composeスタック（`test` プロファイル）が別ポート
（`5433`、`8090`、`8091`、`8092`、`3001`、および認証系specのための専用の`kratos-e2e`/`dex-e2e`/`hydra-e2e`ポート）
上で動作し、開発スタックと競合しません。実プロバイダーの代わりに決定的なOpenAI互換LLMスタブ（`llm-stub/`）を
使用します:

```bash
# クリーンな状態からのフルゲート: 残っていれば停止 → 起動 → 全体実行
task test:e2e:down   # 何も動いていない場合の "no such service" エラーは無視してよい
task test:e2e:up     # Postgres, migrations, API, LLM Gateway, LLM stub, Web, Kratos/Hydra/dex
task test:e2e        # Playwrightスイート全体を実行。globalSetupでフィクスチャを自動seedする
task test:e2e:down   # ボリュームを含めて停止
```

完全にグリーンな実行では、Playwrightのターミナルサマリー行で `chromium`/`rate-limiting` の両プロジェクトにわたり
失敗0件・リトライ後もflakyなし（例: `X passed (Ys)`）と報告されます（`rate-limiting` specが独立した後発
プロジェクトで動く理由は `web/playwright.config.ts` のコメントを参照）。このスイートはStep 10のスモークspec、
各機能領域のspec（ルーム、メンバー、グループ、招待、OAuth、添付/Vision、課金、ルームフォーク、プライベート
モード、ストリーミング、AIコンテキスト制御）、および `web/e2e/regression/` 以下の4つのリグレッションスイート
（リアルタイムAIコア、高度なAI、課金/フォーク、アイデンティティ/コラボレーション）をすべてカバーします。

`task test:e2e:seed` はフィクスチャseedスクリプトだけを単独で再実行します（冪等 — seed済みのスタックに対しても
安全に実行可能）。ハーネス全体の設計は `docs/tasks/step10.md` を参照してください。

#### 単一specの分離実行（高速なイテレーション/デフレーク）

`task test:e2e:up` でスタックを起動した後は、フルスイートの代わりに `web/` から個別のspecファイル
（またはglob）を直接実行できます:

```bash
cd web
bunx playwright test e2e/rooms.spec.ts
bunx playwright test e2e/regression/ai-send-model-select.spec.ts
bunx playwright test e2e/regression/rate-limiting.spec.ts --no-deps   # chromiumプロジェクトへの依存をスキップ
```

`task test:e2e` は**唯一の権威あるフルスイートゲート**です — `task test`／`task test:server`／`task test:gateway`
は高速でCompose不要な単体テストのみのタスクとして、意図的にこのタスクに依存していません（`Taskfile.yml` 参照）。

#### 課金（Stripe）のE2Eカバレッジ

`test` Composeプロファイルには `STRIPE_*` の設定もWebhook転送サービスもないため、
`web/e2e/regression/billing.spec.ts` のStripe Checkout/Webhook/サブスクリプション解約のジャーニーは、デフォルトの
ローカル実行では自己スキップします（実際の認証情報なしで到達可能な部分 — プランカタログ、空状態、エラー
パス — はアサートされます）。これは失敗ではなく、受け入れられた定常状態です。ローカルで完全なCheckout
ジャーニーを実行するために必要な環境変数と `stripe listen` のセットアップは
[`web/e2e/regression/README.md`](web/e2e/regression/README.md) を参照してください。

## ライセンス

[AGPL-3.0](LICENSE.md)
