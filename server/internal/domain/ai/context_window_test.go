package ai

import "testing"

// TestResolveContextWindow verifies ResolveContextWindow's source
// precedence: non-zero live metadata wins, zero live metadata falls
// through to the fallback table, a model absent from live metadata also
// falls through to the fallback table, and a model in neither source
// returns defaultFallbackContextWindow.
func TestResolveContextWindow(t *testing.T) {
	tests := []struct {
		name    string
		models  []ModelInfo
		modelID string
		want    int
	}{
		{
			name:    "live metadata present and non-zero is used",
			models:  []ModelInfo{{ID: "gpt-5-mini", ContextWindow: 123_456}},
			modelID: "gpt-5-mini",
			want:    123_456,
		},
		{
			name:    "live metadata present but zero falls through to the fallback table",
			models:  []ModelInfo{{ID: "gpt-5-mini", ContextWindow: 0}},
			modelID: "gpt-5-mini",
			want:    fallbackContextWindows["gpt-5-mini"],
		},
		{
			name:    "model in fallback table but absent from live metadata",
			models:  []ModelInfo{{ID: "some-other-model", ContextWindow: 999}},
			modelID: "claude-opus-4-6",
			want:    fallbackContextWindows["claude-opus-4-6"],
		},
		{
			name:    "model in neither source returns the default",
			models:  nil,
			modelID: "totally-unknown-model",
			want:    defaultFallbackContextWindow,
		},
		{
			name:    "nil live metadata still resolves the fallback table",
			models:  nil,
			modelID: "gemini-3-pro",
			want:    fallbackContextWindows["gemini-3-pro"],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveContextWindow(tt.models, tt.modelID)
			if got != tt.want {
				t.Errorf("ResolveContextWindow(%v, %q) = %d, want %d", tt.models, tt.modelID, got, tt.want)
			}
		})
	}
}

// TestResolveSupportsImageInput verifies ResolveSupportsImageInput's source
// precedence: live metadata is trusted as-is (true or false, never
// overridden by the fallback table) whenever the model is present there, a
// model absent from live metadata falls through to the fallback table, and
// a model in neither source returns false.
func TestResolveSupportsImageInput(t *testing.T) {
	tests := []struct {
		name    string
		models  []ModelInfo
		modelID string
		want    bool
	}{
		{
			name:    "live metadata true is used",
			models:  []ModelInfo{{ID: "gpt-5-mini", SupportsImageInput: true}},
			modelID: "gpt-5-mini",
			want:    true,
		},
		{
			name:    "live metadata false is trusted, not overridden by the fallback table",
			models:  []ModelInfo{{ID: "gpt-5-mini", SupportsImageInput: false}},
			modelID: "gpt-5-mini",
			want:    false,
		},
		{
			name:    "model absent from live metadata falls through to the fallback table",
			models:  []ModelInfo{{ID: "some-other-model", SupportsImageInput: false}},
			modelID: "claude-opus-4-6",
			want:    true,
		},
		{
			name:    "model in neither source returns false",
			models:  nil,
			modelID: "totally-unknown-model",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveSupportsImageInput(tt.models, tt.modelID)
			if got != tt.want {
				t.Errorf("ResolveSupportsImageInput(%v, %q) = %v, want %v", tt.models, tt.modelID, got, tt.want)
			}
		})
	}
}
