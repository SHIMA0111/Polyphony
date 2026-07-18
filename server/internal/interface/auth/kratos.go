// Package auth defines the authentication service port (token issuance and validation).
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainauth "github.com/SHIMA0111/multi-user-ai/server/internal/domain/auth"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// kratosManagedPasswordHash is stored in users.password_hash for accounts
// whose credentials are owned by Ory Kratos rather than SimpleJWTService.
// It is never a valid argon2id PHC string (see simple_jwt.go's hashPassword),
// so SimpleJWTService.verifyPassword can never accidentally succeed against
// it; the column is NOT NULL, so a placeholder is still required.
const kratosManagedPasswordHash = "!kratos-managed!"

// kratosHTTPHeaderSessionToken is the header KratosAuthService.ValidateToken
// uses to present an opaque native/API session token to Kratos's
// /sessions/whoami endpoint, as documented at
// https://www.ory.sh/docs/kratos/session-management/overview.
const kratosHTTPHeaderSessionToken = "X-Session-Token"

// cookieTokenPrefix marks a token string passed to ValidateToken as an Ory
// Kratos session cookie value rather than an opaque native session token.
// See ValidateToken's GoDoc for the full contract.
const cookieTokenPrefix = "cookie:"

// ensureLocalUserLinkRetries is the number of GetByKratosIdentityID attempts
// ensureLocalUser makes after losing the Create race described in its
// GoDoc's "Concurrency" section, before giving up and returning the original
// conflict error.
const ensureLocalUserLinkRetries = 20

// ensureLocalUserLinkRetryDelay is the delay between each retry described on
// ensureLocalUserLinkRetries. Kept short: the winner's SetKratosIdentityID
// call is expected to land within microseconds of its Create, so this bounds
// the loser's worst-case extra latency to a few milliseconds while still
// reliably closing the race window.
const ensureLocalUserLinkRetryDelay = 1 * time.Millisecond

// KratosAuthService implements domainauth.AuthService using Ory Kratos's
// self-service registration/login API flows and the /sessions/whoami
// endpoint, with the caller's local users.id resolved through
// users.kratos_identity_id. It hand-rolls REST calls with net/http and
// encoding/json (mirroring the pattern in interface/gateway/llm_client.go)
// rather than depending on the full ory/client-go SDK.
type KratosAuthService struct {
	userRepo   user.UserRepository
	publicURL  string
	adminURL   string
	cookieName string
	httpClient *http.Client
}

// NewKratosAuthService creates a new KratosAuthService. publicURL and
// adminURL are Kratos's public (self-service flows, whoami) and admin
// (identity management) API base URLs respectively, with no trailing
// slash expected (e.g. "http://localhost:4433"). cookieName is the name of
// the session cookie Kratos issues (default "ory_kratos_session"); it is
// only used for documentation/consistency here since ValidateToken itself
// receives the cookie value already extracted by the caller (see
// ValidateToken's GoDoc). adminURL is retained on the struct for parity with
// the constructor signature required by container.go, even though this
// type's three AuthService methods do not currently call the Admin API
// (that is cmd/kratosmigrate's job).
func NewKratosAuthService(userRepo user.UserRepository, publicURL, adminURL, cookieName string, httpClient *http.Client) *KratosAuthService {
	return &KratosAuthService{
		userRepo:   userRepo,
		publicURL:  strings.TrimRight(publicURL, "/"),
		adminURL:   strings.TrimRight(adminURL, "/"),
		cookieName: cookieName,
		httpClient: httpClient,
	}
}

// --- Private request/response DTOs matching Kratos's self-service flow API ---

type kratosFlowDTO struct {
	ID string `json:"id"`
}

type kratosTraitsDTO struct {
	Email    string `json:"email"`
	Username string `json:"username"`
}

type kratosIdentityDTO struct {
	ID     string          `json:"id"`
	Traits kratosTraitsDTO `json:"traits"`
}

type kratosRegistrationReqDTO struct {
	Method   string          `json:"method"`
	Password string          `json:"password"`
	Traits   kratosTraitsDTO `json:"traits"`
}

type kratosRegistrationRespDTO struct {
	SessionToken string            `json:"session_token"`
	Identity     kratosIdentityDTO `json:"identity"`
}

