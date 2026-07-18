package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/SHIMA0111/multi-user-ai/server/internal/domain"
	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
	"github.com/SHIMA0111/multi-user-ai/server/internal/interface/middleware"
)

// Kept in its own file (rather than room_handler.go) so that adding these
// methods does not create merge conflicts with other same-wave steps that
// touch RoomHandler's existing CRUD methods — mirrors room_settings_handler.go's
// file-isolation convention.

// Fork handles POST /rooms/:roomId/fork. It creates a new, archived room
// that is a structural copy of :roomId's message history, plus a
// room-fork job tracking the background copy (see
// roomusecase.RoomUsecase.ForkRoom). The caller must be at least admin in
// the source room. On success it returns HTTP 202 (accepted — the copy is
// not yet complete) with a RoomForkResponse. It returns HTTP 403 if the
// caller lacks permission and HTTP 404 if the source room does not exist.
func (h *RoomHandler) Fork(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")

	job, newRoom, err := h.usecase.ForkRoom(c.Request().Context(), userID, roomID)
	if err != nil {
		return handleRoomError(c, err)
	}

	// The caller always becomes the new room's sole member with
	// domainroom.RoleMaster (see roomRepo.Create, reused unchanged by
	// ForkRoom), so RoomResponse.Role is always "master" here.
	return c.JSON(http.StatusAccepted, RoomForkResponse{
		Job:     toForkJobResponse(job),
		NewRoom: toRoomResponse(&domainroom.RoomWithRole{Room: newRoom, Role: domainroom.RoleMaster}),
	})
}

// GetForkJobStatus handles GET /rooms/:roomId/fork-jobs/:jobId. It returns
// the current progress/status of a room-fork job (see
// roomusecase.RoomUsecase.GetForkJobStatus). Any member of either the
// source or destination room may poll it. On success it returns HTTP 200
// with a ForkJobResponse. It returns HTTP 403 if the caller belongs to
// neither room and HTTP 404 if the job does not exist or if roomId
// (the URL's path segment) matches neither job.SourceRoomID nor
// job.NewRoomID — the usecase already authorizes access via the job's real
// rooms regardless of what roomId says, so this check is purely REST-contract
// hygiene: it stops /rooms/{any-room-i-can-name}/fork-jobs/{jobId} from
// returning 200 for a job that has nothing to do with that room in the URL.
func (h *RoomHandler) GetForkJobStatus(c echo.Context) error {
	userID := middleware.GetUserID(c)
	roomID := c.Param("roomId")
	jobID := c.Param("jobId")

	job, err := h.usecase.GetForkJobStatus(c.Request().Context(), userID, jobID)
	if err != nil {
		return handleRoomError(c, err)
	}
	if roomID != job.SourceRoomID && roomID != job.NewRoomID {
		return handleRoomError(c, domain.ErrNotFound)
	}

	return c.JSON(http.StatusOK, toForkJobResponse(job))
}

// toForkJobResponse converts a domainroomfork.Job into the JSON-facing
// ForkJobResponse.
func toForkJobResponse(job *domainroomfork.Job) ForkJobResponse {
	return ForkJobResponse{
		ID:             job.ID,
		SourceRoomID:   job.SourceRoomID,
		NewRoomID:      job.NewRoomID,
		Status:         string(job.Status),
		TotalMessages:  job.TotalMessages,
		CopiedMessages: job.CopiedMessages,
		ErrorMessage:   job.ErrorMessage,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
	}
}
