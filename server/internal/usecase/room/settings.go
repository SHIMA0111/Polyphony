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
// Unlike UpdateRoom and UpdateAIContextCutoff, this is not a plain
// get-check-mutate-persist: aiProvider and aiModel are passed straight
// through to roomRepo.UpdateAISettings, which applies their
// omitted/clear/set semantics itself in a single atomic UPDATE (see its
// GoDoc) rather than this usecase computing final column values from a
// GetByID snapshot and writing both columns back. That distinction matters
// specifically because the two fields can be updated independently: if this
// method instead read the room, merged in just the caller's field, and
// wrote both columns back (as it used to), two concurrent UpdateSettings
// calls each touching only one field (e.g. one setting only aiProvider, the
// other only aiModel) could lost-update each other whenever both calls'
// reads happened before either call's write -- whichever write landed last
// would silently clobber the other's field back to its own stale snapshot
// of it. Pushing the omitted/clear/set resolution down into the single
// UPDATE statement removes that read-then-write window entirely: an omitted
// field is never assigned a new value by the SQL itself, so a concurrent
// write to the other field cannot be overwritten.
//
// The returned domainroom.RoomWithRole pairs the caller's role with a Room
// reflecting this call's own view of the result: AIProvider/AIModel are
// computed by applying aiProvider/aiModel to a GetByID snapshot taken
// before the write, purely for the response, and may not match the row
// UpdateAISettings actually just persisted if a concurrent UpdateSettings
// call raced this one -- callers that need a guaranteed-fresh read should
// GetByID again.
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
	updatedAt := time.Now()

	if err = u.roomRepo.UpdateAISettings(ctx, roomID, aiProvider, aiModel, updatedAt); err != nil {
		return nil, err
	}

	rm.AIProvider = applySettingField(rm.AIProvider, aiProvider)
	rm.AIModel = applySettingField(rm.AIModel, aiModel)
	rm.UpdatedAt = updatedAt

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
