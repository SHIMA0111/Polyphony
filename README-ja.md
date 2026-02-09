# Polyphony

> [!WARNING]
> このプロジェクトは開発中であり、本番環境での使用はまだ準備が整っていません。

*ポリフォニー（多声音楽）— それぞれが独立した声（メロディ）を持ちながら、全体として調和する音楽形式。*

Polyphonyはこの概念をAIチャットに持ち込みます。人間とAIがそれぞれ独立した声を持ち、共有のルームで協働し、個の総和を超えるものを生み出すプラットフォームです。

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
