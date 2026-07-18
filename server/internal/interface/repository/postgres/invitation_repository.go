package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/invitation"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// InvitationRepository implements the invitation.InvitationRepository
// interface using PostgreSQL.
type InvitationRepository struct {
	pool *pgxpool.Pool
}

// NewInvitationRepository creates a new InvitationRepository backed by the
// given connection pool.
func NewInvitationRepository(pool *pgxpool.Pool) *InvitationRepository {
	return &InvitationRepository{pool: pool}
}

// Create persists a new invitation. It returns
// invitation.ErrInviteCodeConflict if inv.InviteCode collides with an
// existing invitation's code.
func (r *InvitationRepository) Create(ctx context.Context, inv *invitation.Invitation) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO room_invitations (id, room_id, inviter_id, invitee_id, invite_code, role, status, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		inv.ID, inv.RoomID, inv.InviterID, inv.InviteeID, inv.InviteCode,
		string(inv.Role), string(inv.Status), inv.ExpiresAt, inv.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
			pgErr.ConstraintName == "room_invitations_invite_code_unique" {
			return invitation.ErrInviteCodeConflict
		}
		return err
	}
	return nil
}

// GetByID retrieves an invitation by its ID. It returns domain.ErrNotFound
// if the invitation does not exist.
func (r *InvitationRepository) GetByID(ctx context.Context, id string) (*invitation.Invitation, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, room_id, inviter_id, invitee_id, invite_code, role, status, expires_at, created_at
		 FROM room_invitations WHERE id = $1`, id,
	)
	return scanInvitation(row)
}

// GetByCode retrieves an invitation by its unique invite code. It returns
// domain.ErrNotFound if no invitation with that code exists.
func (r *InvitationRepository) GetByCode(ctx context.Context, code string) (*invitation.Invitation, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, room_id, inviter_id, invitee_id, invite_code, role, status, expires_at, created_at
		 FROM room_invitations WHERE invite_code = $1`, code,
	)
	return scanInvitation(row)
}

// GetPendingByRoomAndInvitee retrieves the pending, username-targeted
// invitation for the given (roomID, inviteeID) pair. It returns
// domain.ErrNotFound if no such invitation exists.
func (r *InvitationRepository) GetPendingByRoomAndInvitee(ctx context.Context, roomID, inviteeID string) (*invitation.Invitation, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, room_id, inviter_id, invitee_id, invite_code, role, status, expires_at, created_at
		 FROM room_invitations
		 WHERE room_id = $1 AND invitee_id = $2 AND status = $3`,
		roomID, inviteeID, string(invitation.StatusPending),
	)
	return scanInvitation(row)
}

// ListByRoomID returns all invitations created for the given room, ordered
// by creation time descending.
func (r *InvitationRepository) ListByRoomID(ctx context.Context, roomID string) ([]*invitation.Invitation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, room_id, inviter_id, invitee_id, invite_code, role, status, expires_at, created_at
		 FROM room_invitations WHERE room_id = $1 ORDER BY created_at DESC`, roomID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInvitations(rows)
}

// ListPendingByInviteeID returns all pending invitations targeted at the
// given invitee, ordered by creation time descending.
func (r *InvitationRepository) ListPendingByInviteeID(ctx context.Context, inviteeID string) ([]*invitation.Invitation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, room_id, inviter_id, invitee_id, invite_code, role, status, expires_at, created_at
		 FROM room_invitations
		 WHERE invitee_id = $1 AND status = $2
		 ORDER BY created_at DESC`,
		inviteeID, string(invitation.StatusPending),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInvitations(rows)
}

// roomMembersUniqueConstraint is the name of room_members' UNIQUE(room_id,
// user_id) constraint (see schema.sql), checked by isAlreadyMemberConflict
// to translate a concurrent duplicate-membership insert into
// domain.ErrAlreadyMember.
const roomMembersUniqueConstraint = "room_members_room_id_user_id_key"

// isAlreadyMemberConflict reports whether err is a unique-constraint
// violation on room_members' (room_id, user_id) pair.
func isAlreadyMemberConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == roomMembersUniqueConstraint
}

// sqlExecutor is the minimal interface shared by *pgxpool.Pool and an open
// pgx.Tx, letting insertRoomMember and updateStatusCAS run against either
// -- the pool for their standalone single-statement use (UpdateStatus), or
// a shared transaction (AcceptTx), without duplicating the SQL.
type sqlExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// insertRoomMember inserts member into room_members using exec (either the
// shared *pgxpool.Pool or an open pgx.Tx), translating a unique-constraint
// violation on (room_id, user_id) into domain.ErrAlreadyMember rather than
// returning the raw pgconn error.
func insertRoomMember(ctx context.Context, exec sqlExecutor, member *domainroom.RoomMember) error {
	_, err := exec.Exec(ctx,
		`INSERT INTO room_members (id, room_id, user_id, role, joined_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		member.ID, member.RoomID, member.UserID, string(member.Role), member.JoinedAt,
	)
	if err != nil && isAlreadyMemberConflict(err) {
		return domain.ErrAlreadyMember
	}
	return err
}

