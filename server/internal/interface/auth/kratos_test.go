package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// newSeedUser builds a local user row for pre-seeding a mocks.UserRepo in
// tests, with no Kratos link by default.
func newSeedUser(email, username string) *domainuser.User {
	now := time.Now()
	return &domainuser.User{
		ID:           uuid.New().String(),
		Email:        email,
		Username:     username,
		PasswordHash: kratosManagedPasswordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// kratosNodeMessages is the repeated anonymous "node with attributes.name
// and messages" shape used by kratosFlowErrorDTO.UI.Nodes, factored out so
// tests can build one without repeating the inline struct type.
type kratosNodeMessages = struct {
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
	Messages []kratosUIMessageDTO `json:"messages"`
}

func newDuplicateIdentifierFlowError(traitName string) kratosFlowErrorDTO {
	var flowErr kratosFlowErrorDTO
	node := kratosNodeMessages{}
	node.Attributes.Name = traitName
	node.Messages = []kratosUIMessageDTO{
		{ID: 4000007, Text: "An account with the same identifier exists already. Please sign in instead.", Type: "error"},
	}
	flowErr.UI.Nodes = append(flowErr.UI.Nodes, node)
	return flowErr
}

// fakeKratos stands up an httptest.Server implementing just enough of
// Kratos's public API (self-service registration/login flows and
// /sessions/whoami) for KratosAuthService's unit tests, driven entirely by
// the fields below so each test can script the exact success/failure
// response it needs.
type fakeKratos struct {
	server *httptest.Server

	registrationFlowStatus   int
	registrationSubmitStatus int
	registrationSubmitBody   interface{}

	loginFlowStatus   int
	loginSubmitStatus int
	loginSubmitBody   interface{}

	whoamiStatus int
	whoamiBody   interface{}
}

func newFakeKratos() *fakeKratos {
	f := &fakeKratos{
		registrationFlowStatus:   http.StatusOK,
		loginFlowStatus:          http.StatusOK,
		registrationSubmitStatus: http.StatusOK,
		loginSubmitStatus:        http.StatusOK,
		whoamiStatus:             http.StatusOK,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/self-service/registration/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.registrationFlowStatus)
		_ = json.NewEncoder(w).Encode(kratosFlowDTO{ID: "reg-flow-id"})
	})
	mux.HandleFunc("/self-service/registration", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("flow") != "reg-flow-id" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.registrationSubmitStatus)
		_ = json.NewEncoder(w).Encode(f.registrationSubmitBody)
	})
	mux.HandleFunc("/self-service/login/api", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.loginFlowStatus)
		_ = json.NewEncoder(w).Encode(kratosFlowDTO{ID: "login-flow-id"})
	})
	mux.HandleFunc("/self-service/login", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("flow") != "login-flow-id" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.loginSubmitStatus)
		_ = json.NewEncoder(w).Encode(f.loginSubmitBody)
	})
	mux.HandleFunc("/sessions/whoami", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.whoamiStatus)
		_ = json.NewEncoder(w).Encode(f.whoamiBody)
	})
	f.server = httptest.NewServer(mux)
	return f
}

func (f *fakeKratos) close() { f.server.Close() }

func newTestKratosService(f *fakeKratos, userRepo *mocks.UserRepo) *KratosAuthService {
	return NewKratosAuthService(userRepo, f.server.URL, f.server.URL, "ory_kratos_session", f.server.Client())
}

// TestKratosRegisterSuccess proves that Register submits the registration
// flow, creates the local user linked to the returned identity ID, and
// returns the native session token as the TokenPair.
func TestKratosRegisterSuccess(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.registrationSubmitBody = kratosRegistrationRespDTO{
		SessionToken: "native-session-token",
		Identity:     kratosIdentityDTO{ID: identityID, Traits: kratosTraitsDTO{Email: "a@example.com", Username: "auser"}},
	}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	pair, err := svc.Register(context.Background(), "a@example.com", "auser", "Str0ngP@ss1")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if pair.AccessToken != "native-session-token" || pair.TokenType != "Bearer" {
		t.Errorf("unexpected token pair: %+v", pair)
	}

	linkedUser, err := userRepo.GetByKratosIdentityID(context.Background(), identityID)
	if err != nil {
		t.Fatalf("expected local user linked to kratos identity, got error: %v", err)
	}
	if linkedUser.Email != "a@example.com" || linkedUser.Username != "auser" {
		t.Errorf("unexpected linked user: %+v", linkedUser)
	}
}

// TestKratosRegisterEmailAlreadyExists proves that a Kratos "duplicate
// identifier" validation error on the traits.email node maps to
// domain.ErrEmailAlreadyExists.
func TestKratosRegisterEmailAlreadyExists(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.registrationSubmitStatus = http.StatusBadRequest
	f.registrationSubmitBody = newDuplicateIdentifierFlowError("traits.email")

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.Register(context.Background(), "dup@example.com", "dupuser", "Str0ngP@ss1")
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("expected domain.ErrEmailAlreadyExists, got %v", err)
	}
}

// TestKratosRegisterUsernameAlreadyExists proves that a Kratos "duplicate
// identifier" validation error on the traits.username node maps to
// domain.ErrUsernameAlreadyExists.
func TestKratosRegisterUsernameAlreadyExists(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.registrationSubmitStatus = http.StatusBadRequest
	f.registrationSubmitBody = newDuplicateIdentifierFlowError("traits.username")

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.Register(context.Background(), "new@example.com", "dupuser", "Str0ngP@ss1")
	if !errors.Is(err, domain.ErrUsernameAlreadyExists) {
		t.Fatalf("expected domain.ErrUsernameAlreadyExists, got %v", err)
	}
}

