# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

マルチユーザー対応AIチャットアプリケーション。ChatGPTライクなAIチャットとLINE/Slackライクなマルチユーザーチャットルームを統合する。

詳細な開発フェーズ計画・アーキテクチャ・スコープは `phases.md` を参照。

### 差別化要素

- マルチプロバイダーAI統合（OpenAI, Anthropic, Gemini, Ollama, vLLM）
- チームコラボレーション（ユーザーとAIが共存するチャットルーム）
- ルーム単位のRBAC（Reader → Guest → Member → Admin → Master）

## Architecture (4 Components)

1. **Web Frontend** — Next.js App Router + Bulletproof Reactアーキテクチャ
2. **Mobile Frontend** — Flutter (iOS/Android), Riverpod状態管理
3. **Main API Server** — Go + Echo, クリーンアーキテクチャ, WebSocket via Redis Pub/Sub
4. **LLM Gateway** — Rust, ヘキサゴナルアーキテクチャ (Ports & Adapters), gRPC/REST

**データ層**: PostgreSQL単一DB（メッセージは月次パーティショニング）、Redis（セッション、Pub/Sub、レート制限）、S3 + CloudFront（画像、presigned URL）
**認証**: 初期はSimpleJWT（argon2+JWT）→ Phase 9でOry Kratos + Phase 15でOry Hydra（OAuth2/OIDC）に差し替え
**インフラ**: 初期はDocker Compose → Phase 21でAWS（ECS Fargate, Aurora Serverless v2, ElastiCache）、Terraform、GitHub Actions CI/CD

## ディレクトリ構造

各コンポーネントの詳細なディレクトリ構造は `phases.md` を参照。

- `server/` — Go APIサーバー（クリーンアーキテクチャ: domain → usecase → interface/infrastructure）
- `llm-gateway/` — Rust LLM Gateway（ヘキサゴナル: domain/ports/adapters）
- `web/` — Next.js Web Frontend（Bulletproof React: app/features/components/hooks/lib）
- `migrations/` — golang-migrate
- `phases.md` — 開発フェーズ計画（26フェーズ）
- `.ai_progress/` — フェーズ別実装チェックリスト

## 進捗管理

各フェーズの実装開始時に `.ai_progress/phaseX.md` を作成し、タスクをチェックリストで管理する。

- 実装着手前にチェックリストを作成し、スコープを明確化する
- タスク完了時に `[x]` でチェックを入れる
- フェーズ完了時に全項目がチェック済みであることを確認する

## Development Conventions

- **言語**: ドキュメント・コメントは日本語、コード識別子は英語
- **Go docstrings**: GoDoc形式。全公開シンボルに記載。what/why/制約/エラー動作を記述
- **Rust docstrings**: rustdoc (`///`)。全公開シンボルに `# Arguments`, `# Returns`, `# Errors` セクション
- **クリーンアーキテクチャ**: domain層はinfrastructureパッケージをimportしない。層間通信は全てinterface経由
- **CQRS-lite**: 読み取り/書き込みクエリパスを分離、ただしDB単一
- **イベント駆動**: メッセージ送信・ユーザーアクションはイベントを発行。WebSocket配信・AI呼び出しはイベントハンドラで処理
- **テスト**: Repository/Gatewayをモックしたユニットテスト、testcontainers統合テスト（PostgreSQL）、Playwright E2E（web）、Flutter統合テスト、k6負荷テスト
- **DBマイグレーション**: golang-migrate
- **構造化ログ**: Go は `slog`、Rust は `tracing`。JSON形式
- **トレーシング**: OpenTelemetry、リクエストID伝播

## Key Domain Logic

**AIコンテキスト構築**（最複雑部分）: ルーム内でAI呼び出し時、メッセージをフィルタ（除外フラグ、カットオフ日時、削除済み、プライベート可視性）→ LLM Gatewayでトークンカウント → 上限超過時は古い履歴をAI要約してキャッシュ（`message_context_summaries`）。Phase 5, 18で実装。

**ルームフォーク**: 非同期バッチジョブでメッセージを新ルームにコピー（1000件/バッチ）、連番再採番、完了まで`is_archived=true`。Phase 20で実装。

**トークン課金**: ルームmasterが支払い。3つの決済レール（Stripe, App Store, Google Play）が`token_balances`に集約。AIリクエスト前に残高チェック。Phase 16-17, 25で実装。

## インターフェース差し替えポイント

| 抽象化 | 初期実装 | 差し替え時期 |
|--------|----------|-------------|
| `AuthService` | SimpleJWT (argon2+JWT) | Phase 9 (Kratos) |
| `MessageHub` | InProcessHub | Phase 10 (Redis) |
| `LLMClient` | REST client | Phase 8 (gRPC) |
| インフラ | Docker Compose | Phase 21 (AWS) |
