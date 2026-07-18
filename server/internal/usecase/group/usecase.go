// Package group implements the business logic for personal groups: creating,
// listing, updating, and deleting groups an authenticated user owns, and
// managing each group's membership (add/remove by username, list with
// usernames resolved). It also provides BatchInviteToRoom, which fans a
// single batch request out into repeated calls to the existing
// server/internal/usecase/invitation.InvitationUsecase.CreateInvitation,
// one per group member, collecting per-member results instead of failing
// the whole batch on the first duplicate or already-a-member case.
package group

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domaingroup "github.com/SHIMA0111/multi-user-ai/server/internal/domain/group"
	domaininvitation "github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
)

// Stable, client-facing reason strings recorded on a BatchInviteSkip. These
// are the only two per-member errors CreateInvitation can return that
// BatchInviteToRoom treats as skippable (see BatchInviteToRoom's doc
// comment); any other error fails the whole batch instead of producing a
// skip, so no other reason string is ever emitted.
const (
	// BatchInviteReasonAlreadyMember is recorded when the invitee is
	// already a member of the target room.
	BatchInviteReasonAlreadyMember = "already_member"
	// BatchInviteReasonInvitationAlreadyExists is recorded when a pending
	// invitation already exists for the (room, invitee) pair.
	BatchInviteReasonInvitationAlreadyExists = "invitation_already_exists"
)

// BatchInviteSkip records why one group member was not invited by a
// BatchInviteToRoom call: they were already a room member, or already had a
// pending invitation. Reason is always one of the BatchInviteReason*
// constants.
type BatchInviteSkip struct {
	UserID   string
	Username string
	Reason   string
}

// BatchInviteResult is the outcome of a GroupUsecase.BatchInviteToRoom
// call: every invitation successfully created, plus a skip entry (with a
// reason) for every group member who was not invited.
type BatchInviteResult struct {
	Invited []*domaininvitation.Invitation
	Skipped []BatchInviteSkip
}

// GroupUsecase provides group-related business logic: creating, listing,
// reading, updating, and deleting personal groups, managing group
// membership, and batch-inviting a group's members into a room.
type GroupUsecase struct {
	groupRepo    domaingroup.GroupRepository
	userRepo     domainuser.UserRepository
	invitationUC *invitationusecase.InvitationUsecase
}

// NewGroupUsecase creates a new GroupUsecase. invitationUC is the
// already-constructed InvitationUsecase reused as-is by BatchInviteToRoom;
// its CreateInvitation method (RBAC checks, privilege-escalation guard, and
// duplicate-pending-invite detection) is never reimplemented here.
func NewGroupUsecase(
	groupRepo domaingroup.GroupRepository,
	userRepo domainuser.UserRepository,
	invitationUC *invitationusecase.InvitationUsecase,
) *GroupUsecase {
	return &GroupUsecase{
		groupRepo:    groupRepo,
		userRepo:     userRepo,
		invitationUC: invitationUC,
	}
}

