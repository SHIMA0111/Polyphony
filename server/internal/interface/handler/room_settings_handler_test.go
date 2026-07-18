package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	domainroom "github.com/SHIMA0111/multi-user-ai/server/internal/domain/room"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

// TestRoomHandlerUpdateSettings covers PATCH /rooms/:roomId/settings: 200
// with the updated ai_provider/ai_model for an admin caller, 403 for a
// member caller, and 400 for a malformed body.
func TestRoomHandlerUpdateSettings(t *testing.T) {
	t.Run("200 admin sets ai_provider and ai_model", func(t *testing.T) {
		repo := &mocks.RoomRepo{}
		uc := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{})
		h := NewRoomHandler(uc)
		e := echo.New()

		created, err := uc.CreateRoom(context.Background(), "user-1", "Test Room", "desc")
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}
		if err := repo.AddMember(context.Background(), &domainroom.RoomMember{
			ID: "m2", RoomID: created.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
		}); err != nil {
			t.Fatalf("AddMember failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPatch, "/rooms/"+created.Room.ID+"/settings",
			strings.NewReader(`{"ai_provider":"anthropic","ai_model":"claude-opus-4"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId")
		c.SetParamValues(created.Room.ID)
		c.Set("user_id", "user-2")

		if err := h.UpdateSettings(c); err != nil {
			t.Fatalf("UpdateSettings error: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp RoomResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if resp.AIProvider == nil || *resp.AIProvider != "anthropic" {
			t.Fatalf("expected ai_provider anthropic, got %v", resp.AIProvider)
		}
		if resp.AIModel == nil || *resp.AIModel != "claude-opus-4" {
			t.Fatalf("expected ai_model claude-opus-4, got %v", resp.AIModel)
		}
	})

	t.Run("403 member caller", func(t *testing.T) {
		repo := &mocks.RoomRepo{}
		uc := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{})
		h := NewRoomHandler(uc)
		e := echo.New()

		created, err := uc.CreateRoom(context.Background(), "user-1", "Test Room", "desc")
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}
		if err := repo.AddMember(context.Background(), &domainroom.RoomMember{
			ID: "m2", RoomID: created.Room.ID, UserID: "user-2", Role: domainroom.RoleMember,
		}); err != nil {
			t.Fatalf("AddMember failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPatch, "/rooms/"+created.Room.ID+"/settings",
			strings.NewReader(`{"ai_model":"claude-opus-4"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId")
		c.SetParamValues(created.Room.ID)
		c.Set("user_id", "user-2")

		if err := h.UpdateSettings(c); err != nil {
			t.Fatalf("UpdateSettings error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("400 malformed body", func(t *testing.T) {
		repo := &mocks.RoomRepo{}
		uc := roomusecase.NewRoomUsecase(repo, &mocks.MessageRepo{}, &mocks.ForkJobRepo{})
		h := NewRoomHandler(uc)
		e := echo.New()

		created, err := uc.CreateRoom(context.Background(), "user-1", "Test Room", "desc")
		if err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPatch, "/rooms/"+created.Room.ID+"/settings",
			strings.NewReader(`{"ai_model": 123}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("roomId")
		c.SetParamValues(created.Room.ID)
		c.Set("user_id", "user-1")

		if err := h.UpdateSettings(c); err != nil {
			t.Fatalf("UpdateSettings error: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}
