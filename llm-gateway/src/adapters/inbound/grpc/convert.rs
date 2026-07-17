//! Proto ↔ domain conversion helpers for the gRPC inbound adapter.
//!
//! Mirrors the REST DTO conversion pattern in `adapters::inbound::rest::request`
//! (`parse_role`) and `adapters::inbound::rest::response` (`role_to_api_string`), and
//! keeps the gRPC error-transport mapping in lockstep with
//! `adapters::inbound::rest::handlers::AppError::into_response` — both are the
//! canonical mapping for the same `DomainError` enum, so REST and gRPC never silently
//! diverge on a variant.

use tonic::Status;

use crate::domain::error::DomainError;
use crate::domain::model::Role;

use super::pb::ChatRole;

/// Converts a wire-format proto `ChatRole` into the domain `Role`.
///
/// # Arguments
/// * `role` — The `i32` value of a `ChatRole` field as received over the wire.
///
/// # Returns
/// The corresponding `Role`.
///
/// # Errors
/// Returns `DomainError::InvalidRequest` if `role` is not a recognized `ChatRole`
/// value, or is `CHAT_ROLE_UNSPECIFIED`.
pub fn proto_role_to_domain(role: i32) -> Result<Role, DomainError> {
    match ChatRole::try_from(role).unwrap_or(ChatRole::Unspecified) {
        ChatRole::System => Ok(Role::System),
        ChatRole::User => Ok(Role::User),
        ChatRole::Assistant => Ok(Role::Assistant),
        ChatRole::Tool => Ok(Role::Tool),
        ChatRole::Unspecified => Err(DomainError::InvalidRequest(format!(
            "unknown or unspecified chat role: {role}"
        ))),
    }
}

/// Converts a domain `Role` into the wire-format proto `ChatRole`.
///
/// # Arguments
/// * `role` — The domain role to convert.
///
/// # Returns
/// The corresponding `ChatRole` variant. Never returns `Unspecified` — every domain
/// `Role` variant maps to a concrete `ChatRole`.
pub fn domain_role_to_proto(role: &Role) -> ChatRole {
    match role {
        Role::System => ChatRole::System,
        Role::User => ChatRole::User,
        Role::Assistant => ChatRole::Assistant,
        Role::Tool => ChatRole::Tool,
    }
}

/// Maps a `DomainError` to the `tonic::Status` gRPC clients should observe.
///
/// This is the gRPC counterpart of `AppError::into_response` in the REST adapter;
/// both matches must cover every `DomainError` variant identically in spirit (same
/// error class → same transport-level status).
///
/// # Arguments
/// * `err` — The domain error produced by `CompletionUseCase`.
///
/// # Returns
/// A `tonic::Status` with a code appropriate to the error variant and the error's
/// `Display` message as the status message.
pub fn domain_error_to_status(err: DomainError) -> Status {
    let message = err.to_string();
    match err {
        DomainError::InvalidRequest(_) => Status::invalid_argument(message),
        DomainError::ModelNotFound(_) => Status::not_found(message),
        DomainError::KeyNotFound(_) => Status::internal(message),
        DomainError::Timeout => Status::deadline_exceeded(message),
        DomainError::ProviderError { .. } => Status::unavailable(message),
        DomainError::RateLimited { .. } => Status::resource_exhausted(message),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_proto_role_to_domain_round_trips_known_roles() {
        assert_eq!(
            proto_role_to_domain(ChatRole::System as i32).unwrap(),
            Role::System
        );
        assert_eq!(
            proto_role_to_domain(ChatRole::User as i32).unwrap(),
            Role::User
        );
        assert_eq!(
            proto_role_to_domain(ChatRole::Assistant as i32).unwrap(),
            Role::Assistant
        );
        assert_eq!(
            proto_role_to_domain(ChatRole::Tool as i32).unwrap(),
            Role::Tool
        );
    }

    #[test]
    fn test_proto_role_to_domain_rejects_unspecified() {
        assert!(matches!(
            proto_role_to_domain(ChatRole::Unspecified as i32),
            Err(DomainError::InvalidRequest(_))
        ));
    }

    #[test]
    fn test_proto_role_to_domain_rejects_unknown_i32() {
        assert!(matches!(
            proto_role_to_domain(999),
            Err(DomainError::InvalidRequest(_))
        ));
    }

    #[test]
    fn test_domain_role_to_proto_round_trips() {
        for role in [Role::System, Role::User, Role::Assistant, Role::Tool] {
            let proto = domain_role_to_proto(&role);
            assert_eq!(proto_role_to_domain(proto as i32).unwrap(), role);
        }
    }

    #[test]
    fn test_domain_error_to_status_maps_every_variant() {
        assert_eq!(
            domain_error_to_status(DomainError::InvalidRequest("x".into())).code(),
            tonic::Code::InvalidArgument
        );
        assert_eq!(
            domain_error_to_status(DomainError::ModelNotFound("x".into())).code(),
            tonic::Code::NotFound
        );
        assert_eq!(
            domain_error_to_status(DomainError::KeyNotFound("x".into())).code(),
            tonic::Code::Internal
        );
        assert_eq!(
            domain_error_to_status(DomainError::Timeout).code(),
            tonic::Code::DeadlineExceeded
        );
        assert_eq!(
            domain_error_to_status(DomainError::provider_error("x")).code(),
            tonic::Code::Unavailable
        );
        assert_eq!(
            domain_error_to_status(DomainError::RateLimited {
                retry_after_secs: Some(5)
            })
            .code(),
            tonic::Code::ResourceExhausted
        );
    }
}
