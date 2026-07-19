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
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/group"
	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	testutilpg "github.com/SHIMA0111/multi-user-ai/server/internal/testutil/postgres"
)

// newGroupTestFixture starts a fresh testcontainers-backed PostgreSQL
// instance, creates an owner user and a member user, and returns everything
// a test needs to exercise GroupRepository against them.
func newGroupTestFixture(ctx context.Context, t *testing.T) (*GroupRepository, *domainuser.User, *domainuser.User) {
	t.Helper()
	pool := testutilpg.New(ctx, t)

	userRepo := NewUserRepository(pool)
	groupRepo := NewGroupRepository(pool)

	owner := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "owner-" + uuid.New().String() + "@example.com",
		Username:     "owner-" + uuid.New().String(),
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, owner); err != nil {
		t.Fatalf("create owner user: %v", err)
	}

	member := &domainuser.User{
		ID:           uuid.New().String(),
		Email:        "member-" + uuid.New().String() + "@example.com",
		Username:     "member-" + uuid.New().String(),
		PasswordHash: "hash",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := userRepo.Create(ctx, member); err != nil {
		t.Fatalf("create member user: %v", err)
	}

	return groupRepo, owner, member
}

func newTestGroup(ownerID string) *group.Group {
	now := time.Now()
	return &group.Group{
		ID:          uuid.New().String(),
		OwnerID:     ownerID,
		Name:        "Test Group",
		Description: "a test group",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestGroupRepositoryCreateAndGetByID(t *testing.T) {
	ctx := context.Background()
	repo, owner, _ := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := repo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.OwnerID != owner.ID || got.Name != g.Name || got.Description != g.Description {
		t.Fatalf("unexpected group: %+v", got)
	}
}

func TestGroupRepositoryGetByIDNotFound(t *testing.T) {
	ctx := context.Background()
	repo, _, _ := newGroupTestFixture(ctx, t)

	if _, err := repo.GetByID(ctx, uuid.New().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGroupRepositoryListByOwnerID(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g1 := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g1); err != nil {
		t.Fatalf("Create g1 failed: %v", err)
	}
	g2 := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g2); err != nil {
		t.Fatalf("Create g2 failed: %v", err)
	}

	// A group owned by a different user (here, the fixture's "member" user)
	// must not appear in owner's list.
	otherGroup := newTestGroup(member.ID)
	if err := repo.Create(ctx, otherGroup); err != nil {
		t.Fatalf("Create otherGroup failed: %v", err)
	}

	list, err := repo.ListByOwnerID(ctx, owner.ID)
	if err != nil {
		t.Fatalf("ListByOwnerID failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(list))
	}
	for _, g := range list {
		if g.OwnerID != owner.ID {
			t.Fatalf("expected only owner's groups, got one owned by %s", g.OwnerID)
		}
	}
}

func TestGroupRepositoryUpdate(t *testing.T) {
	ctx := context.Background()
	repo, owner, _ := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	g.Name = "Renamed Group"
	g.Description = "updated description"
	g.UpdatedAt = time.Now()
	if err := repo.Update(ctx, g); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, err := repo.GetByID(ctx, g.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Name != "Renamed Group" || got.Description != "updated description" {
		t.Fatalf("unexpected group after update: %+v", got)
	}

	nonexistent := newTestGroup(owner.ID)
	if err := repo.Update(ctx, nonexistent); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound updating a nonexistent group, got %v", err)
	}
}

func TestGroupRepositoryDelete(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	if err := repo.Delete(ctx, g.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if _, err := repo.GetByID(ctx, g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	// ON DELETE CASCADE must have removed the group_members row too.
	if _, err := repo.GetMember(ctx, g.ID, member.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for member after cascading delete, got %v", err)
	}

	if err := repo.Delete(ctx, uuid.New().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting a nonexistent group, got %v", err)
	}
}

func TestGroupRepositoryAddMemberGetMemberRemoveMember(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	gm := &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()}
	if err := repo.AddMember(ctx, gm); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	got, err := repo.GetMember(ctx, g.ID, member.ID)
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if got.GroupID != g.ID || got.UserID != member.ID {
		t.Fatalf("unexpected member: %+v", got)
	}

	if _, err := repo.GetMember(ctx, g.ID, uuid.New().String()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown member, got %v", err)
	}

	if err := repo.RemoveMember(ctx, g.ID, member.ID); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	if _, err := repo.GetMember(ctx, g.ID, member.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after RemoveMember, got %v", err)
	}
	if err := repo.RemoveMember(ctx, g.ID, member.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound removing an already-removed member, got %v", err)
	}
}

// TestGroupRepositoryAddMemberDuplicateReturnsErrAlreadyMember verifies that
// adding the same user to a group twice returns domain.ErrAlreadyMember rather
// than an unmapped Postgres unique-violation error.
func TestGroupRepositoryAddMemberDuplicateReturnsErrAlreadyMember(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()}); err != nil {
		t.Fatalf("AddMember (first) failed: %v", err)
	}

	// A second AddMember for the same (group_id, user_id) pair, with a
	// distinct row ID, must hit the group_members_group_id_user_id_key
	// unique constraint and be mapped to domain.ErrAlreadyMember rather
	// than propagated as a raw *pgconn.PgError.
	err := repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()})
	if !errors.Is(err, domain.ErrAlreadyMember) {
		t.Fatalf("expected ErrAlreadyMember on duplicate AddMember, got %v", err)
	}
}

// TestGroupRepositoryAddMemberConcurrentDuplicateReturnsErrAlreadyMember
// fires two AddMember calls for the same (group_id, user_id) pair
// concurrently, so exactly one hits the unique-constraint race at the
// database level rather than losing to an application-level
// check-then-insert race. Exactly one call must succeed and the other must
// return domain.ErrAlreadyMember.
func TestGroupRepositoryAddMemberConcurrentDuplicateReturnsErrAlreadyMember(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	const attempts = 2
	errs := make([]error, attempts)
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(i int) {
			defer wg.Done()
			errs[i] = repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()})
		}(i)
	}
	wg.Wait()

	var successes, alreadyMember int
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrAlreadyMember):
			alreadyMember++
		default:
			t.Fatalf("unexpected error from concurrent AddMember: %v", err)
		}
	}
	if successes != 1 || alreadyMember != 1 {
		t.Fatalf("expected exactly 1 success and 1 ErrAlreadyMember, got %d successes and %d ErrAlreadyMember (errs: %v)", successes, alreadyMember, errs)
	}
}

func TestGroupRepositoryListMembersResolvesUsername(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	members, err := repo.ListMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].UserID != member.ID || members[0].Username != member.Username {
		t.Fatalf("expected username %s resolved via JOIN, got %+v", member.Username, members[0])
	}
}
