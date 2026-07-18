package room

import "errors"

// ErrOwnerRoleProtected is returned in three situations that all stem from
// the invariant that a room's current owner (rooms.owner_id) must always
// hold RoleMaster, exactly one member holds RoleMaster at a time, and that
// role can only change via an explicit ownership transfer:
//
//  1. The current owner tries to leave the room (RoomUsecase.LeaveRoom)
//     without first transferring ownership to another member.
//  2. Someone tries to change the current owner's role directly
//     (RoomUsecase.ChangeMemberRole) instead of going through
//     RoomUsecase.TransferOwnership.
//  3. Someone tries to grant RoleMaster to a non-owner via
//     RoomUsecase.ChangeMemberRole, which would leave the room with two
//     masters (the existing owner plus the newly-promoted target) instead
//     of transferring ownership away from the current one. The handler
//     layer rejects this with HTTP 400 before ever reaching the usecase;
//     ChangeMemberRole re-checks it as defense in depth.
//
// It carries no external dependencies (only the standard library), so it
// can be used from any layer without creating an upward dependency from
// the domain layer. Only the handler layer translates it into an HTTP
// status code (409 Conflict).
var ErrOwnerRoleProtected = errors.New("room owner role is protected: transfer ownership first")
