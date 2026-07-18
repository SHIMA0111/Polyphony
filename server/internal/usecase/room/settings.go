package room

import (
	"context"
	"time"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// UpdateSettings sets a room's default AI provider/model
// (domainroom.Room.AIProvider / AIModel). AIModel is consulted by
// usecase/message.resolveModel as the room-level fallback tier whenever an
// AI request omits an explicit model, so this endpoint lets a room
// admin/master pin the room to a specific provider+model (e.g. all AI
// replies in a "support" room use "claude-opus-4") without every sender
// having to pass model on each request. AIProvider is persisted alongside
// AIModel for display/consistency only — it is not itself consulted by
// model resolution.
//
// The caller must be at least domainroom.RoleAdmin in the room
// (domainroom.ActionManageRoom) — the same rule UpdateRoom and
// UpdateAIContextCutoff already enforce, so both admin and master may
// configure room AI settings but reader/guest/member may not. It returns
// domain.ErrForbidden if the caller lacks that role or is not a member of
// the room, and domain.ErrNotFound if the room does not exist.
//
// aiProvider and aiModel each independently follow a "nil vs. empty-string
// sentinel" convention, since a plain *string cannot otherwise distinguish
// "field omitted from the request" from "field explicitly cleared" once
// JSON has been decoded:
//   - nil leaves the corresponding stored field unchanged.
//   - a pointer to "" (empty string) clears the stored field back to NULL
//     (domainroom.Room.AIProvider / AIModel = nil).
//   - a pointer to any other non-empty value sets the stored field to that
//     value.
//
// It follows the same get-check-mutate-persist shape as UpdateRoom and
// UpdateAIContextCutoff, returning a domainroom.RoomWithRole pairing the
// updated room with the caller's role so handlers can map the result to
// RoomResponse without special-casing this endpoint.
func (u *RoomUsecase) UpdateSettings(ctx context.Context, userID, roomID string, aiProvider, aiModel *string) (*domainroom.RoomWithRole, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if err := domainroom.Authorize(member.Role, domainroom.ActionManageRoom); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	rm.AIProvider = applySettingField(rm.AIProvider, aiProvider)
	rm.AIModel = applySettingField(rm.AIModel, aiModel)
	rm.UpdatedAt = time.Now()

	if err = u.roomRepo.UpdateAISettings(ctx, roomID, rm.AIProvider, rm.AIModel); err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: member.Role}, nil
}

// applySettingField applies UpdateSettings's nil/empty-string-sentinel/value
// convention to a single nullable string field: a nil update leaves current
// unchanged, a pointer to "" clears the field to nil, and any other pointer
// value replaces current with a copy of the pointed-to value.
func applySettingField(current *string, update *string) *string {
	if update == nil {
		return current
	}
	if *update == "" {
		return nil
	}
	v := *update
	return &v
}
