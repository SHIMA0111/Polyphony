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

// TestGroupRepositoryAddMemberDuplicateReturnsAlreadyMember asserts that a
// second AddMember call for the same (group_id, user_id) pair returns
// domain.ErrAlreadyMember rather than an unmapped Postgres unique-violation
// error.
func TestGroupRepositoryAddMemberDuplicateReturnsAlreadyMember(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()}); err != nil {
		t.Fatalf("first AddMember failed: %v", err)
	}

	err := repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()})
	if !errors.Is(err, domain.ErrAlreadyMember) {
		t.Fatalf("expected domain.ErrAlreadyMember for a duplicate AddMember, got %v", err)
	}
}

// TestGroupRepositoryConcurrentAddMemberOneWinsOneAlreadyMember races two
// concurrent AddMember calls for the same (group_id, user_id) pair: exactly
// one must succeed and the other must observe domain.ErrAlreadyMember (never
// an unmapped constraint-violation error), and exactly one group_members row
// must exist afterward regardless of which call the database happened to
// commit first.
func TestGroupRepositoryConcurrentAddMemberOneWinsOneAlreadyMember(t *testing.T) {
	ctx := context.Background()
	repo, owner, member := newGroupTestFixture(ctx, t)

	g := newTestGroup(owner.ID)
	if err := repo.Create(ctx, g); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	var wg sync.WaitGroup
	var err1, err2 error
	wg.Add(2)
	go func() {
		defer wg.Done()
		err1 = repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()})
	}()
	go func() {
		defer wg.Done()
		err2 = repo.AddMember(ctx, &group.GroupMember{ID: uuid.New().String(), GroupID: g.ID, UserID: member.ID, AddedAt: time.Now()})
	}()
	wg.Wait()

	successes, alreadyMembers := 0, 0
	for _, err := range []error{err1, err2} {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrAlreadyMember):
			alreadyMembers++
		default:
			t.Fatalf("expected nil or domain.ErrAlreadyMember, got %v", err)
		}
	}
	if successes != 1 || alreadyMembers != 1 {
		t.Fatalf("expected exactly one success and one ErrAlreadyMember, got successes=%d alreadyMembers=%d (err1=%v, err2=%v)",
			successes, alreadyMembers, err1, err2)
	}

	members, err := repo.ListMembers(ctx, g.ID)
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected exactly 1 member after the race, got %d", len(members))
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
