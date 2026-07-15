package room

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// strPtr returns a pointer to v, for building *string test fixtures inline.
func strPtr(v string) *string { return &v }

// --- UpdateSettings (Step 24: per-room AI provider/model settings) ---

func TestUpdateSettingsAdminCanSet(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	})

	updated, err := uc.UpdateSettings(ctx, "user-2", rwr.Room.ID, strPtr("anthropic"), strPtr("claude-opus-4"))
	if err != nil {
		t.Fatalf("expected admin to update settings, got error: %v", err)
	}
	if updated.Room.AIProvider == nil || *updated.Room.AIProvider != "anthropic" {
		t.Fatalf("expected ai_provider anthropic, got %v", updated.Room.AIProvider)
	}
	if updated.Room.AIModel == nil || *updated.Room.AIModel != "claude-opus-4" {
		t.Fatalf("expected ai_model claude-opus-4, got %v", updated.Room.AIModel)
	}
}

func TestUpdateSettingsMasterCanSet(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	updated, err := uc.UpdateSettings(ctx, "user-1", rwr.Room.ID, nil, strPtr("gpt-5-mini"))
	if err != nil {
		t.Fatalf("expected master to update settings, got error: %v", err)
	}
	if updated.Room.AIModel == nil || *updated.Room.AIModel != "gpt-5-mini" {
		t.Fatalf("expected ai_model gpt-5-mini, got %v", updated.Room.AIModel)
	}
}

func TestUpdateSettingsMemberForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
	})

	if _, err := uc.UpdateSettings(ctx, "user-2", rwr.Room.ID, nil, strPtr("claude-opus-4")); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for member, got %v", err)
	}
}

func TestUpdateSettingsGuestForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleGuest,
	})

	if _, err := uc.UpdateSettings(ctx, "user-2", rwr.Room.ID, nil, strPtr("claude-opus-4")); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for guest, got %v", err)
	}
}

func TestUpdateSettingsReaderForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleReader,
	})

	if _, err := uc.UpdateSettings(ctx, "user-2", rwr.Room.ID, nil, strPtr("claude-opus-4")); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for reader, got %v", err)
	}
}

func TestUpdateSettingsNonMemberForbidden(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	if _, err := uc.UpdateSettings(ctx, "user-2", rwr.Room.ID, nil, strPtr("claude-opus-4")); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden for non-member, got %v", err)
	}
}

// TestUpdateSettingsNilLeavesFieldUnchanged asserts UpdateSettings's nil
// convention: a nil aiProvider/aiModel argument leaves the corresponding
// stored field exactly as it was, rather than clearing it.
func TestUpdateSettingsNilLeavesFieldUnchanged(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	if _, err := uc.UpdateSettings(ctx, "user-1", rwr.Room.ID, strPtr("anthropic"), strPtr("claude-opus-4")); err != nil {
		t.Fatalf("initial UpdateSettings failed: %v", err)
	}

	// Second call updates only ai_model (nil ai_provider); ai_provider must
	// remain "anthropic".
	updated, err := uc.UpdateSettings(ctx, "user-1", rwr.Room.ID, nil, strPtr("claude-sonnet-4"))
	if err != nil {
		t.Fatalf("second UpdateSettings failed: %v", err)
	}
	if updated.Room.AIProvider == nil || *updated.Room.AIProvider != "anthropic" {
		t.Fatalf("expected ai_provider to remain anthropic, got %v", updated.Room.AIProvider)
	}
	if updated.Room.AIModel == nil || *updated.Room.AIModel != "claude-sonnet-4" {
		t.Fatalf("expected ai_model claude-sonnet-4, got %v", updated.Room.AIModel)
	}
}

// TestUpdateSettingsEmptyStringClearsField asserts UpdateSettings's
// empty-string-sentinel convention: a pointer to "" clears the stored field
// back to NULL.
func TestUpdateSettingsEmptyStringClearsField(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rwr, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	if _, err := uc.UpdateSettings(ctx, "user-1", rwr.Room.ID, strPtr("anthropic"), strPtr("claude-opus-4")); err != nil {
		t.Fatalf("initial UpdateSettings failed: %v", err)
	}

	cleared, err := uc.UpdateSettings(ctx, "user-1", rwr.Room.ID, strPtr(""), strPtr(""))
	if err != nil {
		t.Fatalf("clearing UpdateSettings failed: %v", err)
	}
	if cleared.Room.AIProvider != nil {
		t.Fatalf("expected ai_provider cleared to nil, got %v", cleared.Room.AIProvider)
	}
	if cleared.Room.AIModel != nil {
		t.Fatalf("expected ai_model cleared to nil, got %v", cleared.Room.AIModel)
	}
}
