package invitation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domaininvitation "github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
)

// newTestFixture wires up a fresh InvitationUsecase with in-memory mocks,
// seeds a room ("room-1") with an admin ("admin-1", RoleAdmin) and a plain
// member ("member-1", RoleMember), and registers a user "bob" (userID
// "bob-1") who is not yet a member of the room, so tests can invite him by
// username. It returns the usecase and the backing mocks for direct
// manipulation/assertions.
func newTestFixture() (*InvitationUsecase, *mocks.RoomRepo, *mocks.UserRepo, *mocks.InvitationRepo) {
	roomRepo := &mocks.RoomRepo{}
	userRepo := &mocks.UserRepo{}
	invitationRepo := &mocks.InvitationRepo{}

	roomRepo.SeedMember("room-1", "admin-1", "admin")
	roomRepo.SeedMember("room-1", "member-1", "member")

	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "bob-1", Email: "bob@example.com", Username: "bob"})

	uc := NewInvitationUsecase(invitationRepo, roomRepo, userRepo)
	return uc, roomRepo, userRepo, invitationRepo
}

func TestCreateInvitationByUsername(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}
	if inv.InviteeID == nil || *inv.InviteeID != "bob-1" {
		t.Fatalf("expected invitee to be resolved to bob-1, got %v", inv.InviteeID)
	}
	if inv.InviteCode == "" {
		t.Fatal("expected a non-empty invite code")
	}
	if inv.Status != domaininvitation.StatusPending {
		t.Fatalf("expected status pending, got %s", inv.Status)
	}
	wantExpiry := time.Now().Add(DefaultExpiresInHours * time.Hour)
	if inv.ExpiresAt.Sub(wantExpiry).Abs() > time.Minute {
		t.Fatalf("expected default expiry ~%v, got %v", wantExpiry, inv.ExpiresAt)
	}
}

func TestCreateInvitationAsLink(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	hours := 24

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", nil, domainroom.RoleGuest, &hours)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}
	if inv.InviteeID != nil {
		t.Fatalf("expected nil invitee for a link invitation, got %v", *inv.InviteeID)
	}
	if inv.InviteCode == "" {
		t.Fatal("expected a non-empty invite code")
	}
}

func TestCreateInvitationRejectsNonAdminInviter(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	_, err := uc.CreateInvitation(ctx, "member-1", "room-1", &username, domainroom.RoleMember, nil)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-admin inviter, got %v", err)
	}
}

func TestCreateInvitationRejectsPrivilegeEscalation(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	// admin-1 is RoleAdmin; requesting RoleMaster on the invitation would
	// outrank the inviter's own role.
	_, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMaster, nil)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for privilege escalation, got %v", err)
	}
}

func TestCreateInvitationRejectsDuplicatePending(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	if _, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil); err != nil {
		t.Fatalf("first CreateInvitation failed: %v", err)
	}

	_, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if !errors.Is(err, domain.ErrInvitationAlreadyExists) {
		t.Fatalf("expected ErrInvitationAlreadyExists for duplicate pending invitation, got %v", err)
	}
}

func TestCreateInvitationRejectsAlreadyMember(t *testing.T) {
	uc, roomRepo, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	roomRepo.SeedMember("room-1", "bob-1", "member")

	_, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if !errors.Is(err, domain.ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember, got %v", err)
	}
}

func TestAcceptInvitationByUsername(t *testing.T) {
	uc, roomRepo, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	member, err := uc.AcceptInvitation(ctx, "bob-1", inv.ID)
	if err != nil {
		t.Fatalf("AcceptInvitation failed: %v", err)
	}
	if member.RoomID != "room-1" || member.UserID != "bob-1" || member.Role != domainroom.RoleMember {
		t.Fatalf("unexpected membership: %+v", member)
	}

	if _, err := roomRepo.GetMember(ctx, "room-1", "bob-1"); err != nil {
		t.Fatalf("expected bob-1 to now be a room member: %v", err)
	}
}

