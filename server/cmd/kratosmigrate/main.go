// Package main implements cmd/kratosmigrate, a one-off CLI that backfills
// existing SimpleJWT users into Ory Kratos identities as part of the Phase 9
// auth data-plane migration (see docs/tasks/step20.md).
//
// It reads DATABASE_URL and KRATOS_ADMIN_URL from the environment, connects
// to Postgres, selects every users row with kratos_identity_id IS NULL, and
// for each one calls the Kratos Admin API (POST /admin/identities) to create
// a matching identity — reusing the existing argon2id-hashed password_hash
// directly as the identity's pre-hashed credential, since Kratos's admin
// identity-import API accepts PHC-encoded password hashes — then records the
// link via UserRepository.SetKratosIdentityID. Per-user failures are logged
// and skipped rather than aborting the whole batch.
//
// Usage:
//
//	DATABASE_URL=postgres://... KRATOS_ADMIN_URL=http://localhost:4434 \
//	  go run ./cmd/kratosmigrate
//
// or, against the docker-compose stack:
//
//	docker compose run --rm -e DATABASE_URL -e KRATOS_ADMIN_URL api /app/kratosmigrate
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/repository/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		slog.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	adminURL := os.Getenv("KRATOS_ADMIN_URL")
	if adminURL == "" {
		adminURL = "http://localhost:4434"
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, dbURL, time.Hour, 30*time.Minute, time.Minute)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	userRepo := postgres.NewUserRepository(pool)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	users, err := fetchUnlinkedUsers(ctx, pool)
	if err != nil {
		slog.Error("failed to query users pending kratos migration", "error", err)
		os.Exit(1)
	}

	var migrated, failed int
	for _, u := range users {
		if err := migrateUser(ctx, adminURL, httpClient, userRepo, u); err != nil {
			failed++
			slog.Error("failed to migrate user to kratos", "user_id", u.ID, "email", u.Email, "error", err)
			continue
		}
		migrated++
		slog.Info("migrated user to kratos", "user_id", u.ID, "email", u.Email)
	}

	slog.Info("kratosmigrate complete", "total", len(users), "migrated", migrated, "failed", failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// fetchUnlinkedUsers queries every users row with kratos_identity_id IS
// NULL, i.e. every user not yet migrated to (or created directly in) Kratos.
func fetchUnlinkedUsers(ctx context.Context, pool *pgxpool.Pool) ([]*user.User, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, email, username, password_hash, kratos_identity_id, created_at, updated_at
		 FROM users WHERE kratos_identity_id IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*user.User
	for rows.Next() {
		var u user.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.KratosIdentityID, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}
