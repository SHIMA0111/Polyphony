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

func setupRoomTest() (*echo.Echo, *RoomHandler) {
	repo := &mocks.RoomRepo{}
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

	var resp RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Role != "master" {
		t.Fatalf(`expected role "master" for the room creator, got %q`, resp.Role)
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

func TestGetRoomHandlerIncludesRole(t *testing.T) {
	repo := &mocks.RoomRepo{}
	uc := roomusecase.NewRoomUsecase(repo)
	h := NewRoomHandler(uc)
	e := echo.New()
	ctx := e.NewContext(httptest.NewRequest(http.MethodPost, "/rooms", nil), httptest.NewRecorder())

	rwr, err := uc.CreateRoom(ctx.Request().Context(), "user-1", "Test Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/rooms/"+rwr.Room.ID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues(rwr.Room.ID)
	c.Set("user_id", "user-1")

	if err := h.Get(c); err != nil {
		t.Fatalf("Get handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Role != "master" {
		t.Fatalf(`expected role "master", got %q`, resp.Role)
	}
}

// setupRoomWithMember creates a room owned by "user-1" and adds "user-2"
// with the given role, returning the handler, repo, and room ID for use by
// member-management handler tests.
func setupRoomWithMember(t *testing.T, memberRole domainroom.Role) (*RoomHandler, *mocks.RoomRepo, string) {
	t.Helper()
	repo := &mocks.RoomRepo{}
	uc := roomusecase.NewRoomUsecase(repo)
	h := NewRoomHandler(uc)

	rwr, err := uc.CreateRoom(context.Background(), "user-1", "Test Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if err := repo.AddMember(context.Background(), &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: memberRole,
	}); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}
	return h, repo, rwr.Room.ID
}

func TestListMembersHandler200(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleReader)

	req := httptest.NewRequest(http.MethodGet, "/rooms/"+roomID+"/members", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues(roomID)
	c.Set("user_id", "user-2")

	if err := h.ListMembers(c); err != nil {
		t.Fatalf("ListMembers handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp MemberListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(resp.Members))
	}
}

func TestLeaveHandler204(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleMember)

	req := httptest.NewRequest(http.MethodDelete, "/rooms/"+roomID+"/members/user-2", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-2")
	c.Set("user_id", "user-2")

	if err := h.Leave(c); err != nil {
		t.Fatalf("Leave handler error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestLeaveHandler403WhenTargetNotCaller(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleMember)

	req := httptest.NewRequest(http.MethodDelete, "/rooms/"+roomID+"/members/user-2", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-2")
	c.Set("user_id", "user-3") // caller != target

	if err := h.Leave(c); err != nil {
		t.Fatalf("Leave handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestLeaveHandler409ForOwner(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleMember)

	req := httptest.NewRequest(http.MethodDelete, "/rooms/"+roomID+"/members/user-1", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-1")
	c.Set("user_id", "user-1") // owner leaving

	if err := h.Leave(c); err != nil {
		t.Fatalf("Leave handler error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestChangeRoleHandler200(t *testing.T) {
	// An admin (user-2) changes a third member's (user-3) role. Note the
	// success case can't target the owner (user-1) — that's covered
	// separately by TestChangeRoleHandler409ForOwnerTarget.
	repo := &mocks.RoomRepo{}
	uc := roomusecase.NewRoomUsecase(repo)
	h := NewRoomHandler(uc)
	rwr, err := uc.CreateRoom(context.Background(), "user-1", "Test Room", "desc")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if err := repo.AddMember(context.Background(), &domainroom.RoomMember{
		ID: "m2", RoomID: rwr.Room.ID, UserID: "user-2", Role: domainroom.RoleAdmin,
	}); err != nil {
		t.Fatalf("AddMember(user-2) failed: %v", err)
	}
	if err := repo.AddMember(context.Background(), &domainroom.RoomMember{
		ID: "m3", RoomID: rwr.Room.ID, UserID: "user-3", Role: domainroom.RoleMember,
	}); err != nil {
		t.Fatalf("AddMember(user-3) failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+rwr.Room.ID+"/members/user-3/role",
		strings.NewReader(`{"role":"guest"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(rwr.Room.ID, "user-3")
	c.Set("user_id", "user-2")

	if err := h.ChangeRole(c); err != nil {
		t.Fatalf("ChangeRole handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp MemberResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Role != "guest" {
		t.Fatalf(`expected role "guest", got %q`, resp.Role)
	}
}

func TestChangeRoleHandler400InvalidRole(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleAdmin)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/members/user-2/role",
		strings.NewReader(`{"role":"nonsense"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-2")
	c.Set("user_id", "user-1")

	if err := h.ChangeRole(c); err != nil {
		t.Fatalf("ChangeRole handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChangeRoleHandler400MasterNotGrantable(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleAdmin)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/members/user-2/role",
		strings.NewReader(`{"role":"master"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-2")
	c.Set("user_id", "user-1")

	if err := h.ChangeRole(c); err != nil {
		t.Fatalf("ChangeRole handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestChangeRoleHandler403InsufficientRole(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleMember)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/members/user-1/role",
		strings.NewReader(`{"role":"guest"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-1")
	c.Set("user_id", "user-2") // plain member lacks ActionManageMembers

	if err := h.ChangeRole(c); err != nil {
		t.Fatalf("ChangeRole handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestChangeRoleHandler409ForOwnerTarget(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleAdmin)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/members/user-1/role",
		strings.NewReader(`{"role":"guest"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId", "userId")
	c.SetParamValues(roomID, "user-1")
	c.Set("user_id", "user-2") // admin targeting the owner user-1

	if err := h.ChangeRole(c); err != nil {
		t.Fatalf("ChangeRole handler error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestTransferOwnershipHandler200(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleMember)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/owner",
		strings.NewReader(`{"new_owner_id":"user-2"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues(roomID)
	c.Set("user_id", "user-1")

	if err := h.TransferOwnership(c); err != nil {
		t.Fatalf("TransferOwnership handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp RoomResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.OwnerID != "user-2" {
		t.Fatalf("expected owner_id user-2, got %s", resp.OwnerID)
	}
}

func TestTransferOwnershipHandler400MissingNewOwnerID(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleMember)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/owner",
		strings.NewReader(`{"new_owner_id":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues(roomID)
	c.Set("user_id", "user-1")

	if err := h.TransferOwnership(c); err != nil {
		t.Fatalf("TransferOwnership handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTransferOwnershipHandler403NonOwner(t *testing.T) {
	h, _, roomID := setupRoomWithMember(t, domainroom.RoleAdmin)

	req := httptest.NewRequest(http.MethodPatch, "/rooms/"+roomID+"/owner",
		strings.NewReader(`{"new_owner_id":"user-1"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues(roomID)
	c.Set("user_id", "user-2") // non-owner caller

	if err := h.TransferOwnership(c); err != nil {
		t.Fatalf("TransferOwnership handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
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
