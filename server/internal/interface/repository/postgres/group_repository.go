package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/group"
)

// GroupRepository implements the group.GroupRepository interface using PostgreSQL.
type GroupRepository struct {
	pool *pgxpool.Pool
}

// NewGroupRepository creates a new GroupRepository backed by the given connection pool.
func NewGroupRepository(pool *pgxpool.Pool) *GroupRepository {
	return &GroupRepository{pool: pool}
}

// Create persists a new group.
func (r *GroupRepository) Create(ctx context.Context, g *group.Group) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO groups (id, owner_id, name, description, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		g.ID, g.OwnerID, g.Name, g.Description, g.CreatedAt, g.UpdatedAt,
	)
	return err
}

// GetByID retrieves a group by its unique identifier. It returns
// domain.ErrNotFound if the group does not exist.
func (r *GroupRepository) GetByID(ctx context.Context, id string) (*group.Group, error) {
	var g group.Group
	err := r.pool.QueryRow(ctx,
		`SELECT id, owner_id, name, description, created_at, updated_at FROM groups WHERE id = $1`, id,
	).Scan(&g.ID, &g.OwnerID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &g, nil
}

// ListByOwnerID returns all groups owned by the given user, ordered by
// creation time descending.
func (r *GroupRepository) ListByOwnerID(ctx context.Context, ownerID string) ([]*group.Group, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, owner_id, name, description, created_at, updated_at
		 FROM groups
		 WHERE owner_id = $1
		 ORDER BY created_at DESC`, ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*group.Group
	for rows.Next() {
		var g group.Group
		if err := rows.Scan(&g.ID, &g.OwnerID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, &g)
	}
	return groups, rows.Err()
}

// Update updates a group's name, description, and updated_at fields. It
// returns domain.ErrNotFound if the group does not exist.
func (r *GroupRepository) Update(ctx context.Context, g *group.Group) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE groups SET name = $1, description = $2, updated_at = $3 WHERE id = $4`,
		g.Name, g.Description, g.UpdatedAt, g.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a group by its unique identifier, cascading to its
// group_members rows via ON DELETE CASCADE. It returns domain.ErrNotFound
// if the group does not exist.
func (r *GroupRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// AddMember adds a user to a group. It returns domain.ErrAlreadyMember if
// the user is already a member of the group (a concurrent AddMember for the
// same (group_id, user_id) pair losing the group_members_group_id_user_id_key
// unique-constraint race), so callers relying on Postgres for correctness
// under concurrency see the same domain error the usecase layer's
// check-then-insert already returns for the common non-racing case.
func (r *GroupRepository) AddMember(ctx context.Context, member *group.GroupMember) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO group_members (id, group_id, user_id, added_at)
		 VALUES ($1, $2, $3, $4)`,
		member.ID, member.GroupID, member.UserID, member.AddedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation &&
			pgErr.ConstraintName == "group_members_group_id_user_id_key" {
			return domain.ErrAlreadyMember
		}
		return err
	}
	return nil
}

// GetMember retrieves a specific group membership by group ID and user ID.
// It returns domain.ErrNotFound if the membership does not exist.
func (r *GroupRepository) GetMember(ctx context.Context, groupID, userID string) (*group.GroupMember, error) {
	var m group.GroupMember
	err := r.pool.QueryRow(ctx,
		`SELECT id, group_id, user_id, added_at FROM group_members WHERE group_id = $1 AND user_id = $2`,
		groupID, userID,
	).Scan(&m.ID, &m.GroupID, &m.UserID, &m.AddedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// ListMembers returns all members of a group, ordered by added time
// ascending. It JOINs against the users table to populate each member's
// Username, so callers get human-readable display names without a separate
// user-directory lookup.
func (r *GroupRepository) ListMembers(ctx context.Context, groupID string) ([]*group.GroupMemberWithUsername, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT gm.id, gm.group_id, gm.user_id, gm.added_at, u.username
		 FROM group_members gm
		 JOIN users u ON u.id = gm.user_id
		 WHERE gm.group_id = $1
		 ORDER BY gm.added_at`,
		groupID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []*group.GroupMemberWithUsername
	for rows.Next() {
		var m group.GroupMemberWithUsername
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.AddedAt, &m.Username); err != nil {
			return nil, err
		}
		members = append(members, &m)
	}
	return members, rows.Err()
}

// RemoveMember removes a user from a group. It returns domain.ErrNotFound
// if the membership does not exist.
func (r *GroupRepository) RemoveMember(ctx context.Context, groupID, userID string) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`,
		groupID, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
