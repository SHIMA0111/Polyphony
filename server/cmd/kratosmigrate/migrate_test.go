package main

import (
	"context"
	"encoding/json"
	"errors"
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
// identity ID already linked to a different local user, and that it does
// NOT delete the identity: the reconciliation lookup
// (GetByKratosIdentityID) finds that existing link and skips the cleanup
// delete rather than destroying another user's legitimate link.
func TestMigrateUserAlreadyLinkedIdentity(t *testing.T) {
	createdID := uuid.New().String()
	var deleteCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/identities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
		case r.Method == http.MethodDelete:
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
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

	if deleteCalls != 0 {
		t.Fatalf("expected zero DELETE calls since the identity is confirmed linked to another user, got %d", deleteCalls)
	}
}

// TestMigrateUserResponseMissingIdentityID proves that migrateUser rejects a
// 2xx Admin API response whose body decodes successfully but carries an
// empty/missing "id" field, without attempting to link or delete anything.
func TestMigrateUserResponseMissingIdentityID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/admin/identities" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: ""})
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	u := newTestUser("d@example.com", "duser")
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when the response is missing an identity id")
	}

	got, err := userRepo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.KratosIdentityID != nil {
		t.Fatalf("expected user to remain unlinked when identity id is missing, got %+v", *got.KratosIdentityID)
	}
}

// ambiguousGetUserRepo wraps a *mocks.UserRepo and forces
// GetByKratosIdentityID to return a non-ErrNotFound error, simulating a
// repository failure (e.g. a DB outage) during the cleanup reconciliation
// lookup rather than a confirmed "nothing is linked" result.
type ambiguousGetUserRepo struct {
	*mocks.UserRepo
	getErr error
}

func (r *ambiguousGetUserRepo) GetByKratosIdentityID(_ context.Context, _ string) (*domainuser.User, error) {
	return nil, r.getErr
}

// TestMigrateUserCleanupSkippedOnAmbiguousLookup proves that migrateUser
// does NOT delete the just-created Kratos identity when the reconciliation
// lookup (GetByKratosIdentityID) fails with something other than
// domain.ErrNotFound: an ambiguous failure must not be treated as
// confirmation that the identity is safe to delete.
func TestMigrateUserCleanupSkippedOnAmbiguousLookup(t *testing.T) {
	createdID := uuid.New().String()
	var deleteCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/identities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
		case r.Method == http.MethodDelete:
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	base := &mocks.UserRepo{}
	u := newTestUser("e@example.com", "euser")
	if err := base.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Force SetKratosIdentityID to fail too, by pre-linking a different user
	// to createdID in the base repo (SetKratosIdentityID looks at base.Users
	// directly since ambiguousGetUserRepo embeds *mocks.UserRepo).
	other := newTestUser("f@example.com", "fuser")
	other.KratosIdentityID = &createdID
	if err := base.Create(context.Background(), other); err != nil {
		t.Fatalf("seed other user: %v", err)
	}

	userRepo := &ambiguousGetUserRepo{UserRepo: base, getErr: errors.New("db unavailable")}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when SetKratosIdentityID fails")
	}

	if deleteCalls != 0 {
		t.Fatalf("expected zero DELETE calls when the reconciliation lookup fails ambiguously, got %d", deleteCalls)
	}
}

// TestMigrateUserCleanupConfirmedUnlinked proves that migrateUser DOES
// delete the just-created Kratos identity when the reconciliation lookup
// (GetByKratosIdentityID) confirms via domain.ErrNotFound that nothing is
// linked to it.
func TestMigrateUserCleanupConfirmedUnlinked(t *testing.T) {
	createdID := uuid.New().String()
	var deleteCalls int
	var deletedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/identities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
		case r.Method == http.MethodDelete:
			deleteCalls++
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	// u itself doesn't exist in the repo, so SetKratosIdentityID fails with
	// domain.ErrNotFound (userID unknown), and GetByKratosIdentityID for
	// createdID also confirms ErrNotFound (nothing else is linked to it
	// either), so the cleanup delete should proceed.
	u := newTestUser("g@example.com", "guser")

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when SetKratosIdentityID fails")
	}

	if deleteCalls != 1 {
		t.Fatalf("expected exactly one DELETE call when the identity is confirmed unlinked, got %d", deleteCalls)
	}
	if deletedPath != "/admin/identities/"+createdID {
		t.Fatalf("expected DELETE /admin/identities/%s, got %q", createdID, deletedPath)
	}
}
