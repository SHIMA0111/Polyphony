package mocks

import (
	"context"
	"sync"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// UserRepo is an in-memory, map-backed fake implementing
// user.UserRepository. It mirrors the domain.ErrEmailAlreadyExists,
// domain.ErrUsernameAlreadyExists, and domain.ErrNotFound semantics of
// postgres.UserRepository so usecases can be tested without a real
// database. The zero value (mocks.UserRepo{}) is ready to use; the backing
// map is initialized lazily on first write.
//
// UserRepo is safe for concurrent use.
type UserRepo struct {
	mu sync.Mutex
	// Users is the backing store, keyed by user ID. It is exported
	// deliberately for test fixture ergonomics (seeding/inspecting state
	// directly); access only while holding mu.
	Users map[string]*user.User
}

func (r *UserRepo) ensureInit() {
	if r.Users == nil {
		r.Users = make(map[string]*user.User)
	}
}

// Create persists a new user. Returns domain.ErrEmailAlreadyExists or
// domain.ErrUsernameAlreadyExists if a conflict is detected.
func (r *UserRepo) Create(_ context.Context, u *user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureInit()

	for _, existing := range r.Users {
		if existing.Email == u.Email {
			return domain.ErrEmailAlreadyExists
		}
		if existing.Username == u.Username {
			return domain.ErrUsernameAlreadyExists
		}
	}
	stored := *u // clone: never alias the caller-owned struct
	r.Users[u.ID] = &stored
	return nil
}

// GetByID retrieves a user by ID. Returns domain.ErrNotFound if not present.
func (r *UserRepo) GetByID(_ context.Context, id string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.Users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

// GetByEmail retrieves a user by email. Returns domain.ErrNotFound if not
// present.
func (r *UserRepo) GetByEmail(_ context.Context, email string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.Users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

// GetByUsername retrieves a user by username. Returns domain.ErrNotFound if
// not present.
func (r *UserRepo) GetByUsername(_ context.Context, username string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.Users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

// Update updates user fields. Returns domain.ErrNotFound if the user does
// not exist, or domain.ErrEmailAlreadyExists / domain.ErrUsernameAlreadyExists
// if the update would conflict with a different existing user.
func (r *UserRepo) Update(_ context.Context, u *user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Users[u.ID]; !ok {
		return domain.ErrNotFound
	}
	for id, existing := range r.Users {
		if id == u.ID {
			continue
		}
		if existing.Email == u.Email {
			return domain.ErrEmailAlreadyExists
		}
		if existing.Username == u.Username {
			return domain.ErrUsernameAlreadyExists
		}
	}
	stored := *u // clone: never alias the caller-owned struct
	r.Users[u.ID] = &stored
	return nil
}

// Delete removes a user by ID. Returns domain.ErrNotFound if the user does
// not exist.
func (r *UserRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.Users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.Users, id)
	return nil
}
