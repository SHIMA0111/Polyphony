package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	domainroomfork "github.com/SHIMA0111/multi-user-ai/server/internal/domain/roomfork"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// gatedMessageRepo wraps *mocks.MessageRepo, blocking CountByRoom (the fork
// worker's first call) until proceed is closed, so a test can assert on
// POST /rooms/:roomId/fork's synchronous response before the detached
// background worker it launches can mutate the same Job/Room objects.
type gatedMessageRepo struct {
	*mocks.MessageRepo
	proceed chan struct{}
}

func (g *gatedMessageRepo) CountByRoom(ctx context.Context, roomID string) (int64, error) {
	<-g.proceed
	return g.MessageRepo.CountByRoom(ctx, roomID)
}

// TestRoomHandlerFork covers POST /rooms/:roomId/fork: 202 for a master
// caller (with the response's job.status "pending" and new_room.is_archived
// true) and 403 for a member caller.
func TestRoomHandlerFork(t *testing.T) {
	t.Run("202 master caller", func(t *testing.T) {
		gate := make(chan struct{})
		defer close(gate)

		repo := &mocks.RoomRepo{}
		msgRepo := &gatedMessageRepo{MessageRepo: &mocks.MessageRepo{}, proceed: gate}
		uc := roomusecase.NewRoomUsecase(repo, msgRepo, &mocks.ForkJobRepo{}, nil)
		h := NewRoomHandler(uc)
		e := echo.New()

		created, err := uc.CreateRoom(context.Background(), "user-1", "Source Room", "desc")
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/rooms/"+created.Room.ID+"/fork", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId")
		c.SetParamValues(created.Room.ID)
		c.Set("user_id", "user-1")

		if err := h.Fork(c); err != nil {
			t.Fatalf("Fork error: %v", err)
		}
		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d", rec.Code)
		}

		var resp RoomForkResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.Job.Status != "pending" {
			t.Fatalf("expected job status pending, got %s", resp.Job.Status)
		}
		if !resp.NewRoom.IsArchived {
			t.Fatal("expected new_room.is_archived true")
		}
		if resp.NewRoom.ForkedFromRoomID == nil || *resp.NewRoom.ForkedFromRoomID != created.Room.ID {
			t.Fatalf("expected new_room.forked_from_room_id %s, got %v", created.Room.ID, resp.NewRoom.ForkedFromRoomID)
		}
		if resp.NewRoom.Role != "master" {
			t.Fatalf("expected new_room.role master, got %s", resp.NewRoom.Role)
		}
	})

	t.Run("403 member caller", func(t *testing.T) {
		repo := &mocks.RoomRepo{}
		uc := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{}, nil)
		h := NewRoomHandler(uc)
		e := echo.New()

		created, err := uc.CreateRoom(context.Background(), "user-1", "Source Room", "desc")
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}
		if err := repo.AddMember(context.Background(), &domainroom.RoomMember{
			ID: "m2", RoomID: created.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
		}); err != nil {
			t.Fatalf("AddMember failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/rooms/"+created.Room.ID+"/fork", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId")
		c.SetParamValues(created.Room.ID)
		c.Set("user_id", "user-2")

		if err := h.Fork(c); err != nil {
			t.Fatalf("Fork error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})
}

// TestRoomHandlerGetForkJobStatus covers
// GET /rooms/:roomId/fork-jobs/:jobId: 200 for a member of either the
// source or destination room, and 403 for an outsider.
func TestRoomHandlerGetForkJobStatus(t *testing.T) {
	setup := func() (*echo.Echo, *RoomHandler, *mocks.ForkJobRepo, string) {
		repo := &mocks.RoomRepo{}
		forkJobRepo := &mocks.ForkJobRepo{}
		uc := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, forkJobRepo, nil)
		h := NewRoomHandler(uc)
		e := echo.New()

		repo.SeedMember("source-room", "source-member", "member")
		repo.SeedMember("new-room", "new-room-member", "master")

		now := time.Now()
		job := &domainroomfork.Job{
			ID: "job-1", SourceRoomID: "source-room", NewRoomID: "new-room",
			Status: domainroomfork.StatusRunning, CreatedAt: now, UpdatedAt: now,
		}
		if err := forkJobRepo.Create(context.Background(), job); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		return e, h, forkJobRepo, job.ID
	}

	t.Run("200 source room member", func(t *testing.T) {
		e, h, _, jobID := setup()

		req := httptest.NewRequest(http.MethodGet, "/rooms/source-room/fork-jobs/"+jobID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "jobId")
		c.SetParamValues("source-room", jobID)
		c.Set("user_id", "source-member")

		if err := h.GetForkJobStatus(c); err != nil {
			t.Fatalf("GetForkJobStatus error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("200 new room member", func(t *testing.T) {
		e, h, _, jobID := setup()

		req := httptest.NewRequest(http.MethodGet, "/rooms/new-room/fork-jobs/"+jobID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "jobId")
		c.SetParamValues("new-room", jobID)
		c.Set("user_id", "new-room-member")

		if err := h.GetForkJobStatus(c); err != nil {
			t.Fatalf("GetForkJobStatus error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("403 outsider", func(t *testing.T) {
		e, h, _, jobID := setup()

		req := httptest.NewRequest(http.MethodGet, "/rooms/source-room/fork-jobs/"+jobID, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId", "jobId")
		c.SetParamValues("source-room", jobID)
		c.Set("user_id", "outsider")

		if err := h.GetForkJobStatus(c); err != nil {
			t.Fatalf("GetForkJobStatus error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})
}
