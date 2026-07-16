package user

import (
	"context"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func TestGetDelegatesToRepository(t *testing.T) {
	repo := &mocks.UserRepo{}
	now := time.Now()
	repo.Users = map[string]*domainuser.User{
		"user-1": {
			ID:           "user-1",
			Email:        "test@example.com",
			Username:     "tester",
			PasswordHash: "hashed",
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	uc := NewUserUsecase(repo)

	got, err := uc.Get(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != "user-1" || got.Email != "test@example.com" {
		t.Fatalf("unexpected user returned: %+v", got)
	}
}

func TestGetNotFound(t *testing.T) {
	repo := &mocks.UserRepo{}
	uc := NewUserUsecase(repo)

	_, err := uc.Get(context.Background(), "missing")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
