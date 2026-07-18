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
		`INSERT INTO rooms (id, name, description, owner_id, forked_from_room_id, is_archived, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		rm.ID, rm.Name, rm.Description, rm.OwnerID, rm.ForkedFromRoomID, rm.IsArchived, rm.CreatedAt, rm.UpdatedAt,
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
		`SELECT id, name, description, owner_id, ai_context_cutoff_at, ai_provider, ai_model, forked_from_room_id, is_archived, created_at, updated_at FROM rooms WHERE id = $1`, id,
	).Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.AIContextCutoffAt, &rm.AIProvider, &rm.AIModel, &rm.ForkedFromRoomID, &rm.IsArchived, &rm.CreatedAt, &rm.UpdatedAt)
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
		`SELECT r.id, r.name, r.description, r.owner_id, r.ai_context_cutoff_at, r.ai_provider, r.ai_model, r.forked_from_room_id, r.is_archived, r.created_at, r.updated_at
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
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.AIContextCutoffAt, &rm.AIProvider, &rm.AIModel, &rm.ForkedFromRoomID, &rm.IsArchived, &rm.CreatedAt, &rm.UpdatedAt); err != nil {
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
		`SELECT r.id, r.name, r.description, r.owner_id, r.ai_context_cutoff_at, r.ai_provider, r.ai_model, r.forked_from_room_id, r.is_archived, r.created_at, r.updated_at, rm.role
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
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.Description, &rm.OwnerID, &rm.AIContextCutoffAt, &rm.AIProvider, &rm.AIModel, &rm.ForkedFromRoomID, &rm.IsArchived, &rm.CreatedAt, &rm.UpdatedAt, &roleStr); err != nil {
			return nil, err
		}
		result = append(result, &room.RoomWithRole{Room: &rm, Role: room.Role(roleStr)})
	}
	return result, rows.Err()
}

// UpdateDetails implements room.RoomRepository.UpdateDetails: a narrow
// UPDATE touching only name, description, and updated_at. See its GoDoc for
// why this is kept separate from UpdateAIContextCutoff/UpdateAISettings
// rather than a single full-row Update.
func (r *RoomRepository) UpdateDetails(ctx context.Context, roomID, name, description string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET name = $1, description = $2, updated_at = NOW() WHERE id = $3`,
		name, description, roomID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateAIContextCutoff implements room.RoomRepository.UpdateAIContextCutoff:
// a narrow UPDATE touching only ai_context_cutoff_at and updated_at. See
// UpdateDetails's GoDoc for why this is kept separate from a full-row
// update.
func (r *RoomRepository) UpdateAIContextCutoff(ctx context.Context, roomID string, cutoff *time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET ai_context_cutoff_at = $1, updated_at = NOW() WHERE id = $2`,
		cutoff, roomID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateAISettings implements room.RoomRepository.UpdateAISettings: a
// narrow UPDATE touching only ai_provider, ai_model, and updated_at. See
// UpdateDetails's GoDoc for why this is kept separate from a full-row
// update.
func (r *RoomRepository) UpdateAISettings(ctx context.Context, roomID string, aiProvider, aiModel *string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET ai_provider = $1, ai_model = $2, updated_at = NOW() WHERE id = $3`,
		aiProvider, aiModel, roomID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SetArchived flips a room's is_archived flag via a single dedicated
// UPDATE, without loading or rewriting any other column — see
// room.RoomRepository.SetArchived's GoDoc for why this is kept separate
// from Update. It returns domain.ErrNotFound if the room does not exist.
func (r *RoomRepository) SetArchived(ctx context.Context, roomID string, archived bool) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE rooms SET is_archived = $1, updated_at = NOW() WHERE id = $2`,
		archived, roomID,
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
// returns domain.ErrNotFound if the membership does not exist. It JOINs
// against the users table to populate the returned RoomMember.Username,
// exactly as ListMembers does (Step 42's review fix: RoomUsecase.ChangeMemberRole
// returns GetMember's result directly, and a caller-facing MemberResponse
// with an empty username was a visible regression relative to every other
// member-listing endpoint).
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

// UpdateMemberRole updates a single membership's role. It runs inside its
// own transaction that first locks the room's row with `SELECT owner_id
// FROM rooms WHERE id = $1 FOR UPDATE` and rechecks, under that lock,
// whether userID is the room's current owner — returning
// room.ErrOwnerRoleProtected if so — before performing the UPDATE.
//
// This lock-and-recheck closes a race against a concurrent
// TransferOwnership call for the same room: RoomUsecase.ChangeMemberRole
// reads the room's owner via a separate, non-transactional GetByID before
// ever calling this method, so without a lock here a TransferOwnership
// could complete in the window between that read and this write. Two
// directions of the race matter: (1) userID was the owner at
// ChangeMemberRole's check but a concurrent TransferOwnership moved
// ownership away in between — the recheck below then finds userID no
// longer owns the room and proceeds, which is correct; (2) userID was NOT
// the owner at that check but a concurrent TransferOwnership promoted them
// to owner in between — without the lock, this call would silently demote
// the room's brand-new owner out of RoleMaster, leaving the room with zero
// masters. Locking the rooms row here means TransferOwnership's own
// transaction (which writes rooms.owner_id) and this one serialize on that
// row: whichever commits first is fully visible to the other's FOR UPDATE
// read, so the recheck above is always answered against the true current
// owner, never a stale one.
//
// It returns domain.ErrNotFound if the room does not exist (the FOR UPDATE
// SELECT finds no row) or if the membership (roomID, userID) does not exist
// (the UPDATE affects zero rows), and room.ErrOwnerRoleProtected if userID
// is the room's current owner.
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
// tx.Commit pattern as Create. The rooms.owner_id update is a
// compare-and-swap (`WHERE id = $2 AND owner_id = $3`), not a blind write:
// without the `AND owner_id = oldOwnerID` guard, two concurrent
// TransferOwnership calls for the same room (racing on a stale oldOwnerID
// read from two different callers' pre-transaction GetByID) could both
// pass, with the second silently overwriting the first's newOwnerID and
// then demoting ITS newOwnerID back to admin — leaving the room with the
// wrong owner and, on the resulting role-update mismatch, potentially zero
// masters. It returns domain.ErrNotFound — rolling back all writes made so
// far in the transaction — if the rooms CAS update finds oldOwnerID is no
// longer the current owner (i.e. the room does not exist, or a concurrent
// transfer already moved ownership away from oldOwnerID), or if the new
// owner's membership update or the previous owner's membership update
// affects zero rows (i.e. either user is not already a room member).
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
