//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// newInvitationTestFixture starts a fresh testcontainers-backed PostgreSQL
// instance, creates an inviter user, an invitee user, and a room owned by
// the inviter, and returns everything a test needs to exercise
// InvitationRepository against them.
func newInvitationTestFixture(ctx context.Context, t *testing.T) (*InvitationRepository, *RoomRepository, *domainuser.User, *domainuser.User, *domainroom.Room) {
	t.Helper()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	roomRepo := NewRoomRepository(pool)
	invitationRepo := NewInvitationRepository(pool)

	inviter := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "inviter-" + uuid.New().String() + "@example.com",
		Username:     "inviter-" + uuid.New().String(),
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, inviter); err != nil {
		t.Fatalf("create inviter user: %v", err)
	}

	invitee := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "invitee-" + uuid.New().String() + "@example.com",
		Username:     "invitee-" + uuid.New().String(),
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, invitee); err != nil {
		t.Fatalf("create invitee user: %v", err)
	}

	rm := &domainroom.Room{
		ID:          uuid.New().String(),
		Name:        "Invitation Test Room",
		Description: "",
		OwnerID:     inviter.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := roomRepo.Create(ctx, rm); err != nil {
		t.Fatalf("create room: %v", err)
	}

	return invitationRepo, roomRepo, inviter, invitee, rm
}

func newTestInvitation(roomID, inviterID string, inviteeID *string, code string) *invitation.Invitation {
	now := time.Now()
	return &invitation.Invitation{
		ID:         uuid.New().String(),
		RoomID:     roomID,
		InviterID:  inviterID,
		InviteeID:  inviteeID,
		InviteCode: code,
		Role:       domainroom.RoleMember,
		Status:     invitation.StatusPending,
		ExpiresAt:  now.Add(7 * 24 * time.Hour),
		CreatedAt:  now,
	}
}

func TestInvitationRepositoryCreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	inv := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, uuid.New().String())
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.RoomID != rm.ID || got.InviterID != inviter.ID {
		t.Fatalf("unexpected invitation: %+v", got)
	}
	if got.InviteeID == nil || *got.InviteeID != invitee.ID {
		t.Fatalf("expected invitee_id %s, got %v", invitee.ID, got.InviteeID)
	}
	if got.Role != domainroom.RoleMember {
		t.Fatalf("expected role member, got %s", got.Role)
	}
	if got.Status != invitation.StatusPending {
		t.Fatalf("expected status pending, got %s", got.Status)
	}
}

func TestInvitationRepositoryGetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	repo, _, _, _, _ := newInvitationTestFixture(ctx, t)

	_, err := repo.GetByID(ctx, uuid.New().String())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestInvitationRepositoryGetByCode(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, _, rm := newInvitationTestFixture(ctx, t)

	code := uuid.New().String()
	inv := newTestInvitation(rm.ID, inviter.ID, nil, code)
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByCode(ctx, code)
	if err != nil {
		t.Fatalf("GetByCode failed: %v", err)
	}
	if got.ID != inv.ID {
		t.Fatalf("expected invitation %s, got %s", inv.ID, got.ID)
	}
	if got.InviteeID != nil {
		t.Fatalf("expected a link invitation (nil InviteeID), got %v", got.InviteeID)
	}

	if _, err := repo.GetByCode(ctx, "nonexistent-code"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown code, got %v", err)
	}
}

func TestInvitationRepositoryGetPendingByRoomAndInvitee(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	inv := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, uuid.New().String())
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetPendingByRoomAndInvitee(ctx, rm.ID, invitee.ID)
	if err != nil {
		t.Fatalf("GetPendingByRoomAndInvitee failed: %v", err)
	}
	if got.ID != inv.ID {
		t.Fatalf("expected invitation %s, got %s", inv.ID, got.ID)
	}

	if err := repo.UpdateStatus(ctx, inv.ID, invitation.StatusAccepted, invitation.StatusPending); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	if _, err := repo.GetPendingByRoomAndInvitee(ctx, rm.ID, invitee.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound once the invitation is no longer pending, got %v", err)
	}
}

