use std::time::Duration;

use crate::config::HttpClientConfig;

/// Retry policy for outbound HTTP requests to LLM providers.
///
/// Built from `HttpClientConfig` and shared across provider adapters (OpenAI today;
/// Anthropic/Gemini in later steps) so retry/backoff behavior stays consistent.
#[derive(Debug, Clone, Copy)]
pub struct RetryPolicy {
    /// Maximum number of retry attempts after the initial request.
    max_retries: u32,
    /// Base delay used for exponential backoff between retries.
    base_delay: Duration,
}

impl RetryPolicy {
    /// Builds a `RetryPolicy` from the shared HTTP client configuration.
    ///
    /// # Arguments
    /// * `config` — Shared HTTP client tuning (max retries, base backoff delay).
    ///
    /// # Returns
    /// A `RetryPolicy` configured with `config`'s `max_retries` and `retry_base_delay`.
    pub fn new(config: &HttpClientConfig) -> Self {
        Self {
            max_retries: config.max_retries,
            base_delay: config.retry_base_delay,
        }
    }

    /// Maximum number of retry attempts after the initial request.
    pub fn max_retries(&self) -> u32 {
        self.max_retries
    }

    /// Reports whether a response status warrants a retry.
    ///
    /// # Arguments
    /// * `status` — HTTP status code returned by the provider.
    ///
    /// # Returns
    /// `true` for `429 Too Many Requests` and any `5xx` server error; `false` otherwise
    /// (client errors other than `429` are not retried, since retrying them would not
    /// change the outcome).
    pub fn is_retryable(status: reqwest::StatusCode) -> bool {
        status == reqwest::StatusCode::TOO_MANY_REQUESTS || status.is_server_error()
    }

    /// Computes the exponential backoff delay before a given retry attempt.
    ///
    /// # Arguments
    /// * `attempt` — Zero-based retry attempt number (`0` for the first retry after
    ///   the initial request).
    ///
    /// # Returns
    /// `base_delay * 2^attempt`. Saturates instead of overflowing for very large
    /// `attempt` values.
    pub fn backoff_delay(&self, attempt: u32) -> Duration {
        self.base_delay
            .saturating_mul(1u32.checked_shl(attempt).unwrap_or(u32::MAX))
    }
}

/// Sends an HTTP request with bounded exponential-backoff retry on retryable
/// (`429`/`5xx`) responses.
///
/// # Arguments
/// * `policy` — Retry policy (max attempts, backoff base delay) to apply.
/// * `send` — Closure that performs one attempt of the request, returning a fresh
///   `reqwest::Response` (or transport error) each time it is called. Callers must
///   ensure the closure can be invoked multiple times (e.g. by cloning a
///   `reqwest::Request` builder per attempt), since a `reqwest::Request` body cannot
///   generally be replayed after being consumed.
///
/// # Returns
/// The first non-retryable response (including the first successful one), or the
/// last response/error observed once `policy.max_retries()` retries have been
/// exhausted.
///
/// # Errors
/// Propagates the `reqwest::Error` from the final attempt if the transport itself
/// fails (e.g. connection reset) on the last attempt.
pub async fn send_with_retry<F, Fut>(
    policy: &RetryPolicy,
    mut send: F,
) -> Result<reqwest::Response, reqwest::Error>
where
    F: FnMut() -> Fut,
    Fut: std::future::Future<Output = Result<reqwest::Response, reqwest::Error>>,
{
    let mut attempt = 0u32;
    loop {
        let response = send().await?;

        if attempt >= policy.max_retries() || !RetryPolicy::is_retryable(response.status()) {
            return Ok(response);
        }

        let retry_after = response
            .headers()
            .get(reqwest::header::RETRY_AFTER)
            .and_then(|v| v.to_str().ok())
            .and_then(|v| v.parse::<u64>().ok())
            .map(Duration::from_secs);

        let delay = retry_after.unwrap_or_else(|| policy.backoff_delay(attempt));

        tracing::warn!(
            attempt = attempt + 1,
            status = %response.status(),
            delay_ms = delay.as_millis(),
            "retrying request after retryable response"
        );

        tokio::time::sleep(delay).await;
        attempt += 1;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn policy(max_retries: u32, base_delay_ms: u64) -> RetryPolicy {
        RetryPolicy {
            max_retries,
            base_delay: Duration::from_millis(base_delay_ms),
        }
    }

    #[test]
    fn test_is_retryable_true_for_429() {
        assert!(RetryPolicy::is_retryable(
            reqwest::StatusCode::TOO_MANY_REQUESTS
        ));
    }

    #[test]
    fn test_is_retryable_true_for_5xx() {
        assert!(RetryPolicy::is_retryable(
            reqwest::StatusCode::INTERNAL_SERVER_ERROR
        ));
        assert!(RetryPolicy::is_retryable(reqwest::StatusCode::BAD_GATEWAY));
        assert!(RetryPolicy::is_retryable(
            reqwest::StatusCode::SERVICE_UNAVAILABLE
        ));
    }

    #[test]
    fn test_is_retryable_false_for_2xx_and_4xx() {
        assert!(!RetryPolicy::is_retryable(reqwest::StatusCode::OK));
        assert!(!RetryPolicy::is_retryable(reqwest::StatusCode::BAD_REQUEST));
        assert!(!RetryPolicy::is_retryable(reqwest::StatusCode::NOT_FOUND));
    }

    #[test]
    fn test_backoff_delay_exponential() {
        let p = policy(5, 100);
        assert_eq!(p.backoff_delay(0), Duration::from_millis(100));
        assert_eq!(p.backoff_delay(1), Duration::from_millis(200));
        assert_eq!(p.backoff_delay(2), Duration::from_millis(400));
        assert_eq!(p.backoff_delay(3), Duration::from_millis(800));
    }

    #[test]
    fn test_backoff_delay_does_not_overflow_for_large_attempt() {
        let p = policy(100, 100);
        // Should saturate rather than panic on overflow.
        let delay = p.backoff_delay(u32::MAX - 1);
        assert!(delay >= Duration::from_millis(100));
    }

    #[test]
    fn test_max_retries_accessor() {
        let p = policy(7, 50);
        assert_eq!(p.max_retries(), 7);
    }
}