// TestKratosLoginExistingLink proves that Login resolves the local user via
// an existing kratos_identity_id link without self-healing.
func TestKratosLoginExistingLink(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.loginSubmitBody = kratosLoginRespDTO{
		SessionToken: "login-session-token",
		Session: struct {
			Identity kratosIdentityDTO `json:"identity"`
		}{Identity: kratosIdentityDTO{ID: identityID, Traits: kratosTraitsDTO{Email: "b@example.com", Username: "buser"}}},
	}

	userRepo := &mocks.UserRepo{}
	preexisting := newSeedUser("b@example.com", "buser")
	preexisting.KratosIdentityID = &identityID
	if err := userRepo.Create(context.Background(), preexisting); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	svc := newTestKratosService(f, userRepo)

	pair, err := svc.Login(context.Background(), "b@example.com", "Str0ngP@ss1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if pair.AccessToken != "login-session-token" {
		t.Errorf("unexpected token pair: %+v", pair)
	}

	if got := len(userRepo.Users); got != 1 {
		t.Errorf("expected Login with an existing link not to create a second user, got %d users", got)
	}
}

// TestKratosLoginSelfHeal proves that Login creates a local user row from
// the session's identity traits when the Kratos identity has no existing
// local link (domain.ErrNotFound from GetByKratosIdentityID).
func TestKratosLoginSelfHeal(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.loginSubmitBody = kratosLoginRespDTO{
		SessionToken: "self-heal-session-token",
		Session: struct {
			Identity kratosIdentityDTO `json:"identity"`
		}{Identity: kratosIdentityDTO{ID: identityID, Traits: kratosTraitsDTO{Email: "c@example.com", Username: "cuser"}}},
	}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	pair, err := svc.Login(context.Background(), "c@example.com", "Str0ngP@ss1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if pair.AccessToken != "self-heal-session-token" {
		t.Errorf("unexpected token pair: %+v", pair)
	}

	linkedUser, err := userRepo.GetByKratosIdentityID(context.Background(), identityID)
	if err != nil {
		t.Fatalf("expected Login to self-heal a local user link, got error: %v", err)
	}
	if linkedUser.Email != "c@example.com" || linkedUser.Username != "cuser" {
		t.Errorf("unexpected self-healed user: %+v", linkedUser)
	}
}

// TestKratosLoginInvalidCredentials proves that a 400 response from the
// login flow maps to domain.ErrInvalidCredentials.
func TestKratosLoginInvalidCredentials(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.loginSubmitStatus = http.StatusBadRequest
	f.loginSubmitBody = map[string]string{"error": "invalid credentials"}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.Login(context.Background(), "nope@example.com", "wrong-password")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("expected domain.ErrInvalidCredentials, got %v", err)
	}
}

// TestKratosValidateTokenBearer proves the bare-token (X-Session-Token)
// ValidateToken path resolves the linked local user on a 200 whoami
// response.
func TestKratosValidateTokenBearer(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.whoamiBody = kratosWhoamiRespDTO{Identity: kratosIdentityDTO{ID: identityID}}

	userRepo := &mocks.UserRepo{}
	u := newSeedUser("d@example.com", "duser")
	u.KratosIdentityID = &identityID
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	svc := newTestKratosService(f, userRepo)

	claims, err := svc.ValidateToken(context.Background(), "opaque-native-token")
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.UserID != u.ID {
		t.Errorf("expected UserID %q, got %q", u.ID, claims.UserID)
	}
}

// TestKratosValidateTokenCookie proves the "cookie:"-prefixed ValidateToken
// path resolves the linked local user on a 200 whoami response.
func TestKratosValidateTokenCookie(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.whoamiBody = kratosWhoamiRespDTO{Identity: kratosIdentityDTO{ID: identityID}}

	userRepo := &mocks.UserRepo{}
	u := newSeedUser("e@example.com", "euser")
	u.KratosIdentityID = &identityID
	if err := userRepo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	svc := newTestKratosService(f, userRepo)

	claims, err := svc.ValidateToken(context.Background(), "cookie:some-cookie-value")
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.UserID != u.ID {
		t.Errorf("expected UserID %q, got %q", u.ID, claims.UserID)
	}
}

// TestKratosValidateTokenInvalid proves that a non-200 whoami response maps
// to domain.ErrInvalidToken.
func TestKratosValidateTokenInvalid(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.whoamiStatus = http.StatusUnauthorized
	f.whoamiBody = map[string]string{"error": "session not found"}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.ValidateToken(context.Background(), "expired-token")
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected domain.ErrInvalidToken, got %v", err)
	}
}

// TestKratosValidateTokenUnlinkedIdentity proves that a 200 whoami response
// for an identity with no local link maps to domain.ErrInvalidToken.
func TestKratosValidateTokenUnlinkedIdentity(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.whoamiBody = kratosWhoamiRespDTO{Identity: kratosIdentityDTO{ID: uuid.New().String()}}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.ValidateToken(context.Background(), "opaque-native-token")
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected domain.ErrInvalidToken, got %v", err)
	}
}
