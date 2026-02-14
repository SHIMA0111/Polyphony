package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// RoomRepository implements room.RoomRepository using PostgreSQL.
type RoomRepository struct {
	pool *pgxpool.Pool
}

// NewRoomRepository creates a new RoomRepository.
func NewRoomRepository(pool *pgxpool.Pool) *RoomRepository {
	return &RoomRepository{pool: pool}
}

// Create persists a new room, initializes its sequence counter, and adds the owner as a member.
func (r *RoomRepository) Create(ctx context.Context, rm *room.Room) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx,
		`INSERT INTO rooms (id, name, description, owner_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		rm.ID, rm.Name, rm.Description, rm.OwnerID, rm.CreatedAt, rm.UpdatedAt,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO room_sequences (room_id, next_sequence) VALUES ($1, 1)`,
		rm.ID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO room_members (id, room_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, 'master', $4)`,
		uuid.New().String(), rm.ID, rm.OwnerID, time.Now(),
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetByID retrieves a room by ID.
func (r *RoomRepository) GetByID(ctx context.Context, id string) (*room.Room, error) {
	var rm room.Room
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, owner_id, created_at, updated_at FROM rooms WHERE id = $1`, id,
	).Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.CreatedAt, &rm.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &rm, nil
}

// ListByUserID returns all rooms the user is a member of.
func (r *RoomRepository) ListByUserID(ctx context.Context, userID string) ([]*room.Room, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.name, r.description, r.owner_id, r.created_at, r.updated_at
		 FROM rooms r
		 INNER JOIN room_members rm ON r.id = rm.room_id
		 WHERE rm.user_id = $1
		 ORDER BY r.created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []*room.Room
	for rows.Next() {
		var rm room.Room
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.CreatedAt, &rm.UpdatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, &rm)
	}
	return rooms, rows.Err()
}

// Update updates room fields.
func (r *RoomRepository) Update(ctx context.Context, rm *room.Room) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET name = $1, description = $2, updated_at = $3 WHERE id = $4`,
		rm.Name, rm.Description, rm.UpdatedAt, rm.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a room by ID.
func (r *RoomRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM rooms WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// AddMember adds a user to a room.
func (r *RoomRepository) AddMember(ctx context.Context, member *room.RoomMember) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO room_members (id, room_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.ID, member.RoomID, member.UserID, member.Role, member.JoinedAt,
	)
	return err
}

// GetMember retrieves a specific membership.
func (r *RoomRepository) GetMember(ctx context.Context, roomID, userID string) (*room.RoomMember, error) {
	var m room.RoomMember
	err := r.pool.QueryRow(ctx,
		`SELECT id, room_id, user_id, role, joined_at FROM room_members WHERE room_id = $1 AND user_id = $2`,
		roomID, userID,
	).Scan(&m.ID, &m.RoomID, &m.UserID, &m.Role, &m.JoinedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// ListMembers returns all members of a room.
func (r *RoomRepository) ListMembers(ctx context.Context, roomID string) ([]*room.RoomMember, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, room_id, user_id, role, joined_at FROM room_members WHERE room_id = $1 ORDER BY joined_at`,
		roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*room.RoomMember
	for rows.Next() {
		var m room.RoomMember
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, rows.Err()
}

// RemoveMember removes a user from a room.
func (r *RoomRepository) RemoveMember(ctx context.Context, roomID, userID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM room_members WHERE room_id = $1 AND user_id = $2`,
		roomID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
