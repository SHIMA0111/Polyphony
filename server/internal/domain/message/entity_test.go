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