func TestAcceptInvitationByLink(t *testing.T) {
	uc, roomRepo, _, _ := newTestFixture()
	ctx := context.Background()
	hours := 24

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", nil, domainroom.RoleGuest, &hours)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	member, err := uc.AcceptInvitation(ctx, "bob-1", inv.ID)
	if err != nil {
		t.Fatalf("AcceptInvitation failed: %v", err)
	}
	if member.Role != domainroom.RoleGuest {
		t.Fatalf("expected role guest, got %s", member.Role)
	}
	if _, err := roomRepo.GetMember(ctx, "room-1", "bob-1"); err != nil {
		t.Fatalf("expected bob-1 to now be a room member: %v", err)
	}

	// Link invitations remain reusable: status must still be pending.
	got, err := uc.invitationRepo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domaininvitation.StatusPending {
		t.Fatalf("expected link invitation to remain pending after accept, got %s", got.Status)
	}
}

func TestAcceptInvitationExpired(t *testing.T) {
	uc, _, _, invitationRepo := newTestFixture()
	ctx := context.Background()

	inv := &domaininvitation.Invitation{
		ID:         "inv-expired",
		RoomID:     "room-1",
		InviterID:  "admin-1",
		InviteeID:  strPtr("bob-1"),
		InviteCode: "code-expired",
		Role:       domainroom.RoleMember,
		Status:     domaininvitation.StatusPending,
		ExpiresAt:  time.Now().Add(-time.Hour),
		CreatedAt:  time.Now().Add(-2 * time.Hour),
	}
	if err := invitationRepo.Create(ctx, inv); err != nil {
		t.Fatalf("seed Create failed: %v", err)
	}

	_, err := uc.AcceptInvitation(ctx, "bob-1", inv.ID)
	if !errors.Is(err, domain.ErrInvitationExpired) {
		t.Fatalf("expected ErrInvitationExpired, got %v", err)
	}
}

func TestAcceptInvitationTwiceOnUsernameInvitation(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	if _, err := uc.AcceptInvitation(ctx, "bob-1", inv.ID); err != nil {
		t.Fatalf("first AcceptInvitation failed: %v", err)
	}

	// Second accept: bob-1 is already a member, so ErrAlreadyMember fires
	// before the not-pending check.
	_, err = uc.AcceptInvitation(ctx, "bob-1", inv.ID)
	if !errors.Is(err, domain.ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember on second accept, got %v", err)
	}
}

func TestAcceptInvitationNotPendingAfterRemoval(t *testing.T) {
	uc, roomRepo, _, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}
	if _, err := uc.AcceptInvitation(ctx, "bob-1", inv.ID); err != nil {
		t.Fatalf("AcceptInvitation failed: %v", err)
	}

	// Remove the membership out-of-band so a repeat accept attempt reaches
	// the invitation-status check instead of short-circuiting on
	// ErrAlreadyMember, exercising the "second accept/reject on a
	// non-pending invitation" rule directly.
	if err := roomRepo.RemoveMember(ctx, "room-1", "bob-1"); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	_, err = uc.AcceptInvitation(ctx, "bob-1", inv.ID)
	if !errors.Is(err, domain.ErrInvitationNotPending) {
		t.Fatalf("expected ErrInvitationNotPending, got %v", err)
	}
}

func TestAcceptInvitationForbiddenForDifferentUser(t *testing.T) {
	uc, _, userRepo, _ := newTestFixture()
	ctx := context.Background()
	username := "bob"

	_ = userRepo.Create(ctx, &domainuser.User{ID: "carol-1", Email: "carol@example.com", Username: "carol"})

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	_, err = uc.AcceptInvitation(ctx, "carol-1", inv.ID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden when a different user accepts, got %v", err)
	}
}

