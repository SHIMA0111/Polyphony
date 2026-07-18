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
	mu    sync.Mutex
	Users map[string]*user.User // keyed by user ID
}

func (r *UserRepo) ensureInit() {
	if r.Users == nil {
		r.Users = make(map[string]*user.User)
	}
}

// cloneUser returns a deep-enough copy of u: a struct copy plus a fresh
// *string for KratosIdentityID when non-nil. A plain struct copy (`cp :=
// *u`) still leaves cp.KratosIdentityID pointing at the very same string as
// u.KratosIdentityID, since copying a struct copies its pointer fields by
// value, not what they point to -- so a caller mutating *cp.KratosIdentityID
// (or SetKratosIdentityID later re-deriving a pointer from the same
// address) would silently alias the stored user. cloneUser is used for
// every value stored into or read out of r.Users so no caller can ever
// observe or corrupt the repo's internal state through a shared
// KratosIdentityID pointer. Mirrors mocks.AttachmentRepo's cloneAttachment.
func cloneUser(u *user.User) *user.User {
	cp := *u
	if u.KratosIdentityID != nil {
		kratosID := *u.KratosIdentityID
		cp.KratosIdentityID = &kratosID
	}
	return &cp
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
	// Store a clone so later in-place mutation of the caller's own struct
	// (or of u.KratosIdentityID's pointee) can't alias the repo's state.
	r.Users[u.ID] = cloneUser(u)
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
	return cloneUser(u), nil
}

// GetByEmail retrieves a user by email. Returns domain.ErrNotFound if not
// present.
func (r *UserRepo) GetByEmail(_ context.Context, email string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.Users {
		if u.Email == email {
			return cloneUser(u), nil
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
			return cloneUser(u), nil
		}
	}
	return nil, domain.ErrNotFound
}

// GetByKratosIdentityID retrieves the user linked to the given Kratos
// identity ID. Returns domain.ErrNotFound if no user is linked to it,
// mirroring postgres.UserRepository.GetByKratosIdentityID.
func (r *UserRepo) GetByKratosIdentityID(_ context.Context, kratosIdentityID string) (*user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, u := range r.Users {
		if u.KratosIdentityID != nil && *u.KratosIdentityID == kratosIdentityID {
			return cloneUser(u), nil
		}
	}
	return nil, domain.ErrNotFound
}

// SetKratosIdentityID links the user identified by userID to the given
// Kratos identity ID. Returns domain.ErrNotFound if userID does not exist,
// or domain.ErrKratosIdentityAlreadyLinked if kratosIdentityID is already
// linked to a different user, mirroring postgres.UserRepository.SetKratosIdentityID.
func (r *UserRepo) SetKratosIdentityID(_ context.Context, userID, kratosIdentityID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.Users[userID]
	if !ok {
		return domain.ErrNotFound
	}
	for id, existing := range r.Users {
		if id == userID {
			continue
		}
		if existing.KratosIdentityID != nil && *existing.KratosIdentityID == kratosIdentityID {
			return domain.ErrKratosIdentityAlreadyLinked
		}
	}
	linked := kratosIdentityID
	u.KratosIdentityID = &linked
	return nil
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
	// Store a clone, same reasoning as Create: u is caller-owned and must
	// not be aliased by the repo's internal state.
	r.Users[u.ID] = cloneUser(u)
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
