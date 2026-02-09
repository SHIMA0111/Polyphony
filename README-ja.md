# Polyphony

> [!WARNING]
> このプロジェクトは開発中であり、本番環境での使用はまだ準備が整っていません。

PolyphonyはLLMをネイティブ統合したチャットアプリケーションです。

ユーザーはPolyphony上で他のユーザーとチャットを行いながら、会話履歴を元にLLMとの対話を行うこともできます。複数のユーザーとLLMが同時に一つのルームでチャットでき、ルームごとにロールを設定してユーザーを招待できます。AIを呼び出せないチャット専用のGuest、投稿はできないが内容を閲覧できるReaderなど、柔軟なアクセス制御を備えています。

LLMを活用してチームの思考を拡張する、次世代チャットアプリケーションです。

[English](README.md)

## 特徴

- **マルチプロバイダーAI統合** — OpenAI, Anthropic, Gemini, Ollama, vLLM
- **チームコラボレーション** — ユーザーとAIが共存するチャットルーム
- **ルーム単位のRBAC** — Reader → Guest → Member → Admin → Master

## アーキテクチャ

| コンポーネント | 技術スタック |
|--------------|------------|
| Web Frontend | Next.js App Router, Bulletproof React |
| Mobile | Flutter, Riverpod |
| API Server | Go, Echo, WebSocket, Redis Pub/Sub |
| LLM Gateway | Rust, axum, ヘキサゴナルアーキテクチャ |

**データ層**: PostgreSQL / Redis / S3 + CloudFront
**インフラ**: Docker Compose → AWS (ECS Fargate, Aurora, ElastiCache)

## ディレクトリ構成

```
server/        — Go APIサーバー（クリーンアーキテクチャ）
llm-gateway/   — Rust LLM Gateway（Ports & Adapters）
web/           — Next.js Web Frontend
mobile/        — Flutter Mobile App
migrations/    — DBマイグレーション（golang-migrate）
phases.md      — 開発フェーズ計画（26フェーズ）
.ai_progress/  — フェーズ別実装チェックリスト
```

## 開発

### 前提条件

- Rust 1.93+
- Go 1.24+
- Node.js 22+
- Flutter 3+
- Docker / Docker Compose
- PostgreSQL 17 / Redis 7

### LLM Gateway

```bash
cd llm-gateway
cargo build
cargo test

# サーバー起動
OPENAI_API_KEY=sk-... cargo run
# → http://localhost:8081/health
```

## ライセンス

[AGPL-3.0](LICENSE.md)
