package mocks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// newTestInvitation builds a minimal *invitation.Invitation for the
// AcceptTx tests below. inviteeID nil produces a link invitation.
func newTestInvitation(id, roomID string, inviteeID *string, status invitation.Status) *invitation.Invitation {
	return &invitation.Invitation{
		ID:         id,
		RoomID:     roomID,
		InviterID:  "inviter-1",
		InviteeID:  inviteeID,
		InviteCode: "code-" + id,
		Role:       domainroom.RoleMember,
		Status:     status,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		CreatedAt:  time.Now(),
	}
}

// TestInvitationRepoAcceptTxRollsBackStatusOnAddMemberFailure verifies that
// when AcceptTx's status CAS succeeds (transitionStatus == true) but the
// AddMember callback then fails, the invitation's status is restored to
// what it was before the CAS rather than left stuck StatusAccepted with no
// corresponding member row — mirroring
// postgres.InvitationRepository.AcceptTx's rollback of the whole
// transaction when the room_members insert fails.
func TestInvitationRepoAcceptTxRollsBackStatusOnAddMemberFailure(t *testing.T) {
	ctx := context.Background()
	inviteeID := "invitee-1"
	inv := newTestInvitation("inv-1", "room-1", &inviteeID, invitation.StatusPending)

	wantErr := errors.New("add member failed")
	repo := &InvitationRepo{
		Invitations: map[string]*invitation.Invitation{inv.ID: inv},
		AddMember: func(context.Context, *domainroom.RoomMember) error {
			return wantErr
		},
	}

	member := &domainroom.RoomMember{
		ID:       "member-1",
		RoomID:   "room-1",
		UserID:   inviteeID,
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}
	err := repo.AcceptTx(ctx, inv.ID, invitation.StatusPending, true, member)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected the AddMember callback's error to propagate, got %v", err)
	}

	got, getErr := repo.GetByID(ctx, inv.ID)
	if getErr != nil {
		t.Fatalf("GetByID failed: %v", getErr)
	}
	if got.Status != invitation.StatusPending {
		t.Fatalf("expected status rolled back to pending after AddMember failure, got %q", got.Status)
	}
}

// TestInvitationRepoAcceptTxSucceedsWithoutRollback is a control case for
// TestInvitationRepoAcceptTxRollsBackStatusOnAddMemberFailure: when
// AddMember succeeds, the CAS's StatusAccepted transition must stick.
func TestInvitationRepoAcceptTxSucceedsWithoutRollback(t *testing.T) {
	ctx := context.Background()
	inviteeID := "invitee-1"
	inv := newTestInvitation("inv-1", "room-1", &inviteeID, invitation.StatusPending)

	var addedMember *domainroom.RoomMember
	repo := &InvitationRepo{
		Invitations: map[string]*invitation.Invitation{inv.ID: inv},
		AddMember: func(_ context.Context, member *domainroom.RoomMember) error {
			addedMember = member
			return nil
		},
	}

	member := &domainroom.RoomMember{
		ID:       "member-1",
		RoomID:   "room-1",
		UserID:   inviteeID,
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}
	if err := repo.AcceptTx(ctx, inv.ID, invitation.StatusPending, true, member); err != nil {
		t.Fatalf("AcceptTx failed: %v", err)
	}
	if addedMember != member {
		t.Fatal("expected AddMember to be invoked with the given member")
	}

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != invitation.StatusAccepted {
		t.Fatalf("expected status accepted, got %q", got.Status)
	}
}

// TestInvitationRepoAcceptTxRejectsNonPendingLinkInvitation verifies that
// AcceptTx(transitionStatus=false) — the reusable link invitation path,
// which never runs the status CAS — still refuses to accept once the
// invitation's status is no longer StatusPending (e.g. revoked), returning
// domain.ErrInvitationNotPending and never invoking AddMember.
func TestInvitationRepoAcceptTxRejectsNonPendingLinkInvitation(t *testing.T) {
	ctx := context.Background()
	link := newTestInvitation("link-1", "room-1", nil, invitation.StatusRevoked)

	called := false
	repo := &InvitationRepo{
		Invitations: map[string]*invitation.Invitation{link.ID: link},
		AddMember: func(context.Context, *domainroom.RoomMember) error {
			called = true
			return nil
		},
	}

	member := &domainroom.RoomMember{
		ID:       "member-1",
		RoomID:   "room-1",
		UserID:   "someone",
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}
	err := repo.AcceptTx(ctx, link.ID, invitation.StatusPending, false, member)
	if !errors.Is(err, domain.ErrInvitationNotPending) {
		t.Fatalf("expected ErrInvitationNotPending, got %v", err)
	}
	if called {
		t.Fatal("AddMember must not be invoked when the link invitation is no longer pending")
	}
}

// TestInvitationRepoAcceptTxLinkInvitationNotFound verifies that
// AcceptTx(transitionStatus=false) returns domain.ErrNotFound — the same
// sentinel postgres.InvitationRepository.AcceptTx returns — when
// invitationID does not exist, rather than panicking or silently invoking
// AddMember.
func TestInvitationRepoAcceptTxLinkInvitationNotFound(t *testing.T) {
	ctx := context.Background()
	repo := &InvitationRepo{}

	member := &domainroom.RoomMember{
		ID:       "member-1",
		RoomID:   "room-1",
		UserID:   "someone",
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}
	err := repo.AcceptTx(ctx, "missing-id", invitation.StatusPending, false, member)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestInvitationRepoAcceptTxAcceptsPendingLinkInvitation is a control case
// for TestInvitationRepoAcceptTxRejectsNonPendingLinkInvitation: a link
// invitation that is still StatusPending must be accepted normally, with
// AddMember invoked and its status left unchanged (link invitations are
// reusable and never transition status on accept).
func TestInvitationRepoAcceptTxAcceptsPendingLinkInvitation(t *testing.T) {
	ctx := context.Background()
	link := newTestInvitation("link-1", "room-1", nil, invitation.StatusPending)

	called := false
	repo := &InvitationRepo{
		Invitations: map[string]*invitation.Invitation{link.ID: link},
		AddMember: func(context.Context, *domainroom.RoomMember) error {
			called = true
			return nil
		},
	}

	member := &domainroom.RoomMember{
		ID:       "member-1",
		RoomID:   "room-1",
		UserID:   "someone",
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}
	if err := repo.AcceptTx(ctx, link.ID, invitation.StatusPending, false, member); err != nil {
		t.Fatalf("AcceptTx failed: %v", err)
	}
	if !called {
		t.Fatal("expected AddMember to be invoked for a pending link invitation")
	}

	got, err := repo.GetByID(ctx, link.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != invitation.StatusPending {
		t.Fatalf("expected link invitation status to remain pending, got %q", got.Status)
	}
}
