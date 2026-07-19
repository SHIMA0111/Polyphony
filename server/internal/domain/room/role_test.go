package room

import (
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
)

// allRoles lists the five defined roles in ascending privilege order,
// matching roleRank.
var allRoles = []Role{RoleReader, RoleGuest, RoleMember, RoleAdmin, RoleMaster}

// TestRoleIsValid asserts that exactly the five defined role values are
// valid and that arbitrary/empty strings are not.
func TestRoleIsValid(t *testing.T) {
	for _, r := range allRoles {
		if !r.IsValid() {
			t.Errorf("%q should be valid", r)
		}
	}

	invalid := []Role{"", "owner", "SuperAdmin", "Master", " master"}
	for _, r := range invalid {
		if r.IsValid() {
			t.Errorf("%q should not be valid", r)
		}
	}
}

// TestRoleAtLeast exhaustively checks all 5x5 role pairs against the
// reader < guest < member < admin < master ordering.
func TestRoleAtLeast(t *testing.T) {
	rank := map[Role]int{
		RoleReader: 0,
		RoleGuest:  1,
		RoleMember: 2,
		RoleAdmin:  3,
		RoleMaster: 4,
	}

	for _, r := range allRoles {
		for _, min := range allRoles {
			want := rank[r] >= rank[min]
			if got := r.AtLeast(min); got != want {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", r, min, got, want)
			}
		}
	}
}

// TestRoleAtLeastInvalid asserts that an unrecognized role never satisfies
// AtLeast (as either the receiver or the minimum argument).
func TestRoleAtLeastInvalid(t *testing.T) {
	if Role("bogus").AtLeast(RoleReader) {
		t.Error("an invalid role should never be AtLeast any valid role")
	}
	if RoleMaster.AtLeast(Role("bogus")) {
		t.Error("no role should be AtLeast an invalid minimum")
	}
	if Role("bogus").AtLeast(Role("bogus")) {
		t.Error("two invalid roles should not compare as AtLeast")
	}
}

// TestRoleAllows exhaustively checks all 5 roles x 5 actions (25
// combinations) against the documented capability matrix on Role.Allows.
func TestRoleAllows(t *testing.T) {
	tests := []struct {
		role   Role
		action Action
		want   bool
	}{
		// reader: read-only, allows nothing.
		{RoleReader, ActionSendMessage, false},
		{RoleReader, ActionInvokeAI, false},
		{RoleReader, ActionManageMembers, false},
		{RoleReader, ActionManageRoom, false},
		{RoleReader, ActionDeleteRoom, false},

		// guest: send messages only.
		{RoleGuest, ActionSendMessage, true},
		{RoleGuest, ActionInvokeAI, false},
		{RoleGuest, ActionManageMembers, false},
		{RoleGuest, ActionManageRoom, false},
		{RoleGuest, ActionDeleteRoom, false},

		// member: send messages + invoke AI.
		{RoleMember, ActionSendMessage, true},
		{RoleMember, ActionInvokeAI, true},
		{RoleMember, ActionManageMembers, false},
		{RoleMember, ActionManageRoom, false},
		{RoleMember, ActionDeleteRoom, false},

		// admin: everything except delete room.
		{RoleAdmin, ActionSendMessage, true},
		{RoleAdmin, ActionInvokeAI, true},
		{RoleAdmin, ActionManageMembers, true},
		{RoleAdmin, ActionManageRoom, true},
		{RoleAdmin, ActionDeleteRoom, false},

		// master: full control.
		{RoleMaster, ActionSendMessage, true},
		{RoleMaster, ActionInvokeAI, true},
		{RoleMaster, ActionManageMembers, true},
		{RoleMaster, ActionManageRoom, true},
		{RoleMaster, ActionDeleteRoom, true},
	}

	if len(tests) != len(allRoles)*5 {
		t.Fatalf("expected %d role x action cases, got %d", len(allRoles)*5, len(tests))
	}

	for _, tt := range tests {
		if got := tt.role.Allows(tt.action); got != tt.want {
			t.Errorf("%s.Allows(%s) = %v, want %v", tt.role, tt.action, got, tt.want)
		}
	}
}

// TestRoleAllowsUnknownAction asserts that an Action value outside the
// closed enum, and an invalid Role, are never allowed.
func TestRoleAllowsUnknownAction(t *testing.T) {
	if RoleMaster.Allows(Action("teleport")) {
		t.Error("an unrecognized Action should never be allowed, even for master")
	}
	if Role("bogus").Allows(ActionSendMessage) {
		t.Error("an invalid role should never be allowed to perform any action")
	}
}

// TestAuthorize proves Authorize maps Role.Allows to domain.ErrForbidden
// (moved here from interface/middleware, which used to own this function —
// see Authorize's doc comment for why it now lives in the domain layer).
func TestAuthorize(t *testing.T) {
	if err := Authorize(RoleMember, ActionInvokeAI); err != nil {
		t.Fatalf("expected member to be authorized to invoke AI, got %v", err)
	}
	if err := Authorize(RoleGuest, ActionInvokeAI); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected guest to be forbidden from invoking AI with domain.ErrForbidden, got %v", err)
	}
}
