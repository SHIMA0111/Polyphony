package group

import (
	"context"
	"errors"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
)

// newTestFixture wires up a fresh GroupUsecase (backed by a real
// invitationusecase.InvitationUsecase, per Step 2's shared-mock convention)
// with in-memory mocks. It seeds "room-1" with an admin ("owner-1",
// RoleAdmin) and a plain member ("member-1", RoleMember), and registers
// three users: "owner-1" (the group owner), "bob" and "carol" (not yet room
// members). It returns the usecase and the backing mocks for direct
// manipulation/assertions.
func newTestFixture() (*GroupUsecase, *mocks.GroupRepo, *mocks.RoomRepo, *mocks.UserRepo, *mocks.InvitationRepo) {
	groupRepo := &mocks.GroupRepo{}
	roomRepo := &mocks.RoomRepo{}
	userRepo := &mocks.UserRepo{}
	invitationRepo := &mocks.InvitationRepo{}

	roomRepo.SeedMember("room-1", "owner-1", "admin")
	roomRepo.SeedMember("room-1", "member-1", "member")

	ctx := context.Background()
	_ = userRepo.Create(ctx, &domainuser.User{ID: "owner-1", Email: "owner@example.com", Username: "owner"})
	_ = userRepo.Create(ctx, &domainuser.User{ID: "bob-1", Email: "bob@example.com", Username: "bob"})
	_ = userRepo.Create(ctx, &domainuser.User{ID: "carol-1", Email: "carol@example.com", Username: "carol"})

	// GroupRepo.AddMember (unlike postgres.GroupRepository.AddMember, which
	// JOINs against the real users table on ListMembers) has no way to
	// resolve a userID to a username on its own, so tests must pre-populate
	// the fake's username lookup for every user that might be added as a
	// group member.
	groupRepo.Usernames = map[string]string{
		"owner-1": "owner",
		"bob-1":   "bob",
		"carol-1": "carol",
	}

	invitationUC := invitationusecase.NewInvitationUsecase(invitationRepo, roomRepo, userRepo)
	uc := NewGroupUsecase(groupRepo, userRepo, invitationUC)
	return uc, groupRepo, roomRepo, userRepo, invitationRepo
}

func TestCreateAndGetGroup(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "my team")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if g.OwnerID != "owner-1" || g.Name != "Team" {
		t.Fatalf("unexpected group: %+v", g)
	}

	got, err := uc.GetGroup(ctx, "owner-1", g.ID)
	if err != nil {
		t.Fatalf("GetGroup failed: %v", err)
	}
	if got.ID != g.ID {
		t.Fatalf("expected group %s, got %s", g.ID, got.ID)
	}
}

func TestGetGroupForbiddenForNonOwner(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if _, err := uc.GetGroup(ctx, "bob-1", g.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestListGroups(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	if _, err := uc.CreateGroup(ctx, "owner-1", "Team A", ""); err != nil {
		t.Fatalf("CreateGroup A failed: %v", err)
	}
	if _, err := uc.CreateGroup(ctx, "owner-1", "Team B", ""); err != nil {
		t.Fatalf("CreateGroup B failed: %v", err)
	}
	if _, err := uc.CreateGroup(ctx, "bob-1", "Bob's Team", ""); err != nil {
		t.Fatalf("CreateGroup (bob) failed: %v", err)
	}

	groups, err := uc.ListGroups(ctx, "owner-1")
	if err != nil {
		t.Fatalf("ListGroups failed: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups owned by owner-1, got %d", len(groups))
	}
}

func TestUpdateGroup(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	updated, err := uc.UpdateGroup(ctx, "owner-1", g.ID, "Renamed", "new description")
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}
	if updated.Name != "Renamed" || updated.Description != "new description" {
		t.Fatalf("unexpected updated group: %+v", updated)
	}

	if _, err := uc.UpdateGroup(ctx, "bob-1", g.ID, "Hijacked", ""); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-owner update, got %v", err)
	}
}

