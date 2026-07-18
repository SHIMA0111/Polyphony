package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis_rate/v10"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

// fixedKeyFunc returns a RateLimitConfig.KeyFunc that always returns the same
// key, so every request in a test hits the same token bucket regardless of
// the (unset, in a unit test) client IP.
func fixedKeyFunc(c echo.Context) string {
	return "fixed"
}

// newTestHandler returns a trivial echo.HandlerFunc that records it was
// called and responds 200, for asserting whether RateLimit let a request
// through to next.
func newTestHandler(called *bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		*called = true
		return c.String(http.StatusOK, "ok")
	}
}

// doRequest runs mw wrapping a fresh recording handler and returns the
// recorder and whether next was actually invoked.
func doRequest(mw echo.MiddlewareFunc) (*httptest.ResponseRecorder, bool) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	var called bool
	handler := mw(newTestHandler(&called))
	_ = handler(c)
	return rec, called
}

// TestRateLimitAllowsThenDenies proves that the first Limit.Rate requests
// within the window reach next (HTTP 200), and the request beyond it is
// rejected with HTTP 429, a numeric Retry-After header, and next not called.
func TestRateLimitAllowsThenDenies(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	limiter := redis_rate.NewLimiter(client)

	mw := RateLimit(RateLimitConfig{
		Limiter:   limiter,
		Limit:     redis_rate.PerMinute(2),
		KeyPrefix: "test",
		KeyFunc:   fixedKeyFunc,
	})

	for i := 1; i <= 2; i++ {
		rec, called := doRequest(mw)
		if !called {
			t.Fatalf("request %d: expected next to be called, it was not", i)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	rec, called := doRequest(mw)
	if called {
		t.Fatal("3rd request: expected next NOT to be called once the limit is exceeded")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: expected 429, got %d", rec.Code)
	}
	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Fatal("3rd request: expected a Retry-After header, got none")
	}
	n, err := strconv.Atoi(retryAfter)
	if err != nil || n <= 0 {
		t.Fatalf("3rd request: expected Retry-After to be a positive integer (RateLimit ceils to whole seconds and clamps to a minimum of 1), got %q (err=%v)", retryAfter, err)
	}
}

// TestRateLimitFailsOpenOnUnreachableRedis proves that a request still
// reaches next (without panicking) when the limiter's Redis is unreachable
// (a closed miniredis instance), consistent with RateLimit's documented
// fail-open behavior.
func TestRateLimitFailsOpenOnUnreachableRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	mr.Close() // Redis is now unreachable through client.

	limiter := redis_rate.NewLimiter(client)
	mw := RateLimit(RateLimitConfig{
		Limiter:   limiter,
		Limit:     redis_rate.PerMinute(2),
		KeyPrefix: "test",
		KeyFunc:   fixedKeyFunc,
	})

	rec, called := doRequest(mw)
	if !called {
		t.Fatal("expected next to be called (fail open) when Redis is unreachable")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail open), got %d", rec.Code)
	}
}

// TestRateLimitNilLimiterFailsOpen proves that a nil cfg.Limiter (the state
// container.go leaves Container.RateLimiter in when no Redis client is
// configured at all) also fails open without panicking.
func TestRateLimitNilLimiterFailsOpen(t *testing.T) {
	mw := RateLimit(RateLimitConfig{
		Limiter:   nil,
		Limit:     redis_rate.PerMinute(2),
		KeyPrefix: "test",
		KeyFunc:   fixedKeyFunc,
	})

	rec, called := doRequest(mw)
	if !called {
		t.Fatal("expected next to be called (fail open) when Limiter is nil")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (fail open), got %d", rec.Code)
	}
}
