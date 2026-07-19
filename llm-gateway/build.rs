//! Build script: compiles the shared gRPC contract in `server/proto/` into Rust server
//! stubs via `tonic-build`.
//!
//! The proto files are consumed in-place from `../server/proto` (relative to this
//! crate) rather than copied or vendored — `server/proto/` is the canonical location
//! for the contract (see `docs/tasks/step19.md`'s "Proto location rationale"), and
//! Step 28's Go client generates its own stubs from the same files.
//!
//! Only server stubs are generated (`build_client(false)`): this gateway process only
//! ever *implements* `CompletionService`/`ModelsService`, it never calls them as a
//! client.
fn main() -> Result<(), Box<dyn std::error::Error>> {
    // As of tonic 0.14, proto-compiling codegen lives in `tonic-prost-build`
    // (`tonic-build` itself only holds the client/server service-trait codegen).
    tonic_prost_build::configure()
        .build_server(true)
        .build_client(false)
        .compile_protos(
            &[
                "../server/proto/llmgateway/v1/completion.proto",
                "../server/proto/llmgateway/v1/models.proto",
            ],
            &["../server/proto"],
        )?;
    Ok(())
}
