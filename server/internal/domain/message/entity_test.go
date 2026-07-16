package message

import "testing"

func TestMessageTypeConstants(t *testing.T) {
	if MessageTypeHuman != "human" {
		t.Fatalf("expected human, got %s", MessageTypeHuman)
	}
	if MessageTypeAI != "ai" {
		t.Fatalf("expected ai, got %s", MessageTypeAI)
	}
}

// TestMessageVisibilityConstants asserts the wire values of
// MessageVisibilityPublic/MessageVisibilityPrivate, following the same
// pattern as TestMessageTypeConstants.
func TestMessageVisibilityConstants(t *testing.T) {
	if MessageVisibilityPublic != "public" {
		t.Fatalf("expected public, got %s", MessageVisibilityPublic)
	}
	if MessageVisibilityPrivate != "private" {
		t.Fatalf("expected private, got %s", MessageVisibilityPrivate)
	}
}
