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
// The room is re-read via GetByID after UpdateAISettings's write completes,
// rather than merged into a pre-write snapshot, so the returned
// domainroom.RoomWithRole reflects the room's true persisted state --
// including any field this call left untouched -- instead of a value this
// usecase computed itself from a snapshot taken before the write. Filling in
// the untouched field(s) from a pre-write snapshot here would reintroduce
// exactly the lost-update race UpdateAISettings's single-UPDATE design was
// built to avoid (see above): if a concurrent UpdateSettings call touching
// only the other field persisted between this call's snapshot read and its
// own write, returning a value derived from that stale snapshot would
// misreport the room's state back to this caller even though the database
// row itself is correct.
func (u *RoomUsecase) UpdateSettings(ctx context.Context, userID, roomID string, aiProvider, aiModel *string) (*domainroom.RoomWithRole, error) {
	member, err := u.getMember(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if err := domainroom.Authorize(member.Role, domainroom.ActionManageRoom); err != nil {
		return nil, err
	}

	if err := u.roomRepo.UpdateAISettings(ctx, roomID, aiProvider, aiModel, time.Now()); err != nil {
		return nil, err
	}

	rm, err := u.roomRepo.GetByID(ctx, roomID)
	if err != nil {
		return nil, err
	}

	return &domainroom.RoomWithRole{Room: rm, Role: member.Role}, nil
}
