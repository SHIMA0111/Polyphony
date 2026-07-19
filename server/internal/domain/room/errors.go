package room

import "errors"

// ErrOwnerRoleProtected is returned in two situations that both stem from
// the invariant that a room's current owner (rooms.owner_id) must always
// hold RoleMaster and can only stop being owner via an explicit ownership
// transfer:
//
//  1. The current owner tries to leave the room (RoomUsecase.LeaveRoom)
//     without first transferring ownership to another member.
//  2. Someone tries to change the current owner's role directly
//     (RoomUsecase.ChangeMemberRole) instead of going through
//     RoomUsecase.TransferOwnership.
//
// It carries no external dependencies (only the standard library), so it
// can be used from any layer without creating an upward dependency from
// the domain layer. Only the handler layer translates it into an HTTP
// status code (409 Conflict).
var ErrOwnerRoleProtected = errors.New("room owner role is protected: transfer ownership first")
