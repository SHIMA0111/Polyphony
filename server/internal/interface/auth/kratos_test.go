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

	// expectSessionToken and expectCookieValue, when non-empty, make the
	// /sessions/whoami handler assert the incoming request actually carries
	// the credential ValidateToken is documented to send (see
	// KratosAuthService.ValidateToken's GoDoc): expectSessionToken checks the
	// X-Session-Token header, expectCookieValue checks the cookieName cookie
	// (see cookieName below). On a missing or mismatched credential the
	// handler responds 401, regardless of whoamiStatus/whoamiBody — this
	// catches a ValidateToken regression that silently drops the
	// credential from the outgoing request instead of forwarding it to
	// Kratos. At most one of the two is expected to be set per test, since a
	// single ValidateToken call sends only one of the two credential forms.
	expectSessionToken string
	expectCookieValue  string
	cookieName         string
}

func newFakeKratos() *fakeKratos {
	f := &fakeKratos{
		registrationFlowStatus:   http.StatusOK,
		loginFlowStatus:          http.StatusOK,
		registrationSubmitStatus: http.StatusOK,
		loginSubmitStatus:        http.StatusOK,
		whoamiStatus:             http.StatusOK,
		// Matches the cookieName newTestKratosService passes to
		// NewKratosAuthService, so expectCookieValue checks the same cookie
		// ValidateToken actually sets.
		cookieName: "ory_kratos_session",
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
		if f.expectSessionToken != "" && r.Header.Get(kratosHTTPHeaderSessionToken) != f.expectSessionToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if f.expectCookieValue != "" {
			cookie, err := r.Cookie(f.cookieName)
			if err != nil || cookie.Value != f.expectCookieValue {
				w.WriteHeader(http.StatusUnauthorized)
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

// TestKratosLoginRelinksPreexistingUnlinkedUser proves that Login relinks a
// pre-existing local user (matching email, with a nil kratos_identity_id —
// e.g. a SimpleJWT-era row, or one created directly via the Kratos Admin API
// without going through Register) rather than creating a duplicate second
// user row for the same person: the user count stays 1, and the identity ID
// ends up set on the original row.
func TestKratosLoginRelinksPreexistingUnlinkedUser(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	f.loginSubmitBody = kratosLoginRespDTO{
		SessionToken: "relink-session-token",
		Session: struct {
			Identity kratosIdentityDTO `json:"identity"`
		}{Identity: kratosIdentityDTO{ID: identityID, Traits: kratosTraitsDTO{Email: "f@example.com", Username: "fuser"}}},
	}

	userRepo := &mocks.UserRepo{}
	preexisting := newSeedUser("f@example.com", "fuser")
	if err := userRepo.Create(context.Background(), preexisting); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	svc := newTestKratosService(f, userRepo)

	pair, err := svc.Login(context.Background(), "f@example.com", "Str0ngP@ss1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if pair.AccessToken != "relink-session-token" {
		t.Errorf("unexpected token pair: %+v", pair)
	}

	if got := len(userRepo.Users); got != 1 {
		t.Errorf("expected relinking not to create a second user, got %d users", got)
	}

	got, err := userRepo.GetByID(context.Background(), preexisting.ID)
	if err != nil {
		t.Fatalf("get preexisting user: %v", err)
	}
	if got.KratosIdentityID == nil || *got.KratosIdentityID != identityID {
		t.Fatalf("expected preexisting user to be linked to identity %q, got %+v", identityID, got.KratosIdentityID)
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
	// Asserts ValidateToken actually forwards the bare token via the
	// X-Session-Token header — a mismatched/missing header makes the fake
	// respond 401, which would fail this test.
	f.expectSessionToken = "opaque-native-token"

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
	// Asserts ValidateToken actually forwards the cookie value via a Cookie
	// header named after cookieName — a mismatched/missing cookie makes the
	// fake respond 401, which would fail this test.
	f.expectCookieValue = "some-cookie-value"

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

// failingRoundTripper is an http.RoundTripper that always fails, simulating
// a transport-level failure (e.g. Kratos unreachable) rather than any kind
// of HTTP response.
type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("simulated transport failure")
}

// TestKratosValidateTokenTransportErrorIsNotInvalidToken proves that a
// transport-level failure calling Kratos's whoami endpoint (e.g. Kratos is
// unreachable) is returned as a plain wrapped error, not
// domain.ErrInvalidToken — being unable to reach Kratos at all says nothing
// about whether the presented token/session is valid.
func TestKratosValidateTokenTransportErrorIsNotInvalidToken(t *testing.T) {
	userRepo := &mocks.UserRepo{}
	svc := NewKratosAuthService(userRepo, "http://127.0.0.1:1", "http://127.0.0.1:1", "ory_kratos_session", &http.Client{Transport: failingRoundTripper{}})

	_, err := svc.ValidateToken(context.Background(), "opaque-native-token")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a non-ErrInvalidToken error, got %v", err)
	}
}

// TestKratosValidateTokenServerErrorIsNotInvalidToken proves that a whoami
// status that is neither 200 nor 401/403 (e.g. a 500 from a Kratos outage)
// is returned as a plain wrapped error, not domain.ErrInvalidToken.
func TestKratosValidateTokenServerErrorIsNotInvalidToken(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.whoamiStatus = http.StatusInternalServerError
	f.whoamiBody = map[string]string{"error": "internal server error"}

	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	_, err := svc.ValidateToken(context.Background(), "opaque-native-token")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a non-ErrInvalidToken error, got %v", err)
	}
}

// TestKratosValidateTokenUndecodableBodyIsNotInvalidToken proves that a 200
// whoami response whose body fails to decode as JSON is returned as a plain
// wrapped error, not domain.ErrInvalidToken — a malformed response body from
// Kratos is not evidence the caller's token/session is invalid.
func TestKratosValidateTokenUndecodableBodyIsNotInvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	userRepo := &mocks.UserRepo{}
	svc := NewKratosAuthService(userRepo, server.URL, server.URL, "ory_kratos_session", server.Client())

	_, err := svc.ValidateToken(context.Background(), "opaque-native-token")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a non-ErrInvalidToken error, got %v", err)
	}
}

// repoFailureUserRepo is a minimal domainuser.UserRepository test double
// whose GetByKratosIdentityID always fails with a fixed, non-ErrNotFound
// error (e.g. simulating a database outage). Embedding the (nil) interface
// means every other method panics if called — fine here, since
// ValidateToken only ever calls GetByKratosIdentityID.
type repoFailureUserRepo struct {
	domainuser.UserRepository
	err error
}

func (r *repoFailureUserRepo) GetByKratosIdentityID(_ context.Context, _ string) (*domainuser.User, error) {
	return nil, r.err
}

// TestKratosValidateTokenRepoFailureIsNotInvalidToken proves that
// ValidateToken does not flatten a genuine repository failure (as opposed to
// "no user linked to this identity") into domain.ErrInvalidToken: the
// caller (interface/middleware.JWTAuth) relies on this distinction to
// surface a 5xx instead of misreporting an infrastructure failure as an
// invalid/expired token.
func TestKratosValidateTokenRepoFailureIsNotInvalidToken(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	f.whoamiBody = kratosWhoamiRespDTO{Identity: kratosIdentityDTO{ID: uuid.New().String()}}

	repoErr := errors.New("database is unreachable")
	svc := newTestKratosService(f, nil)
	svc.userRepo = &repoFailureUserRepo{err: repoErr}

	_, err := svc.ValidateToken(context.Background(), "opaque-native-token")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected a non-ErrInvalidToken error wrapping the repo failure, got %v", err)
	}
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected the returned error to wrap the original repo failure, got %v", err)
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

// TestKratosEnsureLocalUserConcurrentFirstLoginsConverge is a regression
// test for a race in ensureLocalUser: two callers resolving the very same
// brand-new Kratos identity for the first time (both observing "no local
// user linked yet") can both reach the Create call. Only one Create can win
// the unique email/username constraint; the loser must retry via
// GetByKratosIdentityID and resolve to the *same* single user rather than
// failing outright. Run with -race to also catch any data race in the
// retry path itself.
func TestKratosEnsureLocalUserConcurrentFirstLoginsConverge(t *testing.T) {
	f := newFakeKratos()
	defer f.close()

	identityID := uuid.New().String()
	userRepo := &mocks.UserRepo{}
	svc := newTestKratosService(f, userRepo)

	const goroutines = 2
	var wg sync.WaitGroup
	results := make([]*domainuser.User, goroutines)
	errs := make([]error, goroutines)

	// A start barrier maximizes the chance both goroutines are mid-Create at
	// the same time, rather than serializing behind mocks.UserRepo's mutex
	// before either has read anything.
	var ready sync.WaitGroup
	ready.Add(goroutines)
	start := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ready.Done()
			<-start
			u, err := svc.ensureLocalUser(context.Background(), identityID, "racer@example.com", "racer")
			results[idx] = u
			errs[idx] = err
		}(i)
	}

	ready.Wait()
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: ensureLocalUser failed: %v", i, err)
		}
	}
	if results[0] == nil || results[1] == nil {
		t.Fatalf("expected both goroutines to resolve a user, got %+v", results)
	}
	if results[0].ID != results[1].ID {
		t.Fatalf("expected both goroutines to converge on the same user, got %q and %q", results[0].ID, results[1].ID)
	}

	// Exactly one local user should have been created for this identity, not
	// two duplicate rows.
	linked, err := userRepo.GetByKratosIdentityID(context.Background(), identityID)
	if err != nil {
		t.Fatalf("expected a linked user, got error: %v", err)
	}
	if linked.ID != results[0].ID {
		t.Fatalf("expected the linked user %q to match the resolved user %q", linked.ID, results[0].ID)
	}
}