func TestRejectInvitation(t *testing.T) {
	uc, _, _, invitationRepo := newTestFixture()
	ctx := context.Background()
	username := "bob"

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	if err := uc.RejectInvitation(ctx, "bob-1", inv.ID); err != nil {
		t.Fatalf("RejectInvitation failed: %v", err)
	}

	got, err := invitationRepo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Status != domaininvitation.StatusRejected {
		t.Fatalf("expected status rejected, got %s", got.Status)
	}

	// A second reject on the now-rejected invitation must fail.
	err = uc.RejectInvitation(ctx, "bob-1", inv.ID)
	if !errors.Is(err, domain.ErrInvitationNotPending) {
		t.Fatalf("expected ErrInvitationNotPending on repeat reject, got %v", err)
	}
}

func TestRejectInvitationOnLinkInvitationForbidden(t *testing.T) {
	uc, _, _, _ := newTestFixture()
	ctx := context.Background()
	hours := 24

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", nil, domainroom.RoleGuest, &hours)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	err = uc.RejectInvitation(ctx, "bob-1", inv.ID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden rejecting a link invitation, got %v", err)
	}
}

func strPtr(s string) *string { return &s }

// TestConcurrentAcceptAndRejectNoMixedFinalState is a regression test for
// Step 22/23's compare-and-swap fix: AcceptInvitation and RejectInvitation
// racing on the very same username-targeted invitation must never leave a
// "mixed" final state -- a room_members row added while the invitation's
// final status reads StatusRejected, or no room_members row while it reads
// StatusAccepted. Before the fix (AddMember running unconditionally before
// the status transition), the loser of the status-update race could still
// have already added the member, producing exactly that inconsistency.
//
// Run with -race to also catch any data race in the CAS path itself.
func TestConcurrentAcceptAndRejectNoMixedFinalState(t *testing.T) {
	uc, roomRepo, _, invitationRepo := newTestFixture()
	ctx := context.Background()
	username := "bob"

	inv, err := uc.CreateInvitation(ctx, "admin-1", "room-1", &username, domainroom.RoleMember, nil)
	if err != nil {
		t.Fatalf("CreateInvitation failed: %v", err)
	}

	var wg sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})

	var acceptErr, rejectErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		_, acceptErr = uc.AcceptInvitation(ctx, "bob-1", inv.ID)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ready.Done()
		<-start
		rejectErr = uc.RejectInvitation(ctx, "bob-1", inv.ID)
	}()

	ready.Wait()
	close(start)
	wg.Wait()

	// Exactly one of the two must have won the CAS; the other must have
	// observed a transition conflict.
	acceptWon := acceptErr == nil
	rejectWon := rejectErr == nil
	if acceptWon == rejectWon {
		t.Fatalf("expected exactly one of accept/reject to win, got acceptErr=%v rejectErr=%v", acceptErr, rejectErr)
	}
	if acceptWon && !errors.Is(rejectErr, domain.ErrInvitationNotPending) {
		t.Fatalf("expected the losing reject to fail with ErrInvitationNotPending, got %v", rejectErr)
	}
	if rejectWon && !errors.Is(acceptErr, domain.ErrInvitationNotPending) {
		t.Fatalf("expected the losing accept to fail with ErrInvitationNotPending, got %v", acceptErr)
	}

	got, err := invitationRepo.GetByID(ctx, inv.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	_, memberErr := roomRepo.GetMember(ctx, "room-1", "bob-1")
	isMember := memberErr == nil

	// The invariant under test: the final invitation status and whether bob
	// ended up a room member must agree -- never "rejected but a member was
	// added" nor "accepted but no member exists".
	switch got.Status {
	case domaininvitation.StatusAccepted:
		if !isMember {
			t.Fatalf("status is accepted but bob-1 was never added as a room member")
		}
		if !acceptWon {
			t.Fatalf("status is accepted but the accept call did not report success")
		}
	case domaininvitation.StatusRejected:
		if isMember {
			t.Fatalf("status is rejected but bob-1 was added as a room member (mixed final state)")
		}
		if !rejectWon {
			t.Fatalf("status is rejected but the reject call did not report success")
		}
	default:
		t.Fatalf("expected a terminal status (accepted/rejected), got %s", got.Status)
	}
}
