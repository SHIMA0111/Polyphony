// Package model implements the model-listing use case, delegating to the
// LLM Gateway to discover the set of AI models currently available.
package model

import (
	"context"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/ai"
)

// ModelUsecase provides model-listing business logic. Today it is a thin,
// delegate-only wrapper around ai.LLMGateway.ListModels so that
// ModelHandler depends on the usecase layer rather than the ai.LLMGateway
// port directly, per Clean Architecture. Step 24 will extend it with
// per-room provider/model filtering; no such filtering is added here.
type ModelUsecase struct {
	gateway ai.LLMGateway
}

// NewModelUsecase creates a new ModelUsecase wrapping the given LLM gateway.
func NewModelUsecase(gateway ai.LLMGateway) *ModelUsecase {
	return &ModelUsecase{gateway: gateway}
}

// ListModels returns the available LLM models by delegating to the
// underlying ai.LLMGateway.ListModels. It returns a domain.ErrLLMGateway-
// wrapped error if the gateway call fails; see ai.LLMGateway for details.
func (u *ModelUsecase) ListModels(ctx context.Context) ([]ai.ModelInfo, error) {
	return u.gateway.ListModels(ctx)
}