type kratosLoginReqDTO struct {
	Method     string `json:"method"`
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type kratosLoginRespDTO struct {
	SessionToken string `json:"session_token"`
	Session      struct {
		Identity kratosIdentityDTO `json:"identity"`
	} `json:"session"`
}

type kratosWhoamiRespDTO struct {
	Identity kratosIdentityDTO `json:"identity"`
}

// kratosUIMessageDTO is a single UI message Kratos attaches to a flow node
// or to the flow itself (e.g. validation errors), per
// https://www.ory.sh/docs/kratos/concepts/ui-user-interface#messages.
type kratosUIMessageDTO struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Type string `json:"type"`
}

// kratosFlowErrorDTO is the shape of a Kratos self-service flow returned
// with a 4xx status when a registration/login attempt fails validation
// (e.g. duplicate identifier, wrong credentials).
type kratosFlowErrorDTO struct {
	UI struct {
		Messages []kratosUIMessageDTO `json:"messages"`
		Nodes    []struct {
			Attributes struct {
				Name string `json:"name"`
			} `json:"attributes"`
			Messages []kratosUIMessageDTO `json:"messages"`
		} `json:"nodes"`
	} `json:"ui"`
}

// Register creates a new Kratos identity via the self-service registration
// API flow (GET .../self-service/registration/api to obtain a flow ID, then
// POST .../self-service/registration?flow=<id> with the password method),
// then mirrors it into a local users row (with a KratosIdentityID link and
// an unusable placeholder PasswordHash, since Kratos now owns credential
// verification) via userRepo.Create followed by userRepo.SetKratosIdentityID.
//
// It returns domain.ErrEmailAlreadyExists or domain.ErrUsernameAlreadyExists
// if Kratos's registration flow reports the email or username trait is
// already taken by another identity.
func (s *KratosAuthService) Register(ctx context.Context, email, username, password string) (*domainauth.TokenPair, error) {
	flowID, err := s.fetchFlowID(ctx, "/self-service/registration/api")
	if err != nil {
		return nil, fmt.Errorf("fetch registration flow: %w", err)
	}

	reqBody := kratosRegistrationReqDTO{
		Method:   "password",
		Password: password,
		Traits:   kratosTraitsDTO{Email: email, Username: username},
	}

	var result kratosRegistrationRespDTO
	status, body, err := s.postFlow(ctx, "/self-service/registration", flowID, reqBody, &result)
	if err != nil {
		return nil, fmt.Errorf("submit registration flow: %w", err)
	}
	if status != http.StatusOK {
		if status >= 400 && status < 500 {
			return nil, classifyRegistrationError(body)
		}
		return nil, fmt.Errorf("kratos registration failed: status %d: %s", status, string(body))
	}

	if result.SessionToken == "" {
		// Kratos only returns a session_token on the registration flow's
		// response when an `after` hook of type "session" is configured for
		// the password method (see ory/kratos/kratos.yml). Without it, the
		// flow succeeds (the identity is created) but the response contains
		// only identity/continue_with and no usable session — returning an
		// empty TokenPair here would surface as a misleading 201 with a
		// blank access_token, so fail loudly instead.
		return nil, fmt.Errorf("kratos registration succeeded but returned no session_token: is the registration flow's after.password.hooks missing the session hook?")
	}

	if _, err := s.ensureLocalUser(ctx, result.Identity.ID, email, username); err != nil {
		return nil, err
	}

	return &domainauth.TokenPair{AccessToken: result.SessionToken, TokenType: "Bearer"}, nil
}

