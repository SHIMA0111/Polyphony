package postgres

import (
	"context"
	"errors"

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
	tag, err := r.pool.Exec(ctx,
		`UPDATE room_invitations SET status = $1 WHERE id = $2 AND status = $3`,
		string(newStatus), id, string(expectedStatus),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM room_invitations WHERE id = $1)`, id,
	).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return domain.ErrNotFound
	}
	return domain.ErrInvitationNotPending
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
