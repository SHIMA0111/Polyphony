# === Build stage ===
FROM rust:1.88-alpine AS build

RUN apk add --no-cache musl-dev

WORKDIR /src
COPY Cargo.toml Cargo.lock* ./
# Create dummy main to cache dependency build
RUN mkdir src && echo "fn main() {}" > src/main.rs && cargo build --release && rm -rf src

COPY src ./src
# Touch main.rs so cargo rebuilds the actual binary
RUN touch src/main.rs && cargo build --release

# === Runtime stage ===
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /src/target/release/llm-gateway /llm-gateway

EXPOSE 8081
ENTRYPOINT ["/llm-gateway"]
