package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
)

// kratosCreateIdentityReqDTO is the request body for the Kratos Admin API's
// POST /admin/identities, using schema_id "default" (see
// ory/kratos/identity.schema.json) and a pre-hashed password credential.
type kratosCreateIdentityReqDTO struct {
	SchemaID string `json:"schema_id"`
	Traits   struct {
		Email    string `json:"email"`
		Username string `json:"username"`
	} `json:"traits"`
	Credentials struct {
		Password struct {
			Config struct {
				HashedPassword string `json:"hashed_password"`
			} `json:"config"`
		} `json:"password"`
	} `json:"credentials"`
}

type kratosCreateIdentityRespDTO struct {
	ID string `json:"id"`
}

// migrateUser creates a Kratos identity for u via the Admin API (POST
// {adminURL}/admin/identities), reusing u.PasswordHash — an argon2id PHC
// string produced by interface/auth/simple_jwt.go's hashPassword — directly
// as the identity's hashed_password credential, then links the created
// identity back to the local user via userRepo.SetKratosIdentityID.
//
// It returns an error, leaving u unlinked, if the Admin API call fails,
// returns a non-2xx status, returns a 2xx response whose body decodes but
// carries an empty/missing identity id (which would otherwise silently link
// u to the zero-value Kratos identity), or if SetKratosIdentityID fails
// (e.g. because the created identity ID collides with an existing link,
// mapped to domain.ErrKratosIdentityAlreadyLinked). Callers are expected to
// log and continue to the next user rather than treat this as fatal for the
// batch.
//
// If SetKratosIdentityID fails after the Kratos identity was already
// created, this reconciles before deleting it: it re-reads
// userRepo.GetByKratosIdentityID(result.ID) and only issues the Admin API
// delete when that lookup confirms the identity is genuinely unlinked
// (domain.ErrNotFound). If the lookup instead finds a user — e.g.
// SetKratosIdentityID failed with domain.ErrKratosIdentityAlreadyLinked
// because a concurrent run already linked this identity to someone — or
// fails ambiguously (a non-ErrNotFound error, such as a database outage,
// where it is unknown whether the identity is linked), the delete is
// skipped and the reason logged, so this never deletes a Kratos identity a
// user might already depend on. Either way, a cleanup failure or skip is
// logged but never replaces the original error returned to the caller —
// the original failure is what a caller needs to know to decide
// whether/how to retry this user.
func migrateUser(ctx context.Context, adminURL string, httpClient *http.Client, userRepo user.UserRepository, u *user.User) error {
	reqBody := kratosCreateIdentityReqDTO{SchemaID: "default"}
	reqBody.Traits.Email = u.Email
	reqBody.Traits.Username = u.Username
	reqBody.Credentials.Password.Config.HashedPassword = u.PasswordHash

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, adminURL+"/admin/identities", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("kratos admin create identity failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result kratosCreateIdentityRespDTO
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if result.ID == "" {
		return fmt.Errorf("kratos admin create identity: response missing identity id")
	}

	if err := userRepo.SetKratosIdentityID(ctx, u.ID, result.ID); err != nil {
		linkErr := fmt.Errorf("set kratos identity id: %w", err)

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, lookupErr := userRepo.GetByKratosIdentityID(cleanupCtx, result.ID)
		switch {
		case lookupErr == nil:
			// A user is now linked to this identity (most likely the very
			// SetKratosIdentityID failure above was
			// domain.ErrKratosIdentityAlreadyLinked, or a concurrent run won
			// the race) — deleting it would break that link.
			slog.Warn("skipping kratos identity cleanup: identity is linked to a user",
				"user_id", u.ID, "kratos_identity_id", result.ID)
		case errors.Is(lookupErr, domain.ErrNotFound):
			// Confirmed unlinked: safe to delete the identity this call just
			// created.
			if cleanupErr := deleteKratosIdentity(cleanupCtx, adminURL, httpClient, result.ID); cleanupErr != nil {
				slog.Error("failed to clean up orphaned kratos identity after link failure",
					"user_id", u.ID, "kratos_identity_id", result.ID, "error", cleanupErr)
			}
		default:
			// Ambiguous: we couldn't determine whether the identity is
			// linked (e.g. a database outage), so err on the side of not
			// deleting it.
			slog.Warn("skipping kratos identity cleanup: could not confirm identity is unlinked",
				"user_id", u.ID, "kratos_identity_id", result.ID, "error", lookupErr)
		}
		cancel()

		return linkErr
	}

	return nil
}

// deleteKratosIdentity removes the Kratos identity identified by id via the
// Admin API (DELETE {adminURL}/admin/identities/{id}).
//
// Used by migrateUser to roll back identity creation when the follow-up
// SetKratosIdentityID call fails, so a failed migration attempt never leaves
// behind an identity with no corresponding local-user link.
//
// # Errors
// Returns an error if the request cannot be built or sent, or if the Admin
// API responds with a status other than 204 (already deleted/never existed)
// or 404 (already gone).
func deleteKratosIdentity(ctx context.Context, adminURL string, httpClient *http.Client, id string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, adminURL+"/admin/identities/"+id, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send delete request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kratos admin delete identity failed: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