// updateStatusCAS runs UpdateStatus's compare-and-swap UPDATE using exec
// (either the pool or an open tx) and translates its RowsAffected into the
// domain.ErrNotFound / domain.ErrInvitationNotPending contract documented on
// invitation.InvitationRepository.UpdateStatus. existsCheck is used only on
// the RowsAffected == 0 path, to distinguish "no such invitation" from
// "invitation exists but is not in expectedStatus" -- it is passed in
// (rather than always querying via r.pool) so AcceptTx can reuse the same
// tx for that follow-up read instead of issuing it against the pool.
func updateStatusCAS(
	ctx context.Context,
	exec sqlExecutor,
	existsCheck func(ctx context.Context, id string) (bool, error),
	id string,
	status, expectedStatus invitation.Status,
) error {
	tag, err := exec.Exec(ctx,
		`UPDATE room_invitations SET status = $1 WHERE id = $2 AND status = $3`,
		string(status), id, string(expectedStatus),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	exists, err := existsCheck(ctx, id)
	if err != nil {
		return err
	}
	if !exists {
		return domain.ErrNotFound
	}
	return domain.ErrInvitationNotPending
}

// UpdateStatus performs a compare-and-swap status transition, per
// invitation.InvitationRepository's UpdateStatus GoDoc: it only updates the
// invitation's status if its current status still equals expectedStatus,
// atomically via `WHERE id = $2 AND status = $3`.
//
// If no row is affected, this distinguishes the two possible causes with a
// follow-up read: domain.ErrNotFound if id does not exist at all, or
// domain.ErrInvitationNotPending if it exists but its status no longer
// equals expectedStatus (a transition conflict -- e.g. a concurrent
// accept/reject already changed it).
func (r *InvitationRepository) UpdateStatus(ctx context.Context, id string, newStatus, expectedStatus invitation.Status) error {
	return updateStatusCAS(ctx, r.pool, r.exists, id, newStatus, expectedStatus)
}

// exists reports whether an invitation with the given id exists, used by
// updateStatusCAS to distinguish a missing row from a CAS mismatch after a
// zero-RowsAffected UPDATE.
func (r *InvitationRepository) exists(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM room_invitations WHERE id = $1)`, id).Scan(&exists)
	return exists, err
}

// AcceptTx implements invitation.InvitationRepository.AcceptTx (see its
// GoDoc for the atomicity guarantee and error contract). It opens a single
// database transaction and validates the invitation's pending status
// atomically inside it, in both modes, before ever inserting into
// room_members: when transitionStatus is true (a username-targeted
// invitation), it runs UpdateStatus's CAS UPDATE within the tx and only
// proceeds if that transition succeeds; when transitionStatus is false (a
// reusable link invitation, which never changes status), it instead locks
// the invitation row with `SELECT ... FOR UPDATE` and rejects the accept if
// its status is no longer StatusPending -- closing a TOCTOU window where a
// revoke or expiry landing after the usecase's own pre-check read, but
// before this call, would otherwise still admit the member. Either way,
// member is only inserted once that check passes, before committing -- so
// the status transition (or status re-check) and the membership insert
// either both take effect or neither does.
func (r *InvitationRepository) AcceptTx(ctx context.Context, invitationID string, expectedStatus invitation.Status, transitionStatus bool, member *domainroom.RoomMember) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	// Safe no-op after a successful Commit below.
	defer func() { _ = tx.Rollback(ctx) }()

	if transitionStatus {
		existsCheck := func(ctx context.Context, id string) (bool, error) {
			var exists bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM room_invitations WHERE id = $1)`, id).Scan(&exists)
			return exists, err
		}
		if err := updateStatusCAS(ctx, tx, existsCheck, invitationID, invitation.StatusAccepted, expectedStatus); err != nil {
			return err
		}
	} else {
		// Reusable link invitations never run the CAS above, so lock and
		// re-check the row's status here instead: without this, a revoke
		// or expiry sweep landing after the usecase's own pre-check read
		// but before this call would still let the accept through.
		var statusStr string
		err := tx.QueryRow(ctx, `SELECT status FROM room_invitations WHERE id = $1 FOR UPDATE`, invitationID).Scan(&statusStr)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if invitation.Status(statusStr) != invitation.StatusPending {
			return domain.ErrInvitationNotPending
		}
	}

	if err := insertRoomMember(ctx, tx, member); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// invitationRow is the minimal interface shared by pgx.Row and pgx.Rows,
// letting scanInvitation be reused by both single-row and multi-row queries.
type invitationRow interface {
	Scan(dest ...any) error
}

// scanInvitation scans a single room_invitations row into an
// *invitation.Invitation, translating pgx.ErrNoRows into domain.ErrNotFound.
func scanInvitation(row invitationRow) (*invitation.Invitation, error) {
	var inv invitation.Invitation
	var roleStr, statusStr string
	err := row.Scan(&inv.ID, &inv.RoomID, &inv.InviterID, &inv.InviteeID, &inv.InviteCode,
		&roleStr, &statusStr, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	inv.Role = domainroom.Role(roleStr)
	inv.Status = invitation.Status(statusStr)
	return &inv, nil
}

// scanInvitations scans every remaining row of rows into a slice of
// *invitation.Invitation.
func scanInvitations(rows pgx.Rows) ([]*invitation.Invitation, error) {
	var result []*invitation.Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}