func TestDeleteGroup(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if err := uc.DeleteGroup(ctx, "bob-1", g.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-owner delete, got %v", err)
	}

	if err := uc.DeleteGroup(ctx, "owner-1", g.ID); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	if _, err := uc.GetGroup(ctx, "owner-1", g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestAddMemberSuccess(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	member, err := uc.AddMember(ctx, "owner-1", g.ID, "bob")
	if err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	if member.UserID != "bob-1" || member.Username != "bob" {
		t.Fatalf("unexpected member: %+v", member)
	}
}

func TestAddMemberUnknownUsername(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "nonexistent"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown username, got %v", err)
	}
}

func TestAddMemberDuplicate(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember (first) failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); !errors.Is(err, domain.ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember on duplicate add, got %v", err)
	}
}

func TestListMembersResolvesUsernames(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember bob failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "carol"); err != nil {
		t.Fatalf("AddMember carol failed: %v", err)
	}

	members, err := uc.ListMembers(ctx, "owner-1", g.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	usernames := map[string]bool{}
	for _, m := range members {
		usernames[m.Username] = true
	}
	if !usernames["bob"] || !usernames["carol"] {
		t.Fatalf("expected bob and carol usernames resolved, got %+v", members)
	}

	if _, err := uc.ListMembers(ctx, "bob-1", g.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-owner ListMembers, got %v", err)
	}
}

func TestRemoveMember(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := uc.RemoveMember(ctx, "bob-1", g.ID, "bob-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-owner RemoveMember, got %v", err)
	}

	if err := uc.RemoveMember(ctx, "owner-1", g.ID, "bob-1"); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	members, err := uc.ListMembers(ctx, "owner-1", g.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 members after removal, got %d", len(members))
	}
}

func TestBatchInviteToRoomAllInvited(t *testing.T) {
	uc, _, _, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember bob failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "carol"); err != nil {
		t.Fatalf("AddMember carol failed: %v", err)
	}

	result, err := uc.BatchInviteToRoom(ctx, "owner-1", "room-1", g.ID, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("BatchInviteToRoom failed: %v", err)
	}
	if len(result.Invited) != 2 {
		t.Fatalf("expected 2 invited, got %d: %+v", len(result.Invited), result)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("expected 0 skipped, got %d: %+v", len(result.Skipped), result)
	}
}

func TestBatchInviteToRoomMixedSuccessAndSkip(t *testing.T) {
	uc, _, roomRepo, _, _ := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember bob failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "carol"); err != nil {
		t.Fatalf("AddMember carol failed: %v", err)
	}

	// bob is already a room member, so his invitation should be skipped
	// with ErrAlreadyMember; carol should be invited successfully.
	roomRepo.SeedMember("room-1", "bob-1", "member")

	result, err := uc.BatchInviteToRoom(ctx, "owner-1", "room-1", g.ID, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("BatchInviteToRoom failed: %v", err)
	}
	if len(result.Invited) != 1 || result.Invited[0].InviteeID == nil || *result.Invited[0].InviteeID != "carol-1" {
		t.Fatalf("expected carol invited, got %+v", result.Invited)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].UserID != "bob-1" {
		t.Fatalf("expected bob skipped, got %+v", result.Skipped)
	}
	if result.Skipped[0].Reason != BatchInviteSkipReasonAlreadyMember {
		t.Fatalf("expected skip reason %q, got %q", BatchInviteSkipReasonAlreadyMember, result.Skipped[0].Reason)
	}
}