// Login authenticates against Kratos's self-service login API flow (GET
// .../self-service/login/api to obtain a flow ID, then POST
// .../self-service/login?flow=<id> with the password method), then resolves
// the caller's local user via userRepo.GetByKratosIdentityID.
//
// If the Kratos identity has no local link yet (domain.ErrNotFound from
// GetByKratosIdentityID — e.g. the identity was created directly via the
// Kratos Admin API, bypassing Register above), Login self-heals by creating
// the local row from the session's identity traits, the same way Register
// does, so a subsequent call resolves without repeating this path.
//
// It returns domain.ErrInvalidCredentials if Kratos's login flow reports
// the identifier/password combination is invalid.
func (s *KratosAuthService) Login(ctx context.Context, email, password string) (*domainauth.TokenPair, error) {
	flowID, err := s.fetchFlowID(ctx, "/self-service/login/api")
	if err != nil {
		return nil, fmt.Errorf("fetch login flow: %w", err)
	}

	reqBody := kratosLoginReqDTO{Method: "password", Identifier: email, Password: password}

	var result kratosLoginRespDTO
	status, body, err := s.postFlow(ctx, "/self-service/login", flowID, reqBody, &result)
	if err != nil {
		return nil, fmt.Errorf("submit login flow: %w", err)
	}
	if status != http.StatusOK {
		if status >= 400 && status < 500 {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("kratos login failed: status %d: %s", status, string(body))
	}

	identity := result.Session.Identity
	if _, err := s.userRepo.GetByKratosIdentityID(ctx, identity.ID); err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}

		if _, err := s.ensureLocalUser(ctx, identity.ID, identity.Traits.Email, identity.Traits.Username); err != nil {
			return nil, err
		}
	}

	return &domainauth.TokenPair{AccessToken: result.SessionToken, TokenType: "Bearer"}, nil
}

// ensureLocalUser resolves the local users row for a Kratos identity
// (identityID, with the given email/username traits), creating or relinking
// it as needed:
//
//   - If a local user already exists matching email EXACTLY and has no
//     kratos_identity_id yet, it is linked to identityID via
//     SetKratosIdentityID and returned. This is the "re-login of a
//     pre-Kratos local user" path: a users row created by SimpleJWTService
//     before AUTH_MODE switched to "kratos", or one created directly via
//     the Kratos Admin API (bypassing Register) without being linked yet.
//     Without this lookup, this path would instead fall through to Create
//     below, which — for a genuinely pre-existing email — fails on the
//     unique constraint (or, absent that constraint, would create a
//     duplicate local account for the same person).
//   - Matching is deliberately email-only, never username. A username
//     match with a different email must NOT be treated as the same person:
//     an attacker who registers a Kratos identity whose username trait
//     happens to collide with a victim's local username (but uses their
//     own, different email) must not have the victim's unlinked local
//     account silently linked to the attacker's identity — that would be
//     an account takeover. Such a collision instead falls through to
//     Create, which fails on the username unique constraint and surfaces
//     domain.ErrUsernameAlreadyExists to the caller.
//   - A local user matching email but already linked to a *different*
//     Kratos identity is treated as no match (falls through to Create),
//     since relinking it here would silently reassign someone else's
//     account.
//   - Otherwise, a brand new local user row is created, with a
//     kratos-managed placeholder password hash, linked to identityID.
//
// Used by Register (a Kratos-side registration for an email that already
// has a local-only, unlinked user record), Login's self-heal path (a
// Kratos identity with no local link yet), and ValidateToken's self-heal
// path (see its GoDoc) — the single shared implementation all three rely on
// so none of them can silently diverge from the others.
//
// Concurrency: two callers racing to be the first to resolve the same brand
// new Kratos identity (e.g. two near-simultaneous requests that both
// observed GetByKratosIdentityID return domain.ErrNotFound) both reach the
// Create call below. Only one Create can win the unique constraint on
// email/username; the loser's Create returns
// domain.ErrEmailAlreadyExists/domain.ErrUsernameAlreadyExists. Rather than
// surfacing that as a hard failure to the losing caller, ensureLocalUser
// re-resolves via GetByKratosIdentityID: the winner has (or is about to have,
// within ensureLocalUserLinkRetries short retries) also called
// SetKratosIdentityID, so the loser's retry should find the same linked row
// and both callers converge on the same single user. If every retry still
// reports domain.ErrNotFound (a genuine, non-race conflict — e.g. the
// email/username collides with an unrelated, already-linked-to-someone-else
// account, or a genuine username-only collision per the bullet above), the
// original Create error is returned unchanged.
func (s *KratosAuthService) ensureLocalUser(ctx context.Context, identityID, email, username string) (*user.User, error) {
	existing, err := s.lookupUnlinkedLocalUser(ctx, email)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		if err := s.userRepo.SetKratosIdentityID(ctx, existing.ID, identityID); err != nil {
			return nil, err
		}
		linked := identityID
		existing.KratosIdentityID = &linked
		return existing, nil
	}

	now := time.Now()
	u := &user.User{
		ID:               uuid.New().String(),
		Email:            email,
		Username:         username,
		PasswordHash:     kratosManagedPasswordHash,
		KratosIdentityID: &identityID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.userRepo.Create(ctx, u); err != nil {
		if errors.Is(err, domain.ErrEmailAlreadyExists) || errors.Is(err, domain.ErrUsernameAlreadyExists) {
			// Likely a concurrent first-login race: another goroutine's
			// Create won and has (or is imminently about to have) linked
			// identityID via its own SetKratosIdentityID call. Re-resolve by
			// identity, briefly retrying to close the tiny window between the
			// winner's Create and its SetKratosIdentityID, rather than
			// failing this caller outright.
			for attempt := 0; attempt < ensureLocalUserLinkRetries; attempt++ {
				winner, getErr := s.userRepo.GetByKratosIdentityID(ctx, identityID)
				if getErr == nil {
					return winner, nil
				}
				if !errors.Is(getErr, domain.ErrNotFound) {
					return nil, getErr
				}
				if attempt < ensureLocalUserLinkRetries-1 {
					time.Sleep(ensureLocalUserLinkRetryDelay)
				}
			}
		}
		return nil, err
	}
	if err := s.userRepo.SetKratosIdentityID(ctx, u.ID, identityID); err != nil {
		return nil, err
	}
	return u, nil
}

