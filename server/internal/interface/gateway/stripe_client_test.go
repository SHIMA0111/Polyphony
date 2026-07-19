package gateway

import (
	"errors"
	"fmt"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
)

// signWebhookPayload builds the Stripe-Signature header value ConstructEvent
// expects for payload signed with secret at t, using the same v1 HMAC
// scheme webhook.ConstructEvent verifies against
// (https://stripe.com/docs/webhooks#signatures).
func signWebhookPayload(t time.Time, payload []byte, secret string) string {
	sig := webhook.ComputeSignature(t, payload, secret)
	return fmt.Sprintf("t=%d,v1=%x", t.Unix(), sig)
}

// TestStripeClientConstructWebhookEventEmptySecret proves that
// ConstructWebhookEvent refuses to verify against an empty configured
// webhook secret, returning domain.ErrInvalidWebhookSignature without
// calling into webhook.ConstructEvent (an empty secret is a fixed,
// publicly-known HMAC key, not "skip verification" — see
// NewStripeClient's GoDoc).
func TestStripeClientConstructWebhookEventEmptySecret(t *testing.T) {
	client := NewStripeClient("sk_test_dummy", "")

	payload := []byte(`{"id":"evt_1","object":"event","type":"customer.created"}`)
	// A validly-computed signature under a guessed/known empty secret still
	// must not be accepted, since the guard runs before any signature check.
	sigHeader := signWebhookPayload(time.Now(), payload, "")

	_, err := client.ConstructWebhookEvent(payload, sigHeader)
	if !errors.Is(err, domain.ErrInvalidWebhookSignature) {
		t.Fatalf("expected domain.ErrInvalidWebhookSignature, got %v", err)
	}
}

// TestStripeClientConstructWebhookEventValidSignature proves that, given a
// configured webhookSecret, ConstructWebhookEvent accepts a payload signed
// with that same secret and maps it into a billing.WebhookEvent carrying
// the event's ID and Type.
func TestStripeClientConstructWebhookEventValidSignature(t *testing.T) {
	const secret = "whsec_test_secret" // #nosec G101 -- test fixture, not a real credential
	client := NewStripeClient("sk_test_dummy", secret)

	payload := []byte(fmt.Sprintf(
		`{"id":"evt_test_123","object":"event","api_version":%q,"created":1700000000,"data":{"object":{}},"type":"customer.created"}`,
		stripe.APIVersion,
	))
	sigHeader := signWebhookPayload(time.Now(), payload, secret)

	event, err := client.ConstructWebhookEvent(payload, sigHeader)
	if err != nil {
		t.Fatalf("ConstructWebhookEvent failed: %v", err)
	}
	if event.ID != "evt_test_123" || event.Type != "customer.created" {
		t.Errorf("unexpected event: %+v", event)
	}
}

// TestStripeClientConstructWebhookEventWrongSecret proves that a payload
// signed with a different secret than the one configured on StripeClient is
// rejected as domain.ErrInvalidWebhookSignature.
func TestStripeClientConstructWebhookEventWrongSecret(t *testing.T) {
	client := NewStripeClient("sk_test_dummy", "whsec_correct")

	payload := []byte(fmt.Sprintf(
		`{"id":"evt_test_456","object":"event","api_version":%q,"created":1700000000,"data":{"object":{}},"type":"customer.created"}`,
		stripe.APIVersion,
	))
	sigHeader := signWebhookPayload(time.Now(), payload, "whsec_wrong")

	_, err := client.ConstructWebhookEvent(payload, sigHeader)
	if !errors.Is(err, domain.ErrInvalidWebhookSignature) {
		t.Fatalf("expected domain.ErrInvalidWebhookSignature, got %v", err)
	}
}
