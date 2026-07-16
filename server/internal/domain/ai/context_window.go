package ai

// defaultFallbackContextWindow is the context window (in tokens) assumed for
// a model that is neither present in a live ai.LLMGateway.ListModels result
// nor in fallbackContextWindows below. It is deliberately conservative --
// small enough that MessageUsecase.assembleAIContext will summarize rather
// than risk overflowing an unknown model's real (possibly much smaller)
// limit.
const defaultFallbackContextWindow = 8000

// fallbackContextWindows hard-codes the context window (in tokens) for
// every model this codebase's provider adapters currently register (see
// llm-gateway/src/adapters/outbound/{openai,anthropic,gemini}/mod.rs), for
// use when a live ai.LLMGateway.ListModels call is unavailable (the LLM
// Gateway is unreachable) or simply does not include the requested model
// (e.g. it was renamed/retired upstream since this table was last updated).
//
// This table is a manually maintained mirror of the provider adapters'
// model catalogs, following the same convention as those adapters' own
// pricing tables: it needs a manual update whenever a model is added to (or
// removed from) an adapter's ListModels response.
var fallbackContextWindows = map[string]int{
	// OpenAI (llm-gateway/src/adapters/outbound/openai/mod.rs)
	"gpt-5.2":    400_000,
	"gpt-5":      272_000,
	"gpt-5-mini": 272_000,
	"o4-mini":    200_000,
	"o3":         200_000,

	// Anthropic (llm-gateway/src/adapters/outbound/anthropic/mod.rs)
	"claude-opus-4-6":   200_000,
	"claude-sonnet-4-6": 200_000,
	"claude-haiku-4-6":  200_000,

	// Gemini (llm-gateway/src/adapters/outbound/gemini/mod.rs)
	"gemini-3-pro":     1_000_000,
	"gemini-3-flash":   1_000_000,
	"gemini-2.5-flash": 1_000_000,
}

// fallbackSupportsImageInput mirrors fallbackContextWindows for the
// SupportsImageInput capability: every model currently registered by this
// codebase's provider adapters is Vision-capable, so every entry here is
// true. It exists as a separate table (rather than folding the two
// concerns into one struct) so a future text-only model can be added to one
// table without disturbing the other, and so ResolveSupportsImageInput's
// "unknown model" default (false) is visibly distinct from "known model,
// no Vision support" (an explicit false entry here).
var fallbackSupportsImageInput = map[string]bool{
	"gpt-5.2":    true,
	"gpt-5":      true,
	"gpt-5-mini": true,
	"o4-mini":    true,
	"o3":         true,

	"claude-opus-4-6":   true,
	"claude-sonnet-4-6": true,
	"claude-haiku-4-6":  true,

	"gemini-3-pro":     true,
	"gemini-3-flash":   true,
	"gemini-2.5-flash": true,
}

// ResolveContextWindow returns modelID's context window in tokens, trying
// three sources in order:
//
//  1. models -- the live result of ai.LLMGateway.ListModels -- if it
//     contains an entry for modelID whose ContextWindow is greater than
//     zero (zero means "the gateway did not report a window", not "no
//     limit"; see ModelInfo.ContextWindow's doc comment).
//  2. fallbackContextWindows[modelID], the hard-coded table above, if
//     modelID is not found in models (or found with a zero ContextWindow).
//  3. defaultFallbackContextWindow, if modelID is in neither source.
//
// Callers (MessageUsecase.assembleAIContext) pass a possibly-nil or
// possibly-stale models slice on a best-effort basis: a ListModels failure
// degrades to skipping straight to sources 2/3 rather than failing the
// whole AI call.
func ResolveContextWindow(models []ModelInfo, modelID string) int {
	for _, m := range models {
		if m.ID == modelID {
			if m.ContextWindow > 0 {
				return m.ContextWindow
			}
			break
		}
	}
	if window, ok := fallbackContextWindows[modelID]; ok {
		return window
	}
	return defaultFallbackContextWindow
}

// ResolveSupportsImageInput reports whether modelID accepts image/Vision
// content parts, trying the same three-source order as
// ResolveContextWindow:
//
//  1. models's live SupportsImageInput field, if modelID is found there.
//  2. fallbackSupportsImageInput[modelID], if modelID is not found in
//     models.
//  3. false, if modelID is in neither source -- a conservative default: an
//     unknown model gets text-only summarization (image parts rendered as
//     placeholders) rather than a doomed Vision call to a model that may not
//     support one.
//
// Unlike ResolveContextWindow, source 1 is trusted even when it reports
// false explicitly (there is no "zero means unknown" ambiguity for a bool
// the way there is for an int window), so a live-metadata model always
// short-circuits here without falling through to the table.
func ResolveSupportsImageInput(models []ModelInfo, modelID string) bool {
	for _, m := range models {
		if m.ID == modelID {
			return m.SupportsImageInput
		}
	}
	return fallbackSupportsImageInput[modelID]
}
