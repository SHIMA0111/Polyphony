package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// UserRepository implements user.UserRepository using PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create persists a new user.
func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, username, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		u.ID, u.Email, u.Username, u.PasswordHash, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			if pgErr.ConstraintName == "users_email_key" {
				return domain.ErrEmailAlreadyExists
			}
			if pgErr.ConstraintName == "users_username_key" {
				return domain.ErrUsernameAlreadyExists
			}
		}
		return err
	}
	return nil
}

// GetByID retrieves a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, created_at, updated_at FROM users WHERE id = $1`, id))
}

// GetByEmail retrieves a user by email.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, created_at, updated_at FROM users WHERE email = $1`, email))
}

// GetByUsername retrieves a user by username.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*user.User, error) {
	return r.scanUser(r.pool.QueryRow(ctx,
		`SELECT id, email, username, password_hash, created_at, updated_at FROM users WHERE username = $1`, username))
}

// Update updates user fields.
func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET email = $1, username = $2, password_hash = $3, updated_at = $4 WHERE id = $5`,
		u.Email, u.Username, u.PasswordHash, u.UpdatedAt, u.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes a user by ID.
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
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}