// CreateGroup creates a new personal group owned by ownerID.
func (u *GroupUsecase) CreateGroup(ctx context.Context, ownerID, name, description string) (*domaingroup.Group, error) {
	now := time.Now()
	g := &domaingroup.Group{
		ID:          uuid.New().String(),
		OwnerID:     ownerID,
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := u.groupRepo.Create(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// GetGroup retrieves a group by ID. Only the group's owner may retrieve it;
// it returns domain.ErrForbidden if callerID does not own groupID, and
// domain.ErrNotFound if groupID does not exist.
func (u *GroupUsecase) GetGroup(ctx context.Context, callerID, groupID string) (*domaingroup.Group, error) {
	return u.getOwnedGroup(ctx, callerID, groupID)
}

// ListGroups returns all groups owned by ownerID.
func (u *GroupUsecase) ListGroups(ctx context.Context, ownerID string) ([]*domaingroup.Group, error) {
	return u.groupRepo.ListByOwnerID(ctx, ownerID)
}

// UpdateGroup updates a group's name and description. Only the group's
// owner may update it; it returns domain.ErrForbidden otherwise, and
// domain.ErrNotFound if groupID does not exist.
func (u *GroupUsecase) UpdateGroup(ctx context.Context, callerID, groupID, name, description string) (*domaingroup.Group, error) {
	g, err := u.getOwnedGroup(ctx, callerID, groupID)
	if err != nil {
		return nil, err
	}

	g.Name = name
	g.Description = description
	g.UpdatedAt = time.Now()

	if err := u.groupRepo.Update(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

// DeleteGroup deletes a group, cascading to its group_members rows via ON
// DELETE CASCADE. Only the group's owner may delete it; it returns
// domain.ErrForbidden otherwise, and domain.ErrNotFound if groupID does not
// exist.
func (u *GroupUsecase) DeleteGroup(ctx context.Context, callerID, groupID string) error {
	if _, err := u.getOwnedGroup(ctx, callerID, groupID); err != nil {
		return err
	}
	return u.groupRepo.Delete(ctx, groupID)
}

// AddMember adds the user identified by username to the group. Only the
// group's owner may add a member; it returns domain.ErrForbidden otherwise,
// domain.ErrNotFound if groupID does not exist or username does not match
// any user, and domain.ErrAlreadyMember if the user is already a member of
// the group.
func (u *GroupUsecase) AddMember(ctx context.Context, callerID, groupID, username string) (*domaingroup.GroupMemberWithUsername, error) {
	if _, err := u.getOwnedGroup(ctx, callerID, groupID); err != nil {
		return nil, err
	}

	target, err := u.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if _, err := u.groupRepo.GetMember(ctx, groupID, target.ID); err == nil {
		return nil, domain.ErrAlreadyMember
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	member := &domaingroup.GroupMember{
		ID:      uuid.New().String(),
		GroupID: groupID,
		UserID:  target.ID,
		AddedAt: time.Now(),
	}
	if err := u.groupRepo.AddMember(ctx, member); err != nil {
		return nil, err
	}

	return &domaingroup.GroupMemberWithUsername{GroupMember: *member, Username: target.Username}, nil
}

// ListMembers returns all members of a group, with usernames resolved.
// Only the group's owner may list its members; it returns
// domain.ErrForbidden otherwise, and domain.ErrNotFound if groupID does not
// exist.
func (u *GroupUsecase) ListMembers(ctx context.Context, callerID, groupID string) ([]*domaingroup.GroupMemberWithUsername, error) {
	if _, err := u.getOwnedGroup(ctx, callerID, groupID); err != nil {
		return nil, err
	}
	return u.groupRepo.ListMembers(ctx, groupID)
}

// RemoveMember removes a user from a group. Only the group's owner may
// remove a member; it returns domain.ErrForbidden otherwise, and
// domain.ErrNotFound if groupID does not exist or userID is not a member of
// the group.
func (u *GroupUsecase) RemoveMember(ctx context.Context, callerID, groupID, userID string) error {
	if _, err := u.getOwnedGroup(ctx, callerID, groupID); err != nil {
		return err
	}
	return u.groupRepo.RemoveMember(ctx, groupID, userID)
}

// BatchInviteToRoom invites every member of groupID into roomID with the
// given role, by calling the existing
// invitationusecase.InvitationUsecase.CreateInvitation once per member —
// exactly as a human operator would call the single-invite endpoint
// repeatedly. It does not reimplement CreateInvitation's RBAC check,
// privilege-escalation guard, or duplicate-pending-invite detection.
//
// callerID must own groupID; it returns domain.ErrForbidden (with zero
// CreateInvitation calls made) otherwise, and domain.ErrNotFound if
// groupID does not exist.
//
// If the very first CreateInvitation call returns domain.ErrForbidden
// (callerID lacks domainroom.RoleAdmin in roomID — the same RBAC check
// applies identically to every member since it depends only on
// callerID/roomID, not on the invitee), BatchInviteToRoom returns that
// error immediately without iterating further. Of the remaining per-member
// errors, only domain.ErrAlreadyMember and domain.ErrInvitationAlreadyExists
// are recorded as a skip (with the matching BatchInviteReason* constant as
// the reason) and let iteration continue to the next member; any other
// error is not a known skippable case and fails the whole batch
// immediately (nil, err), rather than risk silently swallowing an
// unrecognized failure.
func (u *GroupUsecase) BatchInviteToRoom(
	ctx context.Context,
	callerID, roomID, groupID string,
	role domainroom.Role,
	expiresInHours *int,
) (*BatchInviteResult, error) {
	if _, err := u.getOwnedGroup(ctx, callerID, groupID); err != nil {
		return nil, err
	}

	members, err := u.groupRepo.ListMembers(ctx, groupID)
	if err != nil {
		return nil, err
	}

	result := &BatchInviteResult{}
	for i, member := range members {
		username := member.Username
		inv, err := u.invitationUC.CreateInvitation(ctx, callerID, roomID, &username, role, expiresInHours)
		if err != nil {
			// The RBAC check (caller must be Admin+ in roomID) depends only
			// on callerID/roomID, so if it fails for the first member it
			// fails identically for every member: short-circuit rather than
			// recording N redundant per-member skips.
			if i == 0 && errors.Is(err, domain.ErrForbidden) {
				return nil, err
			}
			var reason string
			switch {
			case errors.Is(err, domain.ErrAlreadyMember):
				reason = BatchInviteReasonAlreadyMember
			case errors.Is(err, domain.ErrInvitationAlreadyExists):
				reason = BatchInviteReasonInvitationAlreadyExists
			default:
				// Not a known skippable case: fail the whole batch rather
				// than silently swallowing an unrecognized error.
				return nil, err
			}
			result.Skipped = append(result.Skipped, BatchInviteSkip{
				UserID:   member.UserID,
				Username: member.Username,
				Reason:   reason,
			})
			continue
		}
		result.Invited = append(result.Invited, inv)
	}

	return result, nil
}

// getOwnedGroup loads groupID and verifies that callerID owns it. It
// returns domain.ErrNotFound if groupID does not exist, and
// domain.ErrForbidden if callerID is not the group's owner.
func (u *GroupUsecase) getOwnedGroup(ctx context.Context, callerID, groupID string) (*domaingroup.Group, error) {
	g, err := u.groupRepo.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if g.OwnerID != callerID {
		return nil, domain.ErrForbidden
	}
	return g, nil
}
