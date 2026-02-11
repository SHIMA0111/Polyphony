package handler

import (
	"context"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// Shared mock room repository for handler tests.
type mockRoomRepoForHandler struct {
	rooms   map[string]*domainroom.Room
	members map[string]map[string]*domainroom.RoomMember
}

func newMockRoomRepoForHandler() *mockRoomRepoForHandler {
	return &mockRoomRepoForHandler{
		rooms:   make(map[string]*domainroom.Room),
		members: make(map[string]map[string]*domainroom.RoomMember),
	}
}

func (m *mockRoomRepoForHandler) Create(_ context.Context, rm *domainroom.Room) error {
	m.rooms[rm.ID] = rm
	if m.members[rm.ID] == nil {
		m.members[rm.ID] = make(map[string]*domainroom.RoomMember)
	}
	m.members[rm.ID][rm.OwnerID] = &domainroom.RoomMember{
		ID: "member-1", RoomID: rm.ID, UserID: rm.OwnerID, Role: "owner",
	}
	return nil
}

func (m *mockRoomRepoForHandler) GetByID(_ context.Context, id string) (*domainroom.Room, error) {
	rm, ok := m.rooms[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return rm, nil
}

func (m *mockRoomRepoForHandler) ListByUserID(_ context.Context, userID string) ([]*domainroom.Room, error) {
	var rooms []*domainroom.Room
	for roomID, members := range m.members {
		if _, ok := members[userID]; ok {
			rooms = append(rooms, m.rooms[roomID])
		}
	}
	return rooms, nil
}

func (m *mockRoomRepoForHandler) Update(_ context.Context, rm *domainroom.Room) error {
	if _, ok := m.rooms[rm.ID]; !ok {
		return domain.ErrNotFound
	}
	m.rooms[rm.ID] = rm
	return nil
}

func (m *mockRoomRepoForHandler) Delete(_ context.Context, id string) error {
	if _, ok := m.rooms[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.rooms, id)
	return nil
}

func (m *mockRoomRepoForHandler) AddMember(_ context.Context, member *domainroom.RoomMember) error {
	if m.members[member.RoomID] == nil {
		m.members[member.RoomID] = make(map[string]*domainroom.RoomMember)
	}
	m.members[member.RoomID][member.UserID] = member
	return nil
}

func (m *mockRoomRepoForHandler) GetMember(_ context.Context, roomID, userID string) (*domainroom.RoomMember, error) {
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

func (m *mockRoomRepoForHandler) ListMembers(_ context.Context, roomID string) ([]*domainroom.RoomMember, error) {
	var result []*domainroom.RoomMember
	for _, member := range m.members[roomID] {
		result = append(result, member)
	}
	return result, nil
}

func (m *mockRoomRepoForHandler) RemoveMember(_ context.Context, roomID, userID string) error {
	if m.members[roomID] == nil {
		return domain.ErrNotFound
	}
	delete(m.members[roomID], userID)
	return nil
}