func TestInvitationRepositoryListByRoomID(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	inv1 := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, uuid.New().String())
	if err := repo.Create(ctx, inv1); err != nil {
		t.Fatalf("Create inv1 failed: %v", err)
	}
	inv2 := newTestInvitation(rm.ID, inviter.ID, nil, uuid.New().String())
	if err := repo.Create(ctx, inv2); err != nil {
		t.Fatalf("Create inv2 failed: %v", err)
	}

	list, err := repo.ListByRoomID(ctx, rm.ID)
	if err != nil {
		t.Fatalf("ListByRoomID failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 invitations, got %d", len(list))
	}
}

func TestInvitationRepositoryListPendingByInviteeID(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	inv := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, uuid.New().String())
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	// A link invitation must never appear in ListPendingByInviteeID, even
	// though it shares the same room.
	link := newTestInvitation(rm.ID, inviter.ID, nil, uuid.New().String())
	if err := repo.Create(ctx, link); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	list, err := repo.ListPendingByInviteeID(ctx, invitee.ID)
	if err != nil {
		t.Fatalf("ListPendingByInviteeID failed: %v", err)
	}
	if len(list) != 1 || list[0].ID != inv.ID {
		t.Fatalf("expected exactly [%s], got %+v", inv.ID, list)
	}
}

func TestInvitationRepositoryUpdateStatus(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	inv := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, uuid.New().String())
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.UpdateStatus(ctx, inv.ID, invitation.StatusRejected, invitation.StatusPending); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != invitation.StatusRejected {
		t.Fatalf("expected status rejected, got %s", got.Status)
	}

	if err := repo.UpdateStatus(ctx, uuid.New().String(), invitation.StatusRejected, invitation.StatusPending); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound updating a nonexistent invitation, got %v", err)
	}

	// The invitation is now StatusRejected, so a CAS expecting StatusPending
	// must fail as a transition conflict rather than silently overwriting it.
	if err := repo.UpdateStatus(ctx, inv.ID, invitation.StatusAccepted, invitation.StatusPending); !errors.Is(err, domain.ErrInvitationNotPending) {
		t.Fatalf("expected ErrInvitationNotPending for a CAS against a stale expectedStatus, got %v", err)
	}
	got, err = repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != invitation.StatusRejected {
		t.Fatalf("expected status to remain rejected after a failed CAS, got %s", got.Status)
	}
}

// TestInvitationRepositoryAcceptTxConcurrentAcceptReject races AcceptTx
// (from a simulated AcceptInvitation) against UpdateStatus (from a
// simulated RejectInvitation) for the same username-targeted invitation,
// and asserts the final state is never mixed: either the invitation ends up
// StatusAccepted with the room member present, or StatusRejected with no
// room member ever inserted -- never both a member row AND a rejected
// status, and never neither outcome (e.g. a status stuck on pending). This
// exercises the real Postgres transaction/row-locking behavior AcceptTx
// relies on, which the in-memory mocks.InvitationRepo can only approximate
// with a shared mutex.
func TestInvitationRepositoryAcceptTxConcurrentAcceptReject(t *testing.T) {
	ctx := context.Background()
	repo, roomRepo, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	inv := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, uuid.New().String())
	if err := repo.Create(ctx, inv); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	member := &domainroom.RoomMember{
		ID:       uuid.New().String(),
		RoomID:   rm.ID,
		UserID:   invitee.ID,
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}

	// ready/start form a barrier that provably starts both goroutines
	// concurrently: each signals ready.Done() and then blocks on <-start,
	// so neither can begin its repo call until the main goroutine has
	// observed both are waiting (ready.Wait()) and releases them together
	// (close(start)). Without this, one goroutine could race ahead and
	// finish before the other even begins, defeating the point of the test.
	var ready sync.WaitGroup
	start := make(chan struct{})
	var wg sync.WaitGroup
	var acceptErr, rejectErr error
	ready.Add(2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		acceptErr = repo.AcceptTx(ctx, inv.ID, invitation.StatusPending, true, member)
	}()
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		rejectErr = repo.UpdateStatus(ctx, inv.ID, invitation.StatusRejected, invitation.StatusPending)
	}()
	ready.Wait()
	close(start)
	wg.Wait()

	got, err := repo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	_, memberErr := roomRepo.GetMember(ctx, rm.ID, invitee.ID)
	isMember := memberErr == nil

	switch got.Status {
	case invitation.StatusAccepted:
		if acceptErr != nil {
			t.Fatalf("status is accepted but AcceptTx returned an error: %v", acceptErr)
		}
		if rejectErr == nil {
			t.Fatal("status is accepted but RejectInvitation's UpdateStatus did not report a conflict")
		}
		if !isMember {
			t.Fatal("status is accepted but the room member was never inserted")
		}
	case invitation.StatusRejected:
		if rejectErr != nil {
			t.Fatalf("status is rejected but UpdateStatus returned an error: %v", rejectErr)
		}
		if acceptErr == nil {
			t.Fatal("status is rejected but AcceptTx did not report a conflict")
		}
		if isMember {
			t.Fatal("status is rejected but a room member was inserted anyway")
		}
	default:
		t.Fatalf("expected a definitive final status (accepted or rejected), got %q", got.Status)
	}
}

