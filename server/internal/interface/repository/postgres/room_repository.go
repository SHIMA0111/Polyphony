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

// RoomRepository implements the room.RoomRepository interface using PostgreSQL.
type RoomRepository struct {
	pool *pgxpool.Pool
}

// NewRoomRepository creates a new RoomRepository backed by the given connection pool.
func NewRoomRepository(pool *pgxpool.Pool) *RoomRepository {
	return &RoomRepository{pool: pool}
}

// Create persists a new room, initializes its sequence counter, and adds the owner as a master member.
// The entire operation runs within a single transaction to ensure atomicity.
func (r *RoomRepository) Create(ctx context.Context, rm *room.Room) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

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
		 VALUES ($1, $2, $3, $4, $5)`,
		uuid.New().String(), rm.ID, rm.OwnerID, string(room.RoleMaster), time.Now(),
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetByID retrieves a room by its unique identifier. It returns domain.ErrNotFound if the room does not exist.
func (r *RoomRepository) GetByID(ctx context.Context, id string) (*room.Room, error) {
	var rm room.Room
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, owner_id, ai_context_cutoff_at, created_at, updated_at FROM rooms WHERE id = $1`, id,
	).Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.AIContextCutoffAt, &rm.CreatedAt, &rm.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &rm, nil
}

// ListByUserID returns all rooms that the given user is a member of, ordered by creation time descending.
func (r *RoomRepository) ListByUserID(ctx context.Context, userID string) ([]*room.Room, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.name, r.description, r.owner_id, r.ai_context_cutoff_at, r.created_at, r.updated_at
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
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.AIContextCutoffAt, &rm.CreatedAt, &rm.UpdatedAt); err != nil {
			return nil, err
		}
		rooms = append(rooms, &rm)
	}
	return rooms, rows.Err()
}

// ListByUserIDWithRole returns all rooms that the given user is a member of,
// together with the user's role in each room, ordered by creation time
// descending. It performs a single INNER JOIN query (no N+1 GetMember
// lookups per room).
func (r *RoomRepository) ListByUserIDWithRole(ctx context.Context, userID string) ([]*room.RoomWithRole, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT r.id, r.name, r.description, r.owner_id, r.ai_context_cutoff_at, r.created_at, r.updated_at, rm.role
		 FROM rooms r
		 INNER JOIN room_members rm ON r.id = rm.room_id
		 WHERE rm.user_id = $1
		 ORDER BY r.created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*room.RoomWithRole
	for rows.Next() {
		var rm room.Room
		var roleStr string
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.AIContextCutoffAt, &rm.CreatedAt, &rm.UpdatedAt, &roleStr); err != nil {
			return nil, err
		}
		result = append(result, &room.RoomWithRole{Room: &rm, Role: room.Role(roleStr)})
	}
	return result, rows.Err()
}

// UpdateDetails updates only a room's name, description, and updated_at
// columns, per room.RoomRepository's UpdateDetails GoDoc (a deliberate
// partial update that leaves ai_context_cutoff_at untouched, unlike the
// full-row update this replaced). It returns domain.ErrNotFound if the room
// does not exist.
func (r *RoomRepository) UpdateDetails(ctx context.Context, roomID, name, description string, updatedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET name = $1, description = $2, updated_at = $3 WHERE id = $4`,
		name, description, updatedAt, roomID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateAIContextCutoff updates only a room's ai_context_cutoff_at and
// updated_at columns, per room.RoomRepository's UpdateAIContextCutoff
// GoDoc (a deliberate partial update that leaves name/description
// untouched). It returns domain.ErrNotFound if the room does not exist.
func (r *RoomRepository) UpdateAIContextCutoff(ctx context.Context, roomID string, cutoff *time.Time, updatedAt time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET ai_context_cutoff_at = $1, updated_at = $2 WHERE id = $3`,
		cutoff, updatedAt, roomID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a room by its unique identifier. It returns domain.ErrNotFound if the room does not exist.
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

// AddMember adds a user to a room with the role specified in the RoomMember struct.
func (r *RoomRepository) AddMember(ctx context.Context, member *room.RoomMember) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO room_members (id, room_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.ID, member.RoomID, member.UserID, string(member.Role), member.JoinedAt,
	)
	return err
}

