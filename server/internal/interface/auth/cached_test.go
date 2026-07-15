package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

const cachedTestTTL = 30 * time.Second

// newCountingAuthService returns a mocks.AuthService whose ValidateToken
// always succeeds with the given claims but increments calls on every
// invocation, letting a test assert exactly how many times inner was hit.
func newCountingAuthService(calls *int32, claims *domainauth.Claims) *mocks.AuthService {
	return &mocks.AuthService{
		ValidateTokenFunc: func(_ context.Context, _ string) (*domainauth.Claims, error) {
			atomic.AddInt32(calls, 1)
			return claims, nil
		},
	}
}

func TestCachedAuthServiceValidateTokenCachesAcrossCalls(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	var calls int32
	inner := newCountingAuthService(&calls, &domainauth.Claims{UserID: "user-1"})
	cached := NewCachedAuthService(inner, client, cachedTestTTL)

	ctx := context.Background()

	claims, err := cached.ValidateToken(ctx, "tok-1")
	if err != nil {
		t.Fatalf("1st call: unexpected error: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("1st call: expected UserID user-1, got %s", claims.UserID)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("1st call: expected inner to be called once, got %d", got)
	}

	// 2nd call within TTL: should hit the cache, not inner.
	claims, err = cached.ValidateToken(ctx, "tok-1")
	if err != nil {
		t.Fatalf("2nd call: unexpected error: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("2nd call: expected UserID user-1, got %s", claims.UserID)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("2nd call (cache hit): expected inner call count to stay at 1, got %d", got)
	}

	// Advance miniredis's virtual clock past the TTL: the cache entry
	// expires, so the 3rd call must fall through to inner again.
	mr.FastForward(cachedTestTTL + time.Second)

	claims, err = cached.ValidateToken(ctx, "tok-1")
	if err != nil {
		t.Fatalf("3rd call: unexpected error: %v", err)
	}
	if claims.UserID != "user-1" {
		t.Fatalf("3rd call: expected UserID user-1, got %s", claims.UserID)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("3rd call (cache expired): expected inner call count to be 2, got %d", got)
	}
}

// TestCachedAuthServiceValidateTokenNeverCachesErrors proves that a failing
// ValidateToken result is never cached: two calls in a row within the TTL
// window both increment inner's call counter, since a cached error result
// would otherwise pin a transient failure for the whole TTL window.
func TestCachedAuthServiceValidateTokenNeverCachesErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	var calls int32
	wantErr := errors.New("boom")
	inner := &mocks.AuthService{
		ValidateTokenFunc: func(_ context.Context, _ string) (*domainauth.Claims, error) {
			atomic.AddInt32(&calls, 1)
			return nil, wantErr
		},
	}
	cached := NewCachedAuthService(inner, client, cachedTestTTL)

	ctx := context.Background()

	if _, err := cached.ValidateToken(ctx, "tok-err"); !errors.Is(err, wantErr) {
		t.Fatalf("1st call: expected wrapped error %v, got %v", wantErr, err)
	}
	if _, err := cached.ValidateToken(ctx, "tok-err"); !errors.Is(err, wantErr) {
		t.Fatalf("2nd call: expected wrapped error %v, got %v", wantErr, err)
	}

	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected inner to be called twice (error never cached), got %d", got)
	}
}

// TestCachedAuthServiceRevokeDeletesCacheAndDelegates proves Revoke deletes
// the cached entry for the token (a subsequent ValidateToken call misses the
// cache and hits inner again) and calls inner's Revoke exactly once when
// inner implements domainauth.Revoker.
func TestCachedAuthServiceRevokeDeletesCacheAndDelegates(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	var validateCalls int32
	var revokeCalls int32
	var revokedToken string
	inner := &mocks.AuthService{
		ValidateTokenFunc: func(_ context.Context, _ string) (*domainauth.Claims, error) {
			atomic.AddInt32(&validateCalls, 1)
			return &domainauth.Claims{UserID: "user-1"}, nil
		},
		RevokeFunc: func(_ context.Context, token string) error {
			atomic.AddInt32(&revokeCalls, 1)
			revokedToken = token
			return nil
		},
	}
	cached := NewCachedAuthService(inner, client, cachedTestTTL)

	ctx := context.Background()

	if _, err := cached.ValidateToken(ctx, "tok-revoke"); err != nil {
		t.Fatalf("priming ValidateToken call failed: %v", err)
	}
	if got := atomic.LoadInt32(&validateCalls); got != 1 {
		t.Fatalf("expected 1 ValidateToken call before revoke, got %d", got)
	}

	if err := cached.Revoke(ctx, "tok-revoke"); err != nil {
		t.Fatalf("Revoke returned unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&revokeCalls); got != 1 {
		t.Fatalf("expected inner.Revoke to be called exactly once, got %d", got)
	}
	if revokedToken != "tok-revoke" {
		t.Fatalf("expected inner.Revoke to be called with %q, got %q", "tok-revoke", revokedToken)
	}

	// A cache entry deleted by Revoke must miss on the next ValidateToken
	// call, falling through to inner again.
	if _, err := cached.ValidateToken(ctx, "tok-revoke"); err != nil {
		t.Fatalf("post-revoke ValidateToken call failed: %v", err)
	}
	if got := atomic.LoadInt32(&validateCalls); got != 2 {
		t.Fatalf("expected ValidateToken to hit inner again after Revoke purged the cache, got %d calls", got)
	}
}
