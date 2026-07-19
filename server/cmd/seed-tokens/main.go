// Package main implements cmd/seed-tokens, a small standalone CLI for local
// dev token top-ups (see docs/tasks/step42.md). Automated initial balance
// provisioning is explicitly out of scope for Phase 16 (phases.md), but a
// developer manually granting tokens for local testing is a different,
// supported use case: this CLI looks up a user by email and credits their
// token_balances row via billing.BalanceRepository.CreditAndRecord, reusing
// the same repository code the production billing path uses rather than a
// hand-written SQL script.
//
// Usage:
//
//	DATABASE_URL=postgres://... go run ./cmd/seed-tokens -email user@example.com -amount 100000
//
// or, via the Taskfile wrapper from the repo root:
//
//	task billing:topup -- -email user@example.com -amount 100000
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/billing"
	"github.com/SHIMA0111/multi-user-ai/server/internal/infrastructure/database"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/repository/postgres"
)

// defaultDatabaseURL is the host-reachable equivalent of docker-compose.yml's
// default database configuration, used when DATABASE_URL is unset. This CLI
// runs on the host against the compose-exposed 5432 port, not inside a
// container, so it cannot reuse the in-container "db" hostname other
// services use.
const defaultDatabaseURL = "postgres://polyphony:polyphony@localhost:5432/polyphony?sslmode=disable" //nolint:gosec // dev-only default credential, matches docker-compose.yml's local db service

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	email := flag.String("email", "", "email of the user to credit (required)")
	amount := flag.Int64("amount", 0, "number of tokens to credit, must be > 0 (required)")
	description := flag.String("description", "dev top-up", "description recorded on the token_transactions row")
	flag.Parse()

	if err := run(*email, *amount, *description); err != nil {
		slog.Error("seed-tokens failed", "error", err)
		os.Exit(1)
	}
}

// run performs the top-up: validate flags, connect to the database, look up
// the user by email, and credit their balance. It is factored out of main
// so the exit-code/logging boilerplate stays in one place.
func run(email string, amount int64, description string) error {
	if email == "" {
		return fmt.Errorf("-email is required")
	}
	if amount <= 0 {
		return fmt.Errorf("-amount must be > 0, got %d", amount)
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	ctx := context.Background()
	pool, err := database.NewPool(ctx, dbURL, time.Hour, 30*time.Minute, time.Minute)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	userRepo := postgres.NewUserRepository(pool)
	billingRepo := postgres.NewBillingRepository(pool)

	user, err := userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("no user found with email %q", email)
		}
		return fmt.Errorf("look up user %q: %w", email, err)
	}

	// Lazily create the balance row first: CreditAndRecord requires an
	// existing token_balances row (billing.BalanceRepository's ErrNotFound
	// contract), but a freshly-registered user only gets one on their first
	// balance-touching API call — a fresh e2e/dev user topped up right after
	// registration would otherwise fail with "not found".
	if _, err := billingRepo.GetOrCreateBalance(ctx, user.ID); err != nil {
		return fmt.Errorf("ensure balance row for user %q: %w", email, err)
	}

	txn, err := billingRepo.CreditAndRecord(ctx, user.ID, billing.TransactionTypeCharge, amount, description)
	if err != nil {
		return fmt.Errorf("credit balance for user %q: %w", email, err)
	}

	slog.Info("credited token balance",
		"email", email,
		"user_id", user.ID,
		"amount", amount,
		"new_balance", txn.BalanceAfter,
	)
	fmt.Printf("Credited %d tokens to %s. New balance: %d\n", amount, email, txn.BalanceAfter)
	return nil
}