// lookupUnlinkedLocalUser looks up an existing local user matching email
// EXACTLY that has no kratos_identity_id yet. Returns (nil, nil) — not an
// error — if no such row exists, or if the only email match already has a
// (necessarily different, since the caller already checked
// GetByKratosIdentityID) Kratos identity linked.
//
// Deliberately does not fall back to a username match: a local user whose
// username merely collides with the incoming identity's username trait,
// but whose email differs, is NOT the same person and must not be relinked
// here. Doing so would let an attacker take over a victim's account by
// registering a Kratos identity with the victim's username and the
// attacker's own email — see ensureLocalUser's GoDoc for the full threat
// model. A username-only collision instead falls through to
// ensureLocalUser's Create call, which fails on the username unique
// constraint and surfaces domain.ErrUsernameAlreadyExists.
func (s *KratosAuthService) lookupUnlinkedLocalUser(ctx context.Context, email string) (*user.User, error) {
	byEmail, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil {
		if byEmail.KratosIdentityID == nil {
			return byEmail, nil
		}
		return nil, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	return nil, nil
}

// ValidateToken validates a token against Kratos's GET /sessions/whoami
// endpoint and resolves the local user via ensureLocalUser, self-healing a
// missing local row from the whoami response's identity traits exactly as
// Login does — this is the only path the browser data-plane exercises
// (registration/login there go straight to Kratos via /api/kratos/*, never
// through this package's Register/Login), so without this self-heal a
// freshly browser-registered identity would 401 forever despite holding a
// perfectly valid Kratos session.
//
// token is interpreted using the following convention, which is the
// contract that interface/middleware.JWTAuth relies on: if token has the
// prefix "cookie:", the remainder is treated as the value of the Kratos
// session cookie and sent as a Cookie header (Cookie: <cookieName>=<value>);
// otherwise token is treated as an opaque native/API session token (as
// returned by Register/Login above) and sent via the X-Session-Token header.
// This prefix exists because SimpleJWTService.ValidateToken never receives
// a "cookie:"-prefixed string in practice (SimpleJWT never sets a browser
// cookie), so the distinction only matters when AUTH_MODE=kratos.
//
// It returns domain.ErrInvalidToken only for the whoami responses that
// genuinely mean "this session is not valid" — a 401 or 403 status. Anything
// else — the httpClient.Do call itself failing (e.g. Kratos unreachable), a
// whoami status that is neither 200 nor 401/403 (e.g. a 5xx), a 200 response
// whose body fails to decode, or ensureLocalUser's self-heal failing — is a
// server-side/infrastructure problem, not evidence of an invalid token, and
// is returned as a plain wrapped error instead. This distinction matters
// because interface/middleware.JWTAuth maps domain.ErrInvalidToken to a 401
// and anything else to a 5xx; flattening every failure mode here into
// ErrInvalidToken would misreport a Kratos outage as "your session expired"
// instead of a server error.
func (s *KratosAuthService) ValidateToken(ctx context.Context, token string) (*domainauth.Claims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.publicURL+"/sessions/whoami", nil)
	if err != nil {
		return nil, fmt.Errorf("build whoami request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	if strings.HasPrefix(token, cookieTokenPrefix) {
		cookieValue := strings.TrimPrefix(token, cookieTokenPrefix)
		req.Header.Set("Cookie", fmt.Sprintf("%s=%s", s.cookieName, cookieValue))
	} else {
		req.Header.Set(kratosHTTPHeaderSessionToken, token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kratos whoami request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, domain.ErrInvalidToken
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kratos whoami failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result kratosWhoamiRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode kratos whoami response: %w", err)
	}

	identity := result.Identity
	localUser, err := s.userRepo.GetByKratosIdentityID(ctx, identity.ID)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("resolve local user for kratos identity: %w", err)
		}

		// No local user is linked to this identity yet. Rather than treating
		// that as an invalid token (which would 401 forever a perfectly
		// valid Kratos session — the browser data-plane only ever exercises
		// this method, never Register/Login above), self-heal exactly as
		// Login does: relink an existing unlinked local user matching
		// email/username, or create a new one.
		localUser, err = s.ensureLocalUser(ctx, identity.ID, identity.Traits.Email, identity.Traits.Username)
		if err != nil {
			return nil, fmt.Errorf("resolve local user for kratos identity: %w", err)
		}
	}

	return &domainauth.Claims{UserID: localUser.ID}, nil
}

