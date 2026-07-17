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
// NOT delete the identity: migrateUser's cleanup reconciliation re-reads
// userRepo.GetByKratosIdentityID(result.ID) before deleting, finds the
// "existing" user genuinely linked to it, and skips the DELETE call rather
// than yanking an identity a real user depends on out from under them.
func TestMigrateUserAlreadyLinkedIdentity(t *testing.T) {
	createdID := uuid.New().String()
	var deleteCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
		case http.MethodDelete:
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
		t.Fatalf("expected zero DELETE cleanup calls (identity is genuinely linked), got %d", deleteCalls)
	}
}

// TestMigrateUserConfirmedUnlinkedCleanupDeletes proves that migrateUser
// deletes the just-created identity when its cleanup reconciliation lookup
// (userRepo.GetByKratosIdentityID) confirms the identity is genuinely
// unlinked (domain.ErrNotFound). u is deliberately never Create()'d into
// userRepo, so SetKratosIdentityID fails with domain.ErrNotFound (simulating
// the local user having vanished between fetch and migrate), and no user is
// linked to the created identity either — the reconciliation lookup
// confirms that and the DELETE proceeds.
func TestMigrateUserConfirmedUnlinkedCleanupDeletes(t *testing.T) {
	createdID := uuid.New().String()
	var deletedPath string
	var deleteCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
		case http.MethodDelete:
			deleteCalls++
			deletedPath = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	u := newTestUser("g@example.com", "guser") // deliberately not Create()'d

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when SetKratosIdentityID's target user does not exist")
	}

	if deleteCalls != 1 {
		t.Fatalf("expected exactly one DELETE cleanup call, got %d", deleteCalls)
	}
	if deletedPath != "/admin/identities/"+createdID {
		t.Fatalf("expected cleanup DELETE for %q, got %q", createdID, deletedPath)
	}
}

// ambiguousLookupUserRepo wraps a real *mocks.UserRepo but overrides
// GetByKratosIdentityID to always fail with a fixed, non-ErrNotFound error
// (e.g. simulating a database outage during migrateUser's cleanup
// reconciliation lookup), while every other method keeps the embedded
// mocks.UserRepo's normal in-memory behavior.
type ambiguousLookupUserRepo struct {
	*mocks.UserRepo
	err error
}

func (r *ambiguousLookupUserRepo) GetByKratosIdentityID(context.Context, string) (*domainuser.User, error) {
	return nil, r.err
}

// TestMigrateUserAmbiguousCleanupLookupSkipsDelete proves that migrateUser
// does NOT delete the just-created identity when its cleanup reconciliation
// lookup fails ambiguously (a non-ErrNotFound error, so it is unknown
// whether the identity is actually linked to someone) — it must err on the
// side of not deleting rather than risk yanking an identity a real user
// might depend on.
func TestMigrateUserAmbiguousCleanupLookupSkipsDelete(t *testing.T) {
	createdID := uuid.New().String()
	var deleteCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: createdID})
		case http.MethodDelete:
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	inner := &mocks.UserRepo{}
	// Seed an existing link so SetKratosIdentityID fails (the ambiguous
	// lookup path is only reached after a genuine link failure).
	existing := newTestUser("h@example.com", "huser")
	existing.KratosIdentityID = &createdID
	if err := inner.Create(context.Background(), existing); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	u := newTestUser("i@example.com", "iuser")
	if err := inner.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	userRepo := &ambiguousLookupUserRepo{UserRepo: inner, err: errors.New("database is unreachable")}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when the identity is already linked to another user")
	}

	if deleteCalls != 0 {
		t.Fatalf("expected zero DELETE cleanup calls (lookup was ambiguous), got %d", deleteCalls)
	}
}

// TestMigrateUserCreateIdentityMissingID proves that migrateUser rejects a
// 2xx Admin API response whose body decodes successfully but carries an
// empty/missing identity id, rather than proceeding to link the local user
// to the zero-value Kratos identity ID.
func TestMigrateUserCreateIdentityMissingID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/identities" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(kratosCreateIdentityRespDTO{ID: ""})
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	u := newTestUser("j@example.com", "juser")
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	if err := migrateUser(context.Background(), server.URL, server.Client(), userRepo, u); err == nil {
		t.Fatal("expected migrateUser to fail when the response carries an empty identity id")
	}

	got, err := userRepo.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.KratosIdentityID != nil {
		t.Fatalf("expected user to remain unlinked, got %+v", *got.KratosIdentityID)
	}
}
