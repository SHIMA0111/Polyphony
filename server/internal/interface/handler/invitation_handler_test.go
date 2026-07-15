package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	domainuser "github.com/SHIMA0111/multi-user-ai/server/internal/domain/user"
	"github.com/SHIMA0111/multi-user-ai/server/internal/testutil/mocks"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
)

// setupInvitationTest wires an InvitationHandler backed by in-memory mocks,
// seeding "room-1" with an admin ("admin-1") and a plain member
// ("member-1"), plus a registered user "bob" (ID "bob-1") who is not yet a
// room member.
func setupInvitationTest() (*echo.Echo, *InvitationHandler, *mocks.RoomRepo, *mocks.InvitationRepo) {
	roomRepo := &mocks.RoomRepo{}
	userRepo := &mocks.UserRepo{}
	invitationRepo := &mocks.InvitationRepo{}

	roomRepo.SeedMember("room-1", "admin-1", "admin")
	roomRepo.SeedMember("room-1", "member-1", "member")
	_ = userRepo.Create(context.Background(), &domainuser.User{ID: "bob-1", Email: "bob@example.com", Username: "bob"})

	uc := invitationusecase.NewInvitationUsecase(invitationRepo, roomRepo, userRepo)
	h := NewInvitationHandler(uc)
	return echo.New(), h, roomRepo, invitationRepo
}

func TestCreateInvitationHandler201(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"member"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "admin-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp InvitationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.InviteCode == "" {
		t.Fatal("expected a non-empty invite code")
	}
	if resp.Status != "pending" {
		t.Fatalf(`expected status "pending", got %q`, resp.Status)
	}
	if resp.InviteeID == nil || *resp.InviteeID != "bob-1" {
		t.Fatalf("expected invitee_id bob-1, got %v", resp.InviteeID)
	}
}

func TestCreateInvitationHandlerInvalidRole400(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"owner"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "admin-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid role, got %d", rec.Code)
	}
}

func TestCreateInvitationHandlerExpiresOutOfRange400(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"role":"member","expires_in_hours":10000}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "admin-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for out-of-range expires_in_hours, got %d", rec.Code)
	}
}

func TestCreateInvitationHandlerForbiddenForNonAdmin(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"member"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "member-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestListForRoomHandler200(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"member"}`))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.SetParamNames("roomId")
	createCtx.SetParamValues("room-1")
	createCtx.Set("user_id", "admin-1")
	if err := h.Create(createCtx); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/rooms/room-1/invitations", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "admin-1")

	if err := h.ListForRoom(c); err != nil {
		t.Fatalf("ListForRoom handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp InvitationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Invitations) != 1 {
		t.Fatalf("expected 1 invitation, got %d", len(resp.Invitations))
	}
}

func TestListMineHandler200(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"member"}`))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.SetParamNames("roomId")
	createCtx.SetParamValues("room-1")
	createCtx.Set("user_id", "admin-1")
	if err := h.Create(createCtx); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/invitations", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "bob-1")

	if err := h.ListMine(c); err != nil {
		t.Fatalf("ListMine handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp InvitationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Invitations) != 1 {
		t.Fatalf("expected 1 invitation for bob-1, got %d", len(resp.Invitations))
	}
}

func TestGetByCodeHandler(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"role":"guest"}`))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.SetParamNames("roomId")
	createCtx.SetParamValues("room-1")
	createCtx.Set("user_id", "admin-1")
	if err := h.Create(createCtx); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	var created InvitationResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal created invitation: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/invitations/by-code/"+created.InviteCode, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("code")
	c.SetParamValues(created.InviteCode)
	c.Set("user_id", "bob-1")

	if err := h.GetByCode(c); err != nil {
		t.Fatalf("GetByCode handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetByCodeHandlerNotFound404(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	req := httptest.NewRequest(http.MethodGet, "/invitations/by-code/does-not-exist", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("code")
	c.SetParamValues("does-not-exist")
	c.Set("user_id", "bob-1")

	if err := h.GetByCode(c); err != nil {
		t.Fatalf("GetByCode handler error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAcceptInvitationHandler200AndRepeat409(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"member"}`))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.SetParamNames("roomId")
	createCtx.SetParamValues("room-1")
	createCtx.Set("user_id", "admin-1")
	if err := h.Create(createCtx); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	var created InvitationResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal created invitation: %v", err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/invitations/"+created.ID+"/accept", nil)
	acceptRec := httptest.NewRecorder()
	acceptCtx := e.NewContext(acceptReq, acceptRec)
	acceptCtx.SetParamNames("invitationId")
	acceptCtx.SetParamValues(created.ID)
	acceptCtx.Set("user_id", "bob-1")

	if err := h.Accept(acceptCtx); err != nil {
		t.Fatalf("Accept handler error: %v", err)
	}
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", acceptRec.Code, acceptRec.Body.String())
	}

	var membership RoomMembershipResponse
	if err := json.Unmarshal(acceptRec.Body.Bytes(), &membership); err != nil {
		t.Fatalf("failed to unmarshal membership: %v", err)
	}
	if membership.UserID != "bob-1" || membership.RoomID != "room-1" || membership.Role != "member" {
		t.Fatalf("unexpected membership response: %+v", membership)
	}

	// A repeat accept must return 409 (already a member).
	repeatRec := httptest.NewRecorder()
	repeatCtx := e.NewContext(acceptReq, repeatRec)
	repeatCtx.SetParamNames("invitationId")
	repeatCtx.SetParamValues(created.ID)
	repeatCtx.Set("user_id", "bob-1")

	if err := h.Accept(repeatCtx); err != nil {
		t.Fatalf("Accept handler error: %v", err)
	}
	if repeatRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 on repeat accept, got %d", repeatRec.Code)
	}
}

func TestRejectInvitationHandler204(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"invitee_username":"bob","role":"member"}`))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.SetParamNames("roomId")
	createCtx.SetParamValues("room-1")
	createCtx.Set("user_id", "admin-1")
	if err := h.Create(createCtx); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	var created InvitationResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal created invitation: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/invitations/"+created.ID+"/reject", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("invitationId")
	c.SetParamValues(created.ID)
	c.Set("user_id", "bob-1")

	if err := h.Reject(c); err != nil {
		t.Fatalf("Reject handler error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestRejectLinkInvitationHandler403(t *testing.T) {
	e, h, _, _ := setupInvitationTest()

	createReq := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations",
		strings.NewReader(`{"role":"guest"}`))
	createReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	createRec := httptest.NewRecorder()
	createCtx := e.NewContext(createReq, createRec)
	createCtx.SetParamNames("roomId")
	createCtx.SetParamValues("room-1")
	createCtx.Set("user_id", "admin-1")
	if err := h.Create(createCtx); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	var created InvitationResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to unmarshal created invitation: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/invitations/"+created.ID+"/reject", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("invitationId")
	c.SetParamValues(created.ID)
	c.Set("user_id", "bob-1")

	if err := h.Reject(c); err != nil {
		t.Fatalf("Reject handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 rejecting a link invitation, got %d", rec.Code)
	}
}