// fetchFlowID performs the GET .../self-service/{registration,login}/api
// request that initializes a native/API self-service flow, and returns the
// flow ID to submit with the follow-up POST.
func (s *KratosAuthService) fetchFlowID(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.publicURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var flow kratosFlowDTO
	if err := json.NewDecoder(resp.Body).Decode(&flow); err != nil {
		return "", fmt.Errorf("decode flow: %w", err)
	}
	return flow.ID, nil
}

// postFlow submits the given request body to path?flow=<flowID> and
// decodes the response body into out on a 200 response. It returns the raw
// status code and (unparsed) response body regardless of status, so callers
// can inspect a non-200 body for Kratos's structured validation errors.
func (s *KratosAuthService) postFlow(ctx context.Context, path, flowID string, reqBody, out interface{}) (int, []byte, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s%s?flow=%s", s.publicURL, path, flowID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.StatusCode, body, fmt.Errorf("decode response: %w", err)
		}
	}

	return resp.StatusCode, body, nil
}

// classifyRegistrationError inspects a Kratos registration flow's 4xx
// response body for a "duplicate identifier" validation message on the
// traits.email or traits.username node and maps it to
// domain.ErrEmailAlreadyExists / domain.ErrUsernameAlreadyExists
// respectively. If no such message is found, it returns a generic error
// wrapping the raw response body.
func classifyRegistrationError(body []byte) error {
	var flowErr kratosFlowErrorDTO
	if err := json.Unmarshal(body, &flowErr); err == nil {
		for _, node := range flowErr.UI.Nodes {
			for _, msg := range node.Messages {
				if !isDuplicateIdentifierMessage(msg) {
					continue
				}
				switch node.Attributes.Name {
				case "traits.email":
					return domain.ErrEmailAlreadyExists
				case "traits.username":
					return domain.ErrUsernameAlreadyExists
				}
			}
		}
		for _, msg := range flowErr.UI.Messages {
			if isDuplicateIdentifierMessage(msg) {
				// Kratos's identity schema configures email (not username)
				// as the password credential identifier, so a top-level
				// duplicate-identifier message always refers to email.
				return domain.ErrEmailAlreadyExists
			}
		}
	}
	return fmt.Errorf("kratos registration validation failed: %s", string(body))
}

// isDuplicateIdentifierMessage reports whether msg is Kratos's "an account
// with the same identifier exists already" validation error (message ID
// 4000007 per https://www.ory.sh/docs/kratos/concepts/ui-user-interface).
func isDuplicateIdentifierMessage(msg kratosUIMessageDTO) bool {
	return msg.ID == 4000007 || strings.Contains(strings.ToLower(msg.Text), "exists already")
}
