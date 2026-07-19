package message

import (
	"testing"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
)

// strPtr returns a pointer to v, for building *string test fixtures inline.
func strPtr(v string) *string { return &v }

// TestResolveModel table-drives all three precedence tiers resolveModel
// implements: request > room.AIModel > globalDefault.
func TestResolveModel(t *testing.T) {
	tests := []struct {
		name          string
		requestModel  string
		rm            *room.Room
		globalDefault string
		want          string
	}{
		{
			name:          "request wins over room and global default",
			requestModel:  "request-model",
			rm:            &room.Room{AIModel: strPtr("room-model")},
			globalDefault: "global-model",
			want:          "request-model",
		},
		{
			name:          "room wins over global default when request is empty",
			requestModel:  "",
			rm:            &room.Room{AIModel: strPtr("room-model")},
			globalDefault: "global-model",
			want:          "room-model",
		},
		{
			name:          "global default used when request and room are both empty",
			requestModel:  "",
			rm:            &room.Room{AIModel: nil},
			globalDefault: "global-model",
			want:          "global-model",
		},
		{
			name:          "global default used when room's AIModel is an empty string",
			requestModel:  "",
			rm:            &room.Room{AIModel: strPtr("")},
			globalDefault: "global-model",
			want:          "global-model",
		},
		{
			name:          "global default used when room is nil",
			requestModel:  "",
			rm:            nil,
			globalDefault: "global-model",
			want:          "global-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveModel(tt.requestModel, tt.rm, tt.globalDefault)
			if got != tt.want {
				t.Fatalf("resolveModel(%q, %+v, %q) = %q, want %q",
					tt.requestModel, tt.rm, tt.globalDefault, got, tt.want)
			}
		})
	}
}
