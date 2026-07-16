// Package auth provides concrete implementations of the domain/auth
// AuthService port: SimpleJWTService (self-hosted argon2+JWT), KratosAuthService
// (backed by Ory Kratos's self-service flows), and CachedAuthService (a
// Redis-caching decorator that wraps either of the other two).
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

	if _, err := s.ensureLocalUser(ctx, result.Identity); err != nil {
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
	localUser, err := s.ensureLocalUser(ctx, identity)
	if err != nil {
		return nil, err
	}
	_ = localUser // resolved for parity with the interface contract; not needed in the returned TokenPair

	return &domainauth.TokenPair{AccessToken: result.SessionToken, TokenType: "Bearer"}, nil
}

// ensureLocalUser resolves identity to a local users row via
// userRepo.GetByKratosIdentityID, self-healing a missing link when Kratos
// knows about an identity that this app's database has not (yet) mirrored
// to that identity.
//
// A missing link is resolved in one of two ways, tried in order:
//  1. Relink by traits: if a local users row already exists with this
//     identity's email (e.g. a pre-Kratos-migration SimpleJWT account not
//     yet backfilled by cmd/kratosmigrate, or a row created by Register but
//     whose SetKratosIdentityID call raced/failed independently of Kratos's
//     own identity creation), that existing row is linked to identityID via
//     SetKratosIdentityID and returned as-is. This must be tried before
//     creating a new row: falling straight to Create below with an email
//     that already exists in users would fail on the column's unique
//     constraint (domain.ErrEmailAlreadyExists), turning a self-heal
//     opportunity into a permanent login failure for that account.
//  2. Create: only if no local row exists for this email at all (e.g. an
//     identity created directly via the Kratos Admin API for a brand-new
//     user) is a new row created and linked.
//
// This is the single shared implementation Login and ValidateToken both
// rely on so neither path can silently diverge from the other.
func (s *KratosAuthService) ensureLocalUser(ctx context.Context, identity kratosIdentityDTO) (*user.User, error) {
	localUser, err := s.userRepo.GetByKratosIdentityID(ctx, identity.ID)
	if err == nil {
		return localUser, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	identityID := identity.ID

	if existing, err := s.userRepo.GetByEmail(ctx, identity.Traits.Email); err == nil {
		if err := s.userRepo.SetKratosIdentityID(ctx, existing.ID, identityID); err != nil {
			return nil, err
		}
		linkedID := identityID
		existing.KratosIdentityID = &linkedID
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	now := time.Now()
	u := &user.User{
		ID:               uuid.New().String(),
		Email:            identity.Traits.Email,
		Username:         identity.Traits.Username,
		PasswordHash:     kratosManagedPasswordHash,
		KratosIdentityID: &identityID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.userRepo.Create(ctx, u); err != nil {
		return nil, err
	}
	if err := s.userRepo.SetKratosIdentityID(ctx, u.ID, identityID); err != nil {
		return nil, err
	}
	return u, nil
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
// It returns domain.ErrInvalidToken if the request cannot be built, the
// HTTP call itself fails, whoami responds with a non-200 status, or the
// response body cannot be decoded — every case where the presented
// credential itself is the problem. If ensureLocalUser subsequently fails
// (a repository error resolving/creating/linking the local user), that
// error is wrapped and returned as-is rather than flattened to
// domain.ErrInvalidToken, since by that point whoami has already confirmed
// the token is valid — see interface/middleware.JWTAuth, which relies on
// this distinction to map the two cases to 401 and 500 respectively.
func (s *KratosAuthService) ValidateToken(ctx context.Context, token string) (*domainauth.Claims, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.publicURL+"/sessions/whoami", nil)
	if err != nil {
		return nil, domain.ErrInvalidToken
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
		return nil, domain.ErrInvalidToken
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, domain.ErrInvalidToken
	}

	var result kratosWhoamiRespDTO
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, domain.ErrInvalidToken
	}

	localUser, err := s.ensureLocalUser(ctx, result.Identity)
	if err != nil {
		// A failure here is always a genuine infrastructure problem (a
		// repository error while looking up or creating/linking the local
		// user) rather than evidence the token itself is invalid — whoami
		// already returned 200 and a decodable identity above. Flattening
		// this to domain.ErrInvalidToken would make interface/middleware.
		// JWTAuth surface a misleading 401 instead of a 5xx for what is
		// really a server-side failure.
		return nil, fmt.Errorf("resolve local user for kratos identity: %w", err)
	}

	return &domainauth.Claims{UserID: localUser.ID}, nil
}

// Revoke implements domainauth.Revoker by calling Kratos's session-revocation
// endpoint, DELETE {publicURL}/self-service/logout/api, so a logged-out
// session is rejected by ValidateToken/GET /sessions/whoami immediately,
// rather than only after CachedAuthService's whoami-cache TTL elapses.
//
// token is interpreted using the same "cookie:"-prefix convention documented
// on ValidateToken: if token has the cookieTokenPrefix prefix, the remainder
// is sent as a Cookie header (Cookie: <cookieName>=<value>) — matching
// Kratos's browser-facing logout flow, which identifies the session from its
// cookie; otherwise token is treated as an opaque native/API session token
// and sent via the same kratosHTTPHeaderSessionToken header ValidateToken
// uses. Per Ory Kratos v1.3.1's self-service logout API
// (https://www.ory.sh/docs/kratos/session-management/logout), the API
// variant accepts a JSON body of {"session_token": "<token>"}; this method
// sends that body alongside the header for the non-cookie case so the
// request is valid regardless of which credential-presentation Kratos
// inspects.
//
// A 200 or 204 response is treated as success. A 401 or 404 response (the
// session is already gone, e.g. a double logout or a session that already
// expired) is also treated as success: logout is inherently idempotent, and
// callers (CachedAuthService.Revoke, AuthUsecase.Logout) must not surface an
// error just because the session no longer exists server-side. Any other
// status, or a transport-level error, is returned as an error.
func (s *KratosAuthService) Revoke(ctx context.Context, token string) error {
	url := s.publicURL + "/self-service/logout/api"

	var req *http.Request
	var err error
	if strings.HasPrefix(token, cookieTokenPrefix) {
		req, err = http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			return fmt.Errorf("create logout request: %w", err)
		}
		cookieValue := strings.TrimPrefix(token, cookieTokenPrefix)
		req.Header.Set("Cookie", fmt.Sprintf("%s=%s", s.cookieName, cookieValue))
	} else {
		data, marshalErr := json.Marshal(struct {
			SessionToken string `json:"session_token"`
		}{SessionToken: token})
		if marshalErr != nil {
			return fmt.Errorf("marshal logout request body: %w", marshalErr)
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodDelete, url, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("create logout request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(kratosHTTPHeaderSessionToken, token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send logout request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusUnauthorized, http.StatusNotFound:
		return nil
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kratos logout failed: status %d: %s", resp.StatusCode, string(respBody))
	}
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
