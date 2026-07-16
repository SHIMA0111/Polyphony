package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
// It returns an error, leaving u unlinked, if the Admin API call fails or
// returns a non-2xx status, or if SetKratosIdentityID fails (e.g. because
// the created identity ID collides with an existing link, mapped to
// domain.ErrKratosIdentityAlreadyLinked). Callers are expected to log and
// continue to the next user rather than treat this as fatal for the batch.
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

	if err := userRepo.SetKratosIdentityID(ctx, u.ID, result.ID); err != nil {
		return fmt.Errorf("set kratos identity id: %w", err)
	}

	return nil
}
