// Package user provides use cases for reading the authenticated caller's own
// user identity (the "whoami" endpoint).
package user

import (
	"context"

	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// UserUsecase provides identity-related use cases for the authenticated caller.
type UserUsecase struct {
	repo domainuser.UserRepository
}

// NewUserUsecase creates a new UserUsecase backed by the given UserRepository.
func NewUserUsecase(repo domainuser.UserRepository) *UserUsecase {
	return &UserUsecase{repo: repo}
}

// Get retrieves the user identified by userID. It is a thin delegation to
// UserRepository.GetByID and returns domain.ErrNotFound if no user with that
// ID exists.
func (u *UserUsecase) Get(ctx context.Context, userID string) (*domainuser.User, error) {
	return u.repo.GetByID(ctx, userID)
}
