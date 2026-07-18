package room

import (
	"context"

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

	setProvider, providerValue := settingUpdate(aiProvider)
	setModel, modelValue := settingUpdate(aiModel)

	if err := u.roomRepo.UpdateAISettings(ctx, roomID, setProvider, providerValue, setModel, modelValue); err != nil {
		return nil, err
	}

	// Re-read the room after the atomic UPDATE (rather than merging into a
	// pre-update snapshot, as before) so the returned struct reflects the
	// room's true persisted state, including any field this call left
	// untouched. Using a pre-update snapshot to fill in the untouched
	// field(s) here would reintroduce exactly the lost-update race
	// UpdateAISettings's CASE WHEN UPDATE was built to avoid: this
	// GetByID's result is never written back, only returned to the caller.
	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: member.Role}, nil
}

// settingUpdate translates UpdateSettings's nil/empty-string-sentinel/value
// request convention for a single field into UpdateAISettings's
// set-flag/value pair: a nil update means "field omitted" (set=false, value
// ignored), a pointer to "" means "clear to NULL" (set=true, value=nil), and
// any other pointer value means "set to that value" (set=true, value=a copy
// of *update so the repository's persisted state can't later alias the
// caller's own pointer).
func settingUpdate(update *string) (set bool, value *string) {
	if update == nil {
		return false, nil
	}
	if *update == "" {
		return true, nil
	}
	v := *update
	return true, &v
}
