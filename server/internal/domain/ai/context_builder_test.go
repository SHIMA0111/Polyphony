package ai

import (
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/message"
)

// msg is a small test-only constructor for a fixture message.Message, since
// the fields under test span many structs.
func msg(id string, msgType message.MessageType, status message.MessageStatus, content string, createdAt time.Time, isDeleted, excludeFromAI bool) *message.Message {
	return &message.Message{
		ID:            id,
		Type:          msgType,
		Status:        status,
		Content:       content,
		CreatedAt:     createdAt,
		IsDeleted:     isDeleted,
		ExcludeFromAI: excludeFromAI,
	}
}

func TestDefaultContextBuilderBuild(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		msgs     []*message.Message // sequence-descending (newest first), as ListByRoom returns
		cutoff   *time.Time
		wantRole []string
		wantText []string
	}{
		{
			name: "excludes soft-deleted message",
			msgs: []*message.Message{
				msg("2", message.MessageTypeAI, message.MessageStatusCompleted, "reply", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "deleted content", base.Add(time.Hour), true, false),
			},
			wantRole: []string{"assistant"},
			wantText: []string{"reply"},
		},
		{
			name: "excludes exclude_from_ai message",
			msgs: []*message.Message{
				msg("2", message.MessageTypeAI, message.MessageStatusCompleted, "reply", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "excluded content", base.Add(time.Hour), false, true),
			},
			wantRole: []string{"assistant"},
			wantText: []string{"reply"},
		},
		{
			name: "excludes status=failed message",
			msgs: []*message.Message{
				msg("2", message.MessageTypeAI, message.MessageStatusFailed, "", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "question", base.Add(time.Hour), false, false),
			},
			wantRole: []string{"user"},
			wantText: []string{"question"},
		},
		{
			name: "excludes message before non-nil cutoff",
			msgs: []*message.Message{
				msg("2", message.MessageTypeHuman, message.MessageStatusCompleted, "after cutoff", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "before cutoff", base.Add(time.Hour), false, false),
			},
			cutoff:   timePtr(base.Add(90 * time.Minute)),
			wantRole: []string{"user"},
			wantText: []string{"after cutoff"},
		},
		{
			name: "nil cutoff excludes nothing on cutoff grounds",
			msgs: []*message.Message{
				msg("2", message.MessageTypeHuman, message.MessageStatusCompleted, "second", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "first", base.Add(time.Hour), false, false),
			},
			cutoff:   nil,
			wantRole: []string{"user", "user"},
			wantText: []string{"first", "second"},
		},
		{
			name: "output is chronological given sequence-descending input",
			msgs: []*message.Message{
				msg("3", message.MessageTypeAI, message.MessageStatusCompleted, "third", base.Add(3*time.Hour), false, false),
				msg("2", message.MessageTypeHuman, message.MessageStatusCompleted, "second", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "first", base.Add(time.Hour), false, false),
			},
			wantRole: []string{"user", "user", "assistant"},
			wantText: []string{"first", "second", "third"},
		},
		{
			name: "role mapping: human -> user, ai -> assistant",
			msgs: []*message.Message{
				msg("2", message.MessageTypeAI, message.MessageStatusCompleted, "ai reply", base.Add(2*time.Hour), false, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "human question", base.Add(time.Hour), false, false),
			},
			wantRole: []string{"user", "assistant"},
			wantText: []string{"human question", "ai reply"},
		},
		{
			name: "combination: soft-deleted, excluded, failed, and pre-cutoff all filtered together",
			msgs: []*message.Message{
				msg("5", message.MessageTypeAI, message.MessageStatusCompleted, "final reply", base.Add(5*time.Hour), false, false),
				msg("4", message.MessageTypeAI, message.MessageStatusFailed, "", base.Add(4*time.Hour), false, false),
				msg("3", message.MessageTypeHuman, message.MessageStatusCompleted, "excluded", base.Add(3*time.Hour), false, true),
				msg("2", message.MessageTypeHuman, message.MessageStatusCompleted, "deleted", base.Add(2*time.Hour), true, false),
				msg("1", message.MessageTypeHuman, message.MessageStatusCompleted, "before cutoff", base.Add(time.Hour), false, false),
			},
			cutoff:   timePtr(base.Add(90 * time.Minute)),
			wantRole: []string{"assistant"},
			wantText: []string{"final reply"},
		},
	}

	b := NewDefaultContextBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.Build(tt.msgs, tt.cutoff)
			if len(got) != len(tt.wantText) {
				t.Fatalf("expected %d messages, got %d: %+v", len(tt.wantText), len(got), got)
			}
			for i, want := range tt.wantText {
				if got[i].Content != want {
					t.Errorf("index %d: expected content %q, got %q", i, want, got[i].Content)
				}
				if got[i].Role != tt.wantRole[i] {
					t.Errorf("index %d: expected role %q, got %q", i, tt.wantRole[i], got[i].Role)
				}
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
