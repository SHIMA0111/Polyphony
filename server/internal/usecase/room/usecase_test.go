package room

import (
	"context"
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

type mockRoomRepo struct {
	rooms   map[string]*domainroom.Room
	members map[string]map[string]*domainroom.RoomMember // roomID -> userID -> member
}

func newMockRoomRepo() *mockRoomRepo {
	return &mockRoomRepo{
		rooms:   make(map[string]*domainroom.Room),
		members: make(map[string]map[string]*domainroom.RoomMember),
	}
}

func (m *mockRoomRepo) Create(_ context.Context, rm *domainroom.Room) error {
	m.rooms[rm.ID] = rm
	if m.members[rm.ID] == nil {
		m.members[rm.ID] = make(map[string]*domainroom.RoomMember)
	}
	m.members[rm.ID][rm.OwnerID] = &domainroom.RoomMember{
		ID: "member-1", RoomID: rm.ID, UserID: rm.OwnerID, Role: "owner",
	}
	return nil
}

func (m *mockRoomRepo) GetByID(_ context.Context, id string) (*domainroom.Room, error) {
	rm, ok := m.rooms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return rm, nil
}

func (m *mockRoomRepo) ListByUserID(_ context.Context, userID string) ([]*domainroom.Room, error) {
	var rooms []*domainroom.Room
	for roomID, members := range m.members {
		if _, ok := members[userID]; ok {
			rooms = append(rooms, m.rooms[roomID])
		}
	}
	return rooms, nil
}

func (m *mockRoomRepo) Update(_ context.Context, rm *domainroom.Room) error {
	if _, ok := m.rooms[rm.ID]; !ok {
		return domain.ErrNotFound
	}
	m.rooms[rm.ID] = rm
	return nil
}

func (m *mockRoomRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.rooms[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.rooms, id)
	delete(m.members, id)
	return nil
}

func (m *mockRoomRepo) AddMember(_ context.Context, member *domainroom.RoomMember) error {
	if m.members[member.RoomID] == nil {
		m.members[member.RoomID] = make(map[string]*domainroom.RoomMember)
	}
	m.members[member.RoomID][member.UserID] = member
	return nil
}

func (m *mockRoomRepo) GetMember(_ context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
	members, ok := m.members[roomID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	member, ok := members[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return member, nil
}

func (m *mockRoomRepo) ListMembers(_ context.Context, roomID string) ([]*domainroom.RoomMember, error) {
	members := m.members[roomID]
	var result []*domainroom.RoomMember
	for _, member := range members {
		result = append(result, member)
	}
	return result, nil
}

func (m *mockRoomRepo) RemoveMember(_ context.Context, roomID, userID string) error {
	members, ok := m.members[roomID]
	if !ok {
		return domain.ErrNotFound
	}
	if _, ok := members[userID]; !ok {
		return domain.ErrNotFound
	}
	delete(members, userID)
	return nil
}

func TestCreateRoom(t *testing.T) {
	repo := newMockRoomRepo()
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, err := uc.CreateRoom(ctx, "user-1", "Test Room", "A test room")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if rm.Name != "Test Room" {
		t.Fatalf("expected Test Room, got %s", rm.Name)
	}
	if rm.OwnerID != "user-1" {
		t.Fatalf("expected owner user-1, got %s", rm.OwnerID)
	}
}

func TestGetRoomNotMember(t *testing.T) {
	repo := newMockRoomRepo()
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	_, err := uc.GetRoom(ctx, "user-2", rm.ID)
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateRoomNotOwner(t *testing.T) {
	repo := newMockRoomRepo()
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	// Add user-2 as member
	_ = repo.AddMember(ctx, &domainroom.RoomMember{
		ID: "m2", RoomID: rm.ID, UserID: "user-2", Role: "member",
	})

	_, err := uc.UpdateRoom(ctx, "user-2", rm.ID, "New Name", "new desc")
	if err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestDeleteRoom(t *testing.T) {
	repo := newMockRoomRepo()
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rm, _ := uc.CreateRoom(ctx, "user-1", "Test Room", "desc")

	err := uc.DeleteRoom(ctx, "user-1", rm.ID)
	if err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	_, err = uc.GetRoom(ctx, "user-1", rm.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestListRoomsEmpty(t *testing.T) {
	repo := newMockRoomRepo()
	uc := NewRoomUsecase(repo)
	ctx := context.Background()

	rooms, err := uc.ListRooms(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListRooms failed: %v", err)
	}
	if len(rooms) != 0 {
		t.Fatalf("expected 0 rooms, got %d", len(rooms))
	}
}
