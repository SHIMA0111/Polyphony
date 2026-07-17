package wsticket

import (
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
)

func TestIssueValidateRoundTrip(t *testing.T) {
	issuer := NewIssuer([]byte("secret"), time.Minute)

	ticket, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	userID, err := issuer.Validate(ticket)
	if err != nil {
		t.Fatalf("Validate error: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("expected user-1, got %q", userID)
	}
}

func TestValidateExpiredTicketRejected(t *testing.T) {
	issuer := NewIssuer([]byte("secret"), -time.Second)

	ticket, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	_, err = issuer.Validate(ticket)
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected domain.ErrInvalidToken, got %v", err)
	}
}

func TestValidateTamperedTicketRejected(t *testing.T) {
	issuer := NewIssuer([]byte("secret"), time.Minute)

	ticket, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	tampered := []byte(ticket)
	// Flip a character in the signature segment (after the last '.').
	lastDot := -1
	for i := len(tampered) - 1; i >= 0; i-- {
		if tampered[i] == '.' {
			lastDot = i
			break
		}
	}
	if lastDot == -1 || lastDot == len(tampered)-1 {
		t.Fatalf("could not locate signature segment in ticket %q", ticket)
	}
	flipIdx := lastDot + 1
	if tampered[flipIdx] == 'A' {
		tampered[flipIdx] = 'B'
	} else {
		tampered[flipIdx] = 'A'
	}

	_, err = issuer.Validate(string(tampered))
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected domain.ErrInvalidToken, got %v", err)
	}
}

func TestValidateWrongSecretRejected(t *testing.T) {
	issuer := NewIssuer([]byte("secret-a"), time.Minute)
	otherIssuer := NewIssuer([]byte("secret-b"), time.Minute)

	ticket, err := issuer.Issue("user-1")
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}

	_, err = otherIssuer.Validate(ticket)
	if !errors.Is(err, domain.ErrInvalidToken) {
		t.Fatalf("expected domain.ErrInvalidToken, got %v", err)
	}
}
