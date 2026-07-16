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
