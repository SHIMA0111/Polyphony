use axum::http::Request;
use tower_http::request_id::{MakeRequestId, RequestId};
use uuid::Uuid;

/// `MakeRequestId` implementation that generates a UUIDv4 for each request that does
/// not already carry an `x-request-id` header.
///
/// `tower-http` intentionally ships no UUID generator itself (to avoid a mandatory
/// `uuid` dependency for consumers who don't want one), so this project supplies its
/// own, following the pattern documented by `tower-http`'s request-id example.
#[derive(Clone, Default)]
pub struct UuidRequestId;

impl MakeRequestId for UuidRequestId {
    /// Generates a new request id.
    ///
    /// # Arguments
    /// * `_request` — The incoming request (unused; every call produces a fresh id).
    ///
    /// # Returns
    /// `Some(RequestId)` wrapping a freshly generated UUIDv4, formatted as a string.
    /// Always `Some` — request id generation never fails.
    fn make_request_id<B>(&mut self, _request: &Request<B>) -> Option<RequestId> {
        let id = Uuid::new_v4().to_string();
        id.parse().ok().map(RequestId::new)
    }
}
