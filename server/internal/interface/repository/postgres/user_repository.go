package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// UserRepository implements the user.UserRepository interface using PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository backed by the given connection pool.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create persists a new user. It returns domain.ErrEmailAlreadyExists or
// domain.ErrUsernameAlreadyExists if the email or username is already taken.
func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, username, password_hash, kratos_identity_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Email, u.Username, u.PasswordHash, u.KratosIdentityID, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return mapUserUniqueViolation(err)
	}
	return nil
}

// GetByID retrieves a user by their unique identifier. It returns domain.ErrNotFound if the user does not exist.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, kratos_identity_id, created_at, updated_at FROM users WHERE id = $1`, id))
}

// GetByEmail retrieves a user by their email address. It returns domain.ErrNotFound if no user matches.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, kratos_identity_id, created_at, updated_at FROM users WHERE email = $1`, email))
}

// GetByUsername retrieves a user by their username. It returns domain.ErrNotFound if no user matches.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, kratos_identity_id, created_at, updated_at FROM users WHERE username = $1`, username))
}

// GetByKratosIdentityID retrieves the user linked to the given Ory Kratos
// identity ID. It returns domain.ErrNotFound if no user is linked to that identity.
func (r *UserRepository) GetByKratosIdentityID(ctx context.Context, kratosIdentityID string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, kratos_identity_id, created_at, updated_at FROM users WHERE kratos_identity_id = $1`,
		kratosIdentityID))
}

// SetKratosIdentityID links the user identified by userID to the given Ory
// Kratos identity ID. It returns domain.ErrNotFound if userID does not exist,
// or domain.ErrKratosIdentityAlreadyLinked if kratosIdentityID is already
// linked to a different user.
func (r *UserRepository) SetKratosIdentityID(ctx context.Context, userID, kratosIdentityID string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET kratos_identity_id = $1, updated_at = NOW() WHERE id = $2`,
		kratosIdentityID, userID,
	)
	if err != nil {
		return mapUserUniqueViolation(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Update updates the email, username, password hash, and updated_at fields of a user. It returns domain.ErrNotFound if the user does not exist.
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET email = $1, username = $2, password_hash = $3, updated_at = $4 WHERE id = $5`,
		u.Email, u.Username, u.PasswordHash, u.UpdatedAt, u.ID,
	)
	if err != nil {
		return mapUserUniqueViolation(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a user by their unique identifier. It returns domain.ErrNotFound if the user does not exist.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) scanUser(row pgx.Row) (*user.User, error) {
	var u user.User
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.KratosIdentityID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// mapUserUniqueViolation maps a Postgres unique-constraint violation on the
// users table to the corresponding domain sentinel error
// (domain.ErrEmailAlreadyExists, domain.ErrUsernameAlreadyExists, or
// domain.ErrKratosIdentityAlreadyLinked), based on the violated constraint
// name. If err is not a recognized unique-violation, it is returned unchanged.
func mapUserUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		switch pgErr.ConstraintName {
		case "users_email_unique":
			return domain.ErrEmailAlreadyExists
		case "users_username_unique":
			return domain.ErrUsernameAlreadyExists
		case "users_kratos_identity_id_unique":
			return domain.ErrKratosIdentityAlreadyLinked
		}
	}
	return err
}
