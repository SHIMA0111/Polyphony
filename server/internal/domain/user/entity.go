// Package user defines the user entity and its repository port.
package user

import "time"

// User represents a registered user in the system.
type User struct {
	ID           string
	Email        string
	Username     string
	PasswordHash string
	// KratosIdentityID is the linked Ory Kratos identity ID (Phase 9), or nil
	// if this user has not yet been created/migrated in Kratos. It is set by
	// KratosAuthService.Register/Login (self-heal path) or backfilled by the
	// cmd/kratosmigrate one-off tool.
	KratosIdentityID *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
