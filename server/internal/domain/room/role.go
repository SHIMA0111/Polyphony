package room

import "github.com/SHIMA0111/multi-user-ai/server/internal/domain"

// Role represents a user's authorization level within a single room. It is a
// closed, ordered 5-tier hierarchy — from least to most privileged:
//
//	reader < guest < member < admin < master
//
// Role has zero dependencies beyond the standard library so it can be
// imported from any layer (domain, usecase, interface) without creating an
// upward dependency from the domain layer.
type Role string

const (
	// RoleReader can view a room's messages but cannot send messages,
	// invoke AI, manage members, manage room settings, or delete the room.
	RoleReader Role = "reader"
	// RoleGuest can send messages in addition to everything RoleReader can
	// do, but cannot invoke AI or perform any admin/master-level action.
	RoleGuest Role = "guest"
	// RoleMember can send messages and invoke AI in addition to everything
	// RoleGuest can do, but cannot manage members, manage room settings, or
	// delete the room.
	RoleMember Role = "member"
	// RoleAdmin can manage members and room settings in addition to
	// everything RoleMember can do, but cannot delete the room.
	RoleAdmin Role = "admin"
	// RoleMaster has full control over the room, including deleting it, in
	// addition to everything RoleAdmin can do.
	RoleMaster Role = "master"
)

// roleRank assigns each valid Role an ordinal used by AtLeast to compare
// privilege levels. Higher values are strictly more privileged. This is the
// single source of truth for the reader < guest < member < admin < master
// ordering; IsValid and AtLeast are both defined in terms of it.
var roleRank = map[Role]int{
	RoleReader: 0,
	RoleGuest:  1,
	RoleMember: 2,
	RoleAdmin:  3,
	RoleMaster: 4,
}

// IsValid reports whether r is one of the five defined role values
// (RoleReader, RoleGuest, RoleMember, RoleAdmin, RoleMaster). Any other
// value — including the empty string — is invalid.
func (r Role) IsValid() bool {
	_, ok := roleRank[r]
	return ok
}

// AtLeast reports whether r's privilege rank is greater than or equal to
// min's, per the reader < guest < member < admin < master ordering. If
// either r or min is not one of the five defined role values, AtLeast
// returns false: an unrecognized role is never considered to meet or
// exceed any minimum, and no role is considered to meet an unrecognized
// minimum.
func (r Role) AtLeast(min Role) bool {
	rRank, ok := roleRank[r]
	if !ok {
		return false
	}
	minRank, ok := roleRank[min]
	if !ok {
		return false
	}
	return rRank >= minRank
}

// Action identifies a permission-gated operation that a room member may
// attempt. It is a closed enum; see Role.Allows for the exact capability
// matrix mapping each Role to the set of Actions it permits.
type Action string

const (
	// ActionSendMessage is sending a human chat message into the room.
	ActionSendMessage Action = "send_message"
	// ActionInvokeAI is triggering an AI completion (SendAIMessage or
	// RegenerateAIMessage).
	ActionInvokeAI Action = "invoke_ai"
	// ActionManageMembers is adding, removing, or changing the role of
	// other room members.
	ActionManageMembers Action = "manage_members"
	// ActionManageRoom is updating room settings such as name and
	// description.
	ActionManageRoom Action = "manage_room"
	// ActionDeleteRoom is permanently deleting the room.
	ActionDeleteRoom Action = "delete_room"
)

// Allows reports whether r is permitted to perform action, per the
// following capability matrix:
//
//	                  reader  guest  member  admin  master
//	ActionSendMessage   no     yes    yes     yes    yes
//	ActionInvokeAI      no     no     yes     yes    yes
//	ActionManageMembers no     no     no      yes    yes
//	ActionManageRoom    no     no     no      yes    yes
//	ActionDeleteRoom    no     no     no      no     yes
//
// In words: reader is read-only and is allowed none of these actions.
// Guest may send messages but may not invoke AI or perform any
// admin/master-level action. Member may send messages and invoke AI, but
// may not manage members, manage room settings, or delete the room. Admin
// may do everything member can plus manage members and room settings, but
// may not delete the room. Master may do everything, including deleting
// the room. An action value outside the closed Action enum, or a Role that
// fails IsValid, is never allowed.
func (r Role) Allows(action Action) bool {
	switch action {
	case ActionSendMessage:
		return r.AtLeast(RoleGuest)
	case ActionInvokeAI:
		return r.AtLeast(RoleMember)
	case ActionManageMembers, ActionManageRoom:
		return r.AtLeast(RoleAdmin)
	case ActionDeleteRoom:
		return r.AtLeast(RoleMaster)
	default:
		return false
	}
}

// Authorize checks whether role is permitted to perform action (per
// Role.Allows's capability matrix) and returns domain.ErrForbidden if not,
// or nil if it is.
//
// This lives in the domain layer (rather than interface/middleware, where
// it used to live) specifically so usecase-layer callers that already hold
// a loaded RoomMember (e.g. after their own membership lookup) can call it
// without importing the interface layer — a usecase importing
// interface/middleware is an upward dependency that violates this project's
// Clean Architecture layering (see CLAUDE.md). interface/middleware.
// RequireRole calls this same function for its route-level check, so the
// authorization rule is still defined in exactly one place; it does not
// force a second repository round-trip for callers that already have the
// member's role in hand.
func Authorize(role Role, action Action) error {
	if !role.Allows(action) {
		return domain.ErrForbidden
	}
	return nil
}