func TestBatchInviteToRoomDuplicatePendingInviteSkip(t *testing.T) {
	uc, _, _, _, invitationRepo := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember bob failed: %v", err)
	}

	// First batch invite succeeds.
	result, err := uc.BatchInviteToRoom(ctx, "owner-1", "room-1", g.ID, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("first BatchInviteToRoom failed: %v", err)
	}
	if len(result.Invited) != 1 {
		t.Fatalf("expected 1 invited on first call, got %d", len(result.Invited))
	}
	if len(invitationRepo.Invitations) != 1 {
		t.Fatalf("expected 1 invitation persisted, got %d", len(invitationRepo.Invitations))
	}

	// Second batch invite for the same group/room hits the existing
	// pending invitation and must be recorded as a skip, not a whole-batch
	// failure.
	result2, err := uc.BatchInviteToRoom(ctx, "owner-1", "room-1", g.ID, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("second BatchInviteToRoom failed: %v", err)
	}
	if len(result2.Invited) != 0 {
		t.Fatalf("expected 0 invited on second call, got %d", len(result2.Invited))
	}
	if len(result2.Skipped) != 1 || result2.Skipped[0].UserID != "bob-1" {
		t.Fatalf("expected bob skipped on second call, got %+v", result2.Skipped)
	}
	if result2.Skipped[0].Reason != BatchInviteSkipReasonInvitationAlreadyExists {
		t.Fatalf("expected skip reason %q, got %q", BatchInviteSkipReasonInvitationAlreadyExists, result2.Skipped[0].Reason)
	}
}

// TestBatchInviteToRoomAbortsOnUnrecognizedError asserts that a per-member
// CreateInvitation error other than domain.ErrAlreadyMember or
// domain.ErrInvitationAlreadyExists is not swallowed into a skip entry: it
// aborts the whole batch immediately, returning (nil, err), rather than
// continuing to the remaining members.
func TestBatchInviteToRoomAbortsOnUnrecognizedError(t *testing.T) {
	uc, groupRepo, _, _, invitationRepo := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	// dave is the group's only member but was never registered in userRepo,
	// so CreateInvitation's userRepo.GetByUsername("dave") lookup fails with
	// domain.ErrNotFound — an error BatchInviteToRoom does not recognize as
	// a skippable per-member outcome. SeedMember bypasses
	// GroupUsecase.AddMember's own userRepo lookup, which would otherwise
	// reject an unregistered username outright.
	groupRepo.SeedMember(g.ID, "dave-1", "dave")

	result, err := uc.BatchInviteToRoom(ctx, "owner-1", "room-1", g.ID, domainroom.RoleMember, nil)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound to abort the batch, got result=%+v err=%v", result, err)
	}
	if result != nil {
		t.Fatalf("expected a nil result on abort, got %+v", result)
	}
	if len(invitationRepo.Invitations) != 0 {
		t.Fatalf("expected zero invitations persisted when the batch aborts, got %d", len(invitationRepo.Invitations))
	}
}

func TestBatchInviteToRoomShortCircuitsOnRBACFailure(t *testing.T) {
	uc, _, _, _, invitationRepo := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember bob failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "carol"); err != nil {
		t.Fatalf("AddMember carol failed: %v", err)
	}

	// member-1 is only a plain RoleMember in room-1, not Admin+, so the
	// batch invite must fail immediately with ErrForbidden rather than
	// producing two per-member skips.
	if _, err := uc.BatchInviteToRoom(ctx, "member-1", "room-1", g.ID, domainroom.RoleMember, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(invitationRepo.Invitations) != 0 {
		t.Fatalf("expected zero invitations created on RBAC short-circuit, got %d", len(invitationRepo.Invitations))
	}
}

func TestBatchInviteToRoomForbiddenWhenCallerDoesNotOwnGroup(t *testing.T) {
	uc, _, _, _, invitationRepo := newTestFixture()
	ctx := context.Background()

	g, err := uc.CreateGroup(ctx, "owner-1", "Team", "")
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if _, err := uc.AddMember(ctx, "owner-1", g.ID, "bob"); err != nil {
		t.Fatalf("AddMember bob failed: %v", err)
	}

	if _, err := uc.BatchInviteToRoom(ctx, "bob-1", "room-1", g.ID, domainroom.RoleMember, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for a non-owned group, got %v", err)
	}
	if len(invitationRepo.Invitations) != 0 {
		t.Fatalf("expected zero CreateInvitation calls made, got %d invitations", len(invitationRepo.Invitations))
	}
}