// GetMember retrieves a specific room membership by room ID and user ID. It
// JOINs against the users table to populate RoomMember.Username, mirroring
// ListMembers, so a single-membership lookup is just as usable for
// display purposes as a list one. It returns domain.ErrNotFound if the
// membership does not exist.
func (r *RoomRepository) GetMember(ctx context.Context, roomID, userID string) (*room.RoomMember, error) {
	var m room.RoomMember
	var roleStr string
	err := r.pool.QueryRow(ctx,
		`SELECT rm.id, rm.room_id, rm.user_id, rm.role, rm.joined_at, u.username
		 FROM room_members rm
		 JOIN users u ON u.id = rm.user_id
		 WHERE rm.room_id = $1 AND rm.user_id = $2`,
		roomID, userID,
	).Scan(&m.ID, &m.RoomID, &m.UserID, &roleStr, &m.JoinedAt, &m.Username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	m.Role = room.Role(roleStr)
	return &m, nil
}

// ListMembers returns all members of a room, ordered by join time ascending.
// It JOINs against the users table to populate each RoomMember.Username, so
// callers get human-readable display names without a separate
// user-directory lookup.
func (r *RoomRepository) ListMembers(ctx context.Context, roomID string) ([]*room.RoomMember, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT rm.id, rm.room_id, rm.user_id, rm.role, rm.joined_at, u.username
		 FROM room_members rm
		 JOIN users u ON u.id = rm.user_id
		 WHERE rm.room_id = $1
		 ORDER BY rm.joined_at`,
		roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*room.RoomMember
	for rows.Next() {
		var m room.RoomMember
		var roleStr string
		if err := rows.Scan(&m.ID, &m.RoomID, &m.UserID, &roleStr, &m.JoinedAt, &m.Username); err != nil {
			return nil, err
		}
		m.Role = room.Role(roleStr)
		members = append(members, &m)
	}
	return members, rows.Err()
}

// RemoveMember removes a user from a room. It returns domain.ErrNotFound if the membership does not exist.
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

// UpdateMemberRole implements room.RoomRepository.UpdateMemberRole (see its
// GoDoc for the owner-protection and TransferOwnership-serialization
// contract). It runs the owner recheck and the role UPDATE inside a single
// transaction, following the same r.pool.Begin / defer tx.Rollback /
// tx.Commit pattern as Create and TransferOwnership:
//
//  1. `SELECT owner_id FROM rooms WHERE id = $1 FOR UPDATE` takes a row
//     lock on rooms' roomID row -- the same row TransferOwnership's
//     `UPDATE rooms SET owner_id = ... WHERE id = ... AND owner_id = ...`
//     locks, so the two block each other rather than interleaving. Returns
//     domain.ErrNotFound if the room does not exist.
//  2. If the (possibly just-updated, if this call waited out a concurrent
//     TransferOwnership) owner_id equals userID, returns
//     room.ErrOwnerRoleProtected without writing anything.
//  3. Otherwise runs the existing room_members role UPDATE and returns
//     domain.ErrNotFound if it affects zero rows (the membership does not
//     exist).
func (r *RoomRepository) UpdateMemberRole(ctx context.Context, roomID, userID string, role room.Role) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	var ownerID string
	err = tx.QueryRow(ctx, `SELECT owner_id FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if ownerID == userID {
		return room.ErrOwnerRoleProtected
	}

	tag, err := tx.Exec(ctx,
		`UPDATE room_members SET role = $1 WHERE room_id = $2 AND user_id = $3`,
		string(role), roomID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return tx.Commit(ctx)
}

// TransferOwnership atomically updates rooms.owner_id to newOwnerID, sets
// the new owner's room_members.role to master, and sets the previous
// owner's (oldOwnerID) room_members.role to admin, all within a single
// transaction, following the same r.pool.Begin / defer tx.Rollback /
// tx.Commit pattern as Create.
//
// The rooms.owner_id update is itself a compare-and-swap:
// `WHERE id = $2 AND owner_id = $3` guards against a concurrent transfer of
// the same room racing this one -- without the owner_id condition, two
// overlapping TransferOwnership calls (e.g. the current owner double-
// submitting, or a stale client retrying against an already-transferred
// room) could both report success while only one of their intended
// grant/demote pairs actually reflects the room's final owner, leaving
// room_members with an admin who still thinks they're master or vice versa.
// A zero-rows result from this CAS is treated as domain.ErrNotFound (the
// caller's assumed oldOwnerID is stale -- ownership already moved), the
// same as a genuinely missing room.
//
// It returns domain.ErrNotFound — rolling back all writes made so far in the
// transaction — if the rooms CAS update, the new owner's membership update,
// or the previous owner's membership update affects zero rows (i.e. the
// room does not exist, oldOwnerID is no longer the current owner, or either
// user is not already a room member).
func (r *RoomRepository) TransferOwnership(ctx context.Context, roomID, oldOwnerID, newOwnerID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after a successful Commit returns pgx.ErrTxClosed by design; safe to ignore.
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE rooms SET owner_id = $1 WHERE id = $2 AND owner_id = $3`,
		newOwnerID, roomID, oldOwnerID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	tag, err = tx.Exec(ctx,
		`UPDATE room_members SET role = $1 WHERE room_id = $2 AND user_id = $3`,
		string(room.RoleMaster), roomID, newOwnerID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	tag, err = tx.Exec(ctx,
		`UPDATE room_members SET role = $1 WHERE room_id = $2 AND user_id = $3`,
		string(room.RoleAdmin), roomID, oldOwnerID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return tx.Commit(ctx)
}
