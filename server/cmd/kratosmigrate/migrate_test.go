package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

func newTestUser(email, username string) *domainuser.User {
	now := time.Now()
	return &domainuser.User{
		ID:           uuid.New().String(),
		Email:        email,
		Username:     username,
		PasswordHash: "$argon2id$v=19$m=65536,t=1,p=4$c29tZXNhbHQ$c29tZWhhc2g",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// TestMigrateUserSuccess proves that migrateUser posts the user's traits and
// existing argon2id password hash to the Admin API's /admin/identities
// endpoint and links the returned identity ID back via SetKratosIdentityID.
func TestMigrateUserSuccess(t *testing.T) {
	createdID := uuid.New().String()
	var captured kratosCreateIdentityReqDTO

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/identities" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	u := newTestUser("a@example.com", "auser")
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err != nil {
		t.Fatalf("migrateUser failed: %v", err)
	}

	if captured.SchemaID != "default" {
		t.Errorf("expected schema_id \"default\", got %q", captured.SchemaID)
	}
	if captured.Traits.Email != u.Email || captured.Traits.Username != u.Username {
		t.Errorf("unexpected traits in request: %+v", captured.Traits)
	}
	if captured.Credentials.Password.Config.HashedPassword != u.PasswordHash {
		t.Errorf("expected hashed_password to reuse users.password_hash verbatim, got %q",
			captured.Credentials.Password.Config.HashedPassword)
	}

	got, err := userRepo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.KratosIdentityID == nil || *got.KratosIdentityID != createdID {
		t.Fatalf("expected linked kratos identity id %q, got %+v", createdID, got.KratosIdentityID)
	}
}

// TestMigrateUserAdminAPIFailure proves that migrateUser surfaces an error
// (leaving the user unlinked) when the Admin API returns a non-2xx status.
func TestMigrateUserAdminAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	u := newTestUser("b@example.com", "buser")
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when the admin API returns 500")
	}

	got, err := userRepo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.KratosIdentityID != nil {
		t.Fatalf("expected user to remain unlinked after admin API failure, got %+v", *got.KratosIdentityID)
	}
}

// TestMigrateUserAlreadyLinkedIdentity proves that migrateUser surfaces the
// error from SetKratosIdentityID when the Admin API happens to return an
// identity ID already linked to a different local user.
func TestMigrateUserAlreadyLinkedIdentity(t *testing.T) {
	createdID := uuid.New().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	existing := newTestUser("existing@example.com", "existinguser")
	existing.KratosIdentityID = &createdID
	if err := userRepo.Create(context.Background(), existing); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}

	u := newTestUser("c@example.com", "cuser")
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when the identity is already linked to another user")
	}
}
