package message

import "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"

// resolveModel picks the effective model string for an AI request
// (SendAIMessage / RegenerateAIMessage), applying exactly this three-tier
// precedence, highest first:
//
//  1. requestModel — if non-empty, the caller explicitly asked for a
//     specific model on this request, and it always wins.
//  2. rm.AIModel — if requestModel is empty and rm is non-nil with a
//     non-nil, non-empty AIModel, the room's configured default is used
//     (see roomusecase.RoomUsecase.UpdateSettings, which sets this field).
//  3. globalDefault — used only when both of the above are empty; this is
//     the deployment-wide default sourced from Config.DefaultAIModel at
//     MessageUsecase construction time.
//
// resolveModel depends only on domain/room (to read Room.AIModel) and plain
// strings — no echo, no pgx, no config package import — keeping it usable
// from any layer without pulling in infrastructure concerns; the caller is
// responsible for sourcing globalDefault from Config.
func resolveModel(requestModel string, rm *room.Room, globalDefault string) string {
	if requestModel != "" {
		return requestModel
	}
	if rm != nil && rm.AIModel != nil && *rm.AIModel != "" {
		return *rm.AIModel
	}
	return globalDefault
}
