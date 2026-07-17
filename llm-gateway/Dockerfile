# NOTE: built with the repo root as build context (`context: .` in docker-compose.yml,
# `dockerfile: llm-gateway/Dockerfile`), not `./llm-gateway`, because `build.rs` reads
# the shared gRPC contract from `server/proto/` via a relative include path
# (`../server/proto`, relative to this crate) — that directory must be inside the
# build context for `docker compose build llm-gateway` to succeed. All COPY sources
# below are therefore repo-root-relative.

# === Build stage ===
FROM rust:1.88-alpine AS build

# `protobuf-dev`/`protoc` are needed for `build.rs`'s `tonic-prost-build` proto
# compilation step; `musl-dev` is needed for the existing C toolchain bits.
RUN apk add --no-cache musl-dev protobuf-dev protoc

WORKDIR /src
COPY llm-gateway/Cargo.toml llm-gateway/Cargo.lock* ./
# Create dummy main to cache dependency build. No build.rs/protos are copied yet, so
# this layer never invokes proto compilation — it only warms the crates.io dependency
# cache, which is the point of splitting it out.
RUN mkdir src && echo "fn main() {}" > src/main.rs && cargo build --release && rm -rf src

COPY llm-gateway/src ./src
COPY llm-gateway/build.rs ./build.rs
COPY server/proto /server/proto
# Touch main.rs so cargo rebuilds the actual binary, now with build.rs compiling the
# real proto files against ../server/proto (i.e. /server/proto in this image).
RUN touch src/main.rs && cargo build --release

# === Runtime stage ===
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /src/target/release/llm-gateway /llm-gateway

EXPOSE 8081
EXPOSE 50051
ENTRYPOINT ["/llm-gateway"]