// TestInvitationRepositoryAcceptTxRejectsRevokedLinkInvitation verifies the
// transitionStatus=false path's TOCTOU fix: if a reusable link invitation
// is revoked (or otherwise moved out of StatusPending) before AcceptTx
// runs, AcceptTx must lock and re-check the row's status inside its own
// transaction and refuse the accept, rather than trusting a caller's
// now-stale pre-check read. Before this fix, AcceptTx(transitionStatus=
// false) ran no status validation at all and would have inserted the room
// member regardless of a concurrent revoke or expiry.
func TestInvitationRepositoryAcceptTxRejectsRevokedLinkInvitation(t *testing.T) {
	ctx := context.Background()
	repo, roomRepo, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	link := newTestInvitation(rm.ID, inviter.ID, nil, uuid.New().String())
	if err := repo.Create(ctx, link); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Simulate a revoke landing after the usecase's own pre-check read but
	// before its call to AcceptTx.
	if err := repo.UpdateStatus(ctx, link.ID, invitation.StatusRevoked, invitation.StatusPending); err != nil {
		t.Fatalf("UpdateStatus (revoke) failed: %v", err)
	}

	member := &domainroom.RoomMember{
		ID:       uuid.New().String(),
		RoomID:   rm.ID,
		UserID:   invitee.ID,
		Role:     domainroom.RoleMember,
		JoinedAt: time.Now(),
	}
	err := repo.AcceptTx(ctx, link.ID, invitation.StatusPending, false, member)
	if !errors.Is(err, domain.ErrInvitationNotPending) {
		t.Fatalf("expected ErrInvitationNotPending for a revoked link invitation, got %v", err)
	}

	if _, memberErr := roomRepo.GetMember(ctx, rm.ID, invitee.ID); !errors.Is(memberErr, domain.ErrNotFound) {
		t.Fatalf("expected no room_members row to be inserted, got member lookup error %v", memberErr)
	}
}

func TestInvitationRepositoryInviteCodeUniqueConstraint(t *testing.T) {
	ctx := context.Background()
	repo, _, inviter, invitee, rm := newInvitationTestFixture(ctx, t)

	code := uuid.New().String()
	inv1 := newTestInvitation(rm.ID, inviter.ID, &invitee.ID, code)
	if err := repo.Create(ctx, inv1); err != nil {
		t.Fatalf("Create inv1 failed: %v", err)
	}

	inv2 := newTestInvitation(rm.ID, inviter.ID, nil, code)
	err := repo.Create(ctx, inv2)
	if !errors.Is(err, invitation.ErrInviteCodeConflict) {
		t.Fatalf("expected ErrInviteCodeConflict for a duplicate invite_code, got %v", err)
	}
}
