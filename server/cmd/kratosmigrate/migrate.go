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
// returns a non-2xx status, returns a 2xx response with a missing/empty
// identity id, or if SetKratosIdentityID fails (e.g. because the created
// identity ID collides with an existing link, mapped to
// domain.ErrKratosIdentityAlreadyLinked). Callers are expected to log and
// continue to the next user rather than treat this as fatal for the batch.
//
// If SetKratosIdentityID fails after the identity was successfully created,
// migrateUser first reconciles via userRepo.GetByKratosIdentityID to confirm
// the identity is genuinely unlinked (domain.ErrNotFound) before
// best-effort deleting it (via DELETE {adminURL}/admin/identities/{id}); if
// that lookup instead finds a linked user, or fails ambiguously for any
// other reason, the delete is skipped so a concurrently-created or
// previously-established link is never destroyed. This reconciliation
// keeps a re-run of this tool from accumulating identities that exist in
// Kratos but are never linked to (or migratable by) any local user, without
// risking deletion of an identity another user now legitimately owns. A
// deletion failure (or a skipped deletion) is logged but does not change
// the error returned for the original link failure.
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
		defer cancel()

		// Before deleting the identity we just created, confirm it is truly
		// unlinked: if another user is (or might already be) linked to
		// result.ID, deleting it here could destroy a legitimate link
		// created concurrently, or one this same identity ID was already
		// attached to by a prior run. Only delete when we can positively
		// confirm via ErrNotFound that nothing is linked to it; on any
		// other outcome (a user is linked, or the lookup itself failed) we
		// skip the delete and just surface the original link error.
		if _, getErr := userRepo.GetByKratosIdentityID(cleanupCtx, result.ID); getErr != nil {
			if errors.Is(getErr, domain.ErrNotFound) {
				if delErr := deleteIdentity(cleanupCtx, adminURL, httpClient, result.ID); delErr != nil {
					slog.Error("failed to clean up orphaned kratos identity after link failure",
						"user_id", u.ID, "kratos_identity_id", result.ID, "error", delErr)
				}
			} else {
				slog.Warn("skipping kratos identity cleanup: could not confirm it is unlinked",
					"user_id", u.ID, "kratos_identity_id", result.ID, "error", getErr)
			}
		} else {
			slog.Warn("skipping kratos identity cleanup: identity is linked to a user",
				"user_id", u.ID, "kratos_identity_id", result.ID)
		}

		return linkErr
	}

	return nil
}

// deleteIdentity deletes the Kratos identity identified by identityID via the
// Admin API (DELETE {adminURL}/admin/identities/{id}). Used only to roll back
// an identity migrateUser just created when linking it back to the local
// user subsequently fails, so that failure doesn't leave an orphaned
// identity behind in Kratos.
func deleteIdentity(ctx context.Context, adminURL string, httpClient *http.Client, identityID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, adminURL+"/admin/identities/"+identityID, nil)
	if err != nil {
		return fmt.Errorf("create delete request: %w", err)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send delete request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("kratos admin delete identity failed: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
