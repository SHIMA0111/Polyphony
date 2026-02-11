package user

import "context"

// UserRepository defines persistence operations for users.
type UserRepository interface {
	// Create persists a new user. Returns ErrEmailAlreadyExists or
	// ErrUsernameAlreadyExists if a conflict is detected.
	Create(ctx context.Context, user *User) error

	// GetByID retrieves a user by ID. Returns ErrNotFound if not found.
	GetByID(ctx context.Context, id string) (*User, error)

	// GetByEmail retrieves a user by email. Returns ErrNotFound if not found.
	GetByEmail(ctx context.Context, email string) (*User, error)

	// GetByUsername retrieves a user by username. Returns ErrNotFound if not found.
	GetByUsername(ctx context.Context, username string) (*User, error)

	// Update updates user fields. Returns ErrNotFound if user does not exist.
	Update(ctx context.Context, user *User) error

	// Delete removes a user by ID. Returns ErrNotFound if user does not exist.
	Delete(ctx context.Context, id string) error
}
