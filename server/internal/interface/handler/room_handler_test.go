package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	roomusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/room"
)

func setupRoomTest() (*echo.Echo, *RoomHandler) {
	repo := newMockRoomRepoForHandler()
	uc := roomusecase.NewRoomUsecase(repo)
	h := NewRoomHandler(uc)
	e := echo.New()
	return e, h
}

func TestCreateRoomHandler201(t *testing.T) {
	e, h := setupRoomTest()

	req := httptest.NewRequest(http.MethodPost, "/rooms",
		strings.NewReader(`{"name":"Test Room","description":"desc"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestCreateRoomHandler400(t *testing.T) {
	e, h := setupRoomTest()

	req := httptest.NewRequest(http.MethodPost, "/rooms",
		strings.NewReader(`{"name":"","description":"desc"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListRoomsHandler200(t *testing.T) {
	e, h := setupRoomTest()

	req := httptest.NewRequest(http.MethodGet, "/rooms", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "user-1")

	if err := h.List(c); err != nil {
		t.Fatalf("List handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetRoomHandler403(t *testing.T) {
	e, h := setupRoomTest()

	req := httptest.NewRequest(http.MethodGet, "/rooms/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("nonexistent")
	c.Set("user_id", "user-1")

	if err := h.Get(c); err != nil {
		t.Fatalf("Get handler error: %v", err)
	}
	// nonexistent room → membership check fails → forbidden or not found
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("expected 403 or 404, got %d", rec.Code)
	}
}
