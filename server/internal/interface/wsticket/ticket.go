// Package wsticket mints and validates short-lived, single-purpose tickets
// used to authenticate WebSocket upgrade requests.
//
// Browsers cannot attach custom headers (such as "Authorization: Bearer
// ...") to a WebSocket handshake request, so the existing JWTAuth middleware
// cannot protect the upgrade endpoint directly. Instead, an already
// authenticated REST call (POST /ws/ticket) mints a ticket that the client
// then passes as a query parameter on the WebSocket URL.
//
// Issuer is deliberately independent of domain/auth.AuthService: it only
// needs a userID string, which callers obtain from the already-validated
// request context (via middleware.GetUserID). This keeps the WebSocket
// upgrade path working unchanged regardless of which concrete
// auth.AuthService implementation issued the original session token —
// today's SimpleJWTService or, after the Phase 9 swap, KratosAuthService.
// Nothing in this package imports domain/auth.
package wsticket

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
)

// ticketIssuerName is the JWT "iss" (issuer) claim used to identify tokens
// minted by this package, distinguishing them from ordinary session tokens
// issued by auth.AuthService even though both happen to use HS256 JWTs.
const ticketIssuerName = "polyphony-ws-ticket"

// Issuer mints and validates short-lived WebSocket authentication tickets.
// A ticket is an HS256 JWT whose only claims are the subject (user ID),
// issuer, issued-at, and expiry; it carries no other authorization
// information and is not itself a session token.
//
// The zero value is not usable; construct with NewIssuer.
type Issuer struct {
	secret []byte
	ttl    time.Duration
}

// NewIssuer creates an Issuer that signs tickets with secret and gives them
// a lifetime of ttl. Tickets should be short-lived (the caller typically
// passes on the order of tens of seconds) since they exist only to bridge
// the gap between an authenticated REST call and the immediately-following
// WebSocket upgrade.
func NewIssuer(secret []byte, ttl time.Duration) *Issuer {
	return &Issuer{secret: secret, ttl: ttl}
}

// Issue mints a signed ticket for userID. The ticket carries userID as its
// subject, ticketIssuerName as its issuer, and expires after the Issuer's
// configured ttl (measured from the current time). It returns an error only
// if signing itself fails, which does not happen for HS256 with a non-nil
// secret.
func (i *Issuer) Issue(userID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		Issuer:    ticketIssuerName,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(i.secret)
}

// Validate parses and verifies ticket, checking its signature, expiry, and
// issuer. On success it returns the user ID carried in the ticket's subject
// claim. It returns domain.ErrInvalidToken for any failure: malformed
// input, an unexpected signing method, an expired ticket, a signature that
// does not verify against this Issuer's secret, or an issuer claim other
// than ticketIssuerName (which rejects an ordinary session token being
// replayed as a ticket, even though both are HS256 JWTs).
func (i *Issuer) Validate(ticket string) (string, error) {
	token, err := jwt.Parse(ticket, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return i.secret, nil
	})
	if err != nil {
		return "", domain.ErrInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", domain.ErrInvalidToken
	}

	iss, err := claims.GetIssuer()
	if err != nil || iss != ticketIssuerName {
		return "", domain.ErrInvalidToken
	}

	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return "", domain.ErrInvalidToken
	}

	return sub, nil
}
