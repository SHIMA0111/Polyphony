//! LLM Gateway library crate root.
//!
//! Exposes the gateway's hexagonal-architecture modules (`domain`, `ports`, `adapters`,
//! `config`) as a library so both the `main` binary and the `tests/` integration test
//! binaries (which run as separate crates and can only see `pub` items of this crate)
//! can construct a real `axum::Router` and real outbound provider adapters without
//! duplicating any wiring logic.

pub mod adapters;
pub mod config;
pub mod domain;
pub mod ports;
