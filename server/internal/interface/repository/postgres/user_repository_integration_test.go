//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// TestUserRepositoryUniqueViolationMapping proves that
// UserRepository.Create maps the real Postgres unique-constraint violations
// on the users_email_unique and users_username_unique constraints to
// domain.ErrEmailAlreadyExists and domain.ErrUsernameAlreadyExists
// respectively, verified via errors.Is against the actual database (rather
// than an in-memory mock's approximation of the same logic).
func TestUserRepositoryUniqueViolationMapping(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)

	base := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "unique@example.com",
		Username:     "uniqueuser",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, base); err != nil {
		t.Fatalf("create base user: %v", err)
	}

	dupEmail := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        base.Email,
		Username:     "differentuser",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	err := userRepo.Create(ctx, dupEmail)
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("expected errors.Is(err, ErrEmailAlreadyExists) to be true, got %v", err)
	}

	dupUsername := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "other@example.com",
		Username:     base.Username,
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	err = userRepo.Create(ctx, dupUsername)
	if !errors.Is(err, domain.ErrUsernameAlreadyExists) {
		t.Fatalf("expected errors.Is(err, ErrUsernameAlreadyExists) to be true, got %v", err)
	}
}

// TestUserRepositoryKratosIdentityIDRoundTrip proves that
// SetKratosIdentityID/GetByKratosIdentityID round-trip against the real
// database, and that inserting a second user with the same
// kratos_identity_id violates the users_kratos_identity_id_unique
// constraint, mapped to domain.ErrKratosIdentityAlreadyLinked.
func TestUserRepositoryKratosIdentityIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testutilpg.New(ctx, t)
	userRepo := NewUserRepository(pool)

	u := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "kratos-link@example.com",
		Username:     "kratoslinkuser",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Unlinked users are not resolvable by kratos identity ID.
	if _, err := userRepo.GetByKratosIdentityID(ctx, uuid.New().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound for an unlinked identity, got %v", err)
	}

	kratosIdentityID := uuid.New().String()
	if err := userRepo.SetKratosIdentityID(ctx, u.ID, kratosIdentityID); err != nil {
		t.Fatalf("SetKratosIdentityID: %v", err)
	}

	linked, err := userRepo.GetByKratosIdentityID(ctx, kratosIdentityID)
	if err != nil {
		t.Fatalf("GetByKratosIdentityID: %v", err)
	}
	if linked.ID != u.ID {
		t.Fatalf("expected linked user ID %q, got %q", u.ID, linked.ID)
	}

	other := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "kratos-link-2@example.com",
		Username:     "kratoslinkuser2",
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, other); err != nil {
		t.Fatalf("create second user: %v", err)
	}

	err = userRepo.SetKratosIdentityID(ctx, other.ID, kratosIdentityID)
	if !errors.Is(err, domain.ErrKratosIdentityAlreadyLinked) {
		t.Fatalf("expected errors.Is(err, ErrKratosIdentityAlreadyLinked) to be true, got %v", err)
	}
}
