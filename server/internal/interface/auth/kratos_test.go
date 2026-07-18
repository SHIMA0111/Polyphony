package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
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

	// whoamiExpectedSessionToken and whoamiExpectedCookieValue, when
	// non-empty, make the /sessions/whoami handler assert the request
	// carries that exact credential (X-Session-Token header or
	// "<cookieName>=<value>" Cookie header respectively) and respond 401 on
	// a missing/mismatched credential instead of the scripted whoamiStatus —
	// proving ValidateToken actually forwards the token/cookie it was given
	// rather than e.g. silently always sending an empty header.
	whoamiExpectedSessionToken string
	whoamiExpectedCookieValue  string
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
		if f.whoamiExpectedSessionToken != "" {
			if got := r.Header.Get(kratosHTTPHeaderSessionToken); got != f.whoamiExpectedSessionToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or mismatched session token"})
				return
			}
		}
		if f.whoamiExpectedCookieValue != "" {
			cookie, err := r.Cookie("ory_kratos_session")
			if err != nil || cookie.Value != f.whoamiExpectedCookieValue {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or mismatched session cookie"})
				return
			}
		}
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

// TestKratosRegisterMissingSessionToken proves that Register returns an
// explicit error (rather than a TokenPair with an empty AccessToken) when
// Kratos's registration flow response omits session_token, which happens in
// a real Kratos deployment when selfservice.flows.registration.after's
// password hooks do not include the "session" hook (see ory/kratos/kratos.yml).
// The other fakeKratos-based tests always set a non-empty SessionToken, which
// previously masked this gap.
func TestKratosRegisterMissingSessionToken(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.registrationSubmitBody = kratosRegistrationRespDTO{
		SessionToken: "",
		Identity:     kratosIdentityDTO{ID: identityID, Traits: kratosTraitsDTO{Email: "nohook@example.com", Username: "nohookuser"}},
	}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	pair, err := svc.Register(context.Background(), "nohook@example.com", "nohookuser", "Str0ngP@ss1")
	if err == nil {
		t.Fatalf("expected an error when registration response has no session_token, got token pair: %+v", pair)
	}
	if pair != nil {
		t.Errorf("expected a nil token pair on error, got: %+v", pair)
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

// TestKratosLoginRelinksExistingUserByEmail proves that ensureLocalUser's
// self-heal path relinks a pre-existing local users row (e.g. a
// pre-Kratos-migration SimpleJWT account, or one cmd/kratosmigrate hasn't
// backfilled yet) by matching the Kratos identity's email, instead of
// falling straight to Create — which would fail on the email unique
// constraint (domain.ErrEmailAlreadyExists) since a row with that email
// already exists, permanently locking that account out of Kratos login.
func TestKratosLoginRelinksExistingUserByEmail(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	existing := newSeedUser("relink@example.com", "relinkuser")
	userRepo := &mocks.UserRepo{}
	if err := userRepo.Create(context.Background(), existing); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}

	identityID := uuid.New().String()
	f.loginSubmitBody = kratosLoginRespDTO{
		SessionToken: "relink-session-token",
		Session: struct {
			Identity kratosIdentityDTO `json:"identity"`
		}{Identity: kratosIdentityDTO{ID: identityID, Traits: kratosTraitsDTO{Email: "relink@example.com", Username: "relinkuser"}}},
	}

	svc := newTestKratosService(f, userRepo)

	if _, err := svc.Login(context.Background(), "relink@example.com", "Str0ngP@ss1"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if got := len(userRepo.Users); got != 1 {
		t.Fatalf("expected relink not to create a second user row, got %d users", got)
	}

	linkedUser, err := userRepo.GetByKratosIdentityID(context.Background(), identityID)
	if err != nil {
		t.Fatalf("expected the existing user to be linked to the identity, got error: %v", err)
	}
	if linkedUser.ID != existing.ID {
		t.Fatalf("expected the pre-existing user %q to be relinked, got a different user %+v", existing.ID, linkedUser)
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
	f.whoamiExpectedSessionToken = "opaque-native-token"

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
	f.whoamiExpectedCookieValue = "some-cookie-value"

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

// TestKratosValidateTokenInvalid proves that a 401 whoami response maps to
// domain.ErrInvalidToken.
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

// TestKratosValidateTokenTransportError proves that a transport-level
// failure calling whoami (here, a closed server refusing the connection) is
// surfaced as a plain error rather than domain.ErrInvalidToken — the token
// itself was never evaluated, so treating it as an invalid credential would
// misleadingly surface as a 401 instead of a 5xx via
// interface/middleware.JWTAuth.
func TestKratosValidateTokenTransportError(t *testing.T) {
	f := newFakeKratos()
	svc := newTestKratosService(f, &mocks.UserRepo{})
	f.close() // close before use so the request fails at the transport level

	_, err := svc.ValidateToken(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected ValidateToken to fail when the transport call errors")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a plain transport error, not domain.ErrInvalidToken, got %v", err)
	}
}

// TestKratosValidateTokenServerError proves that a 5xx whoami response is
// surfaced as a plain error rather than domain.ErrInvalidToken, since a
// server error says nothing about whether the presented credential is
// valid.
func TestKratosValidateTokenServerError(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.whoamiStatus = http.StatusInternalServerError
	f.whoamiBody = map[string]string{"error": "internal error"}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.ValidateToken(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected ValidateToken to fail on a 500 whoami response")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a plain server error, not domain.ErrInvalidToken, got %v", err)
	}
}

// TestKratosValidateTokenUndecodableBody proves that a 200 whoami response
// whose body isn't valid JSON is surfaced as a plain error rather than
// domain.ErrInvalidToken — whoami confirmed the credential is valid (status
// 200); a malformed body is an upstream/decode problem, not evidence the
// token itself is bad.
func TestKratosValidateTokenUndecodableBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	svc := NewKratosAuthService(userRepo, server.URL, server.URL, "ory_kratos_session", server.Client())

	_, err := svc.ValidateToken(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected ValidateToken to fail when the whoami body doesn't decode")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a plain decode error, not domain.ErrInvalidToken, got %v", err)
	}
}

// TestKratosValidateTokenSelfHeal proves that ValidateToken self-heals a
// missing local user row from the whoami response's identity traits, the
// same way Login does, instead of returning domain.ErrInvalidToken — this
// is the only path the browser data-plane exercises (registration/login
// there go straight to Kratos via /api/kratos/*, never through this
// package's own Register/Login), so a freshly browser-registered identity
// must not 401 forever despite holding a valid Kratos session.
func TestKratosValidateTokenSelfHeal(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.whoamiBody = kratosWhoamiRespDTO{
		Identity: kratosIdentityDTO{
			ID:     identityID,
			Traits: kratosTraitsDTO{Email: "d@example.com", Username: "duser"},
		},
	}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	claims, err := svc.ValidateToken(context.Background(), "cookie:some-cookie-value")
	if err != nil {
		t.Fatalf("expected ValidateToken to self-heal a local user link, got error: %v", err)
	}

	linkedUser, err := userRepo.GetByKratosIdentityID(context.Background(), identityID)
	if err != nil {
		t.Fatalf("expected a self-healed local user link, got error: %v", err)
	}
	if linkedUser.Email != "d@example.com" || linkedUser.Username != "duser" {
		t.Errorf("unexpected self-healed user: %+v", linkedUser)
	}
	if claims.UserID != linkedUser.ID {
		t.Errorf("expected claims.UserID %q to match self-healed user %q", claims.UserID, linkedUser.ID)
	}
}

// TestKratosEnsureLocalUserConcurrentFirstLoginRace covers the
// concurrent-first-login race documented on ensureLocalUser: two callers
// racing ValidateToken for the same brand-new Kratos identity can both reach
// userRepo.Create for the same email/username, since both see
// GetByKratosIdentityID/GetByEmail miss before either has written. The
// loser must self-heal by re-resolving to the winner's row via
// GetByKratosIdentityID rather than surfacing a
// domain.ErrEmailAlreadyExists/domain.ErrUsernameAlreadyExists failure to
// the caller. Run with -race to confirm the mocks.UserRepo access itself is
// also race-free.
func TestKratosEnsureLocalUserConcurrentFirstLoginRace(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.whoamiBody = kratosWhoamiRespDTO{
		Identity: kratosIdentityDTO{
			ID:     identityID,
			Traits: kratosTraitsDTO{Email: "race@example.com", Username: "raceuser"},
		},
	}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	const goroutines = 2
	var wg sync.WaitGroup
	userIDs := make([]string, goroutines)
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			claims, err := svc.ValidateToken(context.Background(), "cookie:some-cookie-value")
			if err != nil {
				errs[i] = err
				return
			}
			userIDs[i] = claims.UserID
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: ValidateToken failed: %v", i, err)
		}
	}
	if userIDs[0] == "" || userIDs[1] == "" {
		t.Fatalf("expected both goroutines to resolve a user ID, got %q and %q", userIDs[0], userIDs[1])
	}
	if userIDs[0] != userIDs[1] {
		t.Fatalf("expected both concurrent first-logins to resolve to the same user, got %q and %q", userIDs[0], userIDs[1])
	}

	// Exactly one users row should have been created for this identity, not
	// two racing Create calls both succeeding.
	count := 0
	for _, u := range userRepo.Users {
		if u.Email == "race@example.com" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 local user row for the raced identity, got %d", count)
	}
}
