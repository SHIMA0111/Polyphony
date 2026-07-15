# Minimal deterministic OpenAI-compatible completion stub, used only by the
# Docker Compose `test` profile (see docker-compose.yml's `llm-stub` service
# and docs/tasks/step10.md). Not part of the normal dev stack.
FROM oven/bun:1-alpine

WORKDIR /app
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile

COPY . .

EXPOSE 8092
ENV PORT=8092

CMD ["bun", "run", "server.ts"]
