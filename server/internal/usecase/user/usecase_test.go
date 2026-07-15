package user

import (
	"context"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

type mockUserRepo struct {
	users map[string]*domainuser.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*domainuser.User)}
}

func (m *mockUserRepo) Create(_ context.Context, u *domainuser.User) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*domainuser.User, error) {
	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByEmail(_ context.Context, email string) (*domainuser.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*domainuser.User, error) {
	for _, u := range m.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *mockUserRepo) Update(_ context.Context, u *domainuser.User) error {
	if _, ok := m.users[u.ID]; !ok {
		return domain.ErrNotFound
	}
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.users, id)
	return nil
}

func TestGetDelegatesToRepository(t *testing.T) {
	repo := newMockUserRepo()
	now := time.Now()
	repo.users["user-1"] = &domainuser.User{
		ID:           "user-1",
		Email:        "test@example.com",
		Username:     "tester",
		PasswordHash: "hashed",
		CreatedAt:    now,
		UpdatedAt:    now,
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
	repo := newMockUserRepo()
	uc := NewUserUsecase(repo)

	_, err := uc.Get(context.Background(), "missing")
	if err != domain.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
