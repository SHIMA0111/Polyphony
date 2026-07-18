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
	groupusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/group"
	invitationusecase "github.com/SHIMA0111/multi-user-ai/server/internal/usecase/invitation"
)

// setupGroupTest wires a GroupHandler backed by in-memory mocks, seeding
// "room-1" with an admin ("owner-1") and a plain member ("member-1"), and
// registered users "owner-1", "bob" (ID "bob-1"), and "carol" (ID
// "carol-1"), where "bob" and "carol" are not yet room members.
func setupGroupTest() (*echo.Echo, *GroupHandler, *mocks.GroupRepo, *mocks.RoomRepo, *mocks.InvitationRepo) {
	groupRepo := &mocks.GroupRepo{}
	roomRepo := &mocks.RoomRepo{}
	userRepo := &mocks.UserRepo{}
	invitationRepo := &mocks.InvitationRepo{}

	roomRepo.SeedMember("room-1", "owner-1", "admin")
	roomRepo.SeedMember("room-1", "member-1", "member")

	ctx := context.Background()
	_ = userRepo.Create(ctx, &domainuser.User{ID: "owner-1", Email: "owner@example.com", Username: "owner"})
	_ = userRepo.Create(ctx, &domainuser.User{ID: "bob-1", Email: "bob@example.com", Username: "bob"})
	_ = userRepo.Create(ctx, &domainuser.User{ID: "carol-1", Email: "carol@example.com", Username: "carol"})

	groupRepo.Usernames = map[string]string{
		"owner-1": "owner",
		"bob-1":   "bob",
		"carol-1": "carol",
	}

	invitationUC := invitationusecase.NewInvitationUsecase(invitationRepo, roomRepo, userRepo)
	uc := groupusecase.NewGroupUsecase(groupRepo, userRepo, invitationUC)
	h := NewGroupHandler(uc)
	return echo.New(), h, groupRepo, roomRepo, invitationRepo
}

func TestCreateGroupHandler201(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()

	req := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"name":"Team","description":"my team"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "owner-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Name != "Team" || resp.OwnerID != "owner-1" {
		t.Fatalf("unexpected group response: %+v", resp)
	}
}

func TestCreateGroupHandlerMissingName400(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()

	req := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"description":"no name"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "owner-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestCreateGroupHandlerNameTooLong400 verifies that a name exceeding the
// groups.name VARCHAR(255) column limit is rejected with HTTP 400 by the
// handler, rather than reaching the usecase/repository and surfacing as an
// HTTP 500 database error.
func TestCreateGroupHandlerNameTooLong400(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()

	tooLong := strings.Repeat("a", maxGroupNameLength+1)
	req := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"name":"`+tooLong+`"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", "owner-1")

	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUpdateGroupHandlerNameTooLong400 mirrors
// TestCreateGroupHandlerNameTooLong400 for PUT /groups/:groupId.
func TestUpdateGroupHandlerNameTooLong400(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	tooLong := strings.Repeat("a", maxGroupNameLength+1)
	req := httptest.NewRequest(http.MethodPut, "/groups/"+groupID, strings.NewReader(`{"name":"`+tooLong+`"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	c.Set("user_id", "owner-1")

	if err := h.Update(c); err != nil {
		t.Fatalf("Update handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func createTestGroup(t *testing.T, e *echo.Echo, h *GroupHandler, ownerID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/groups", strings.NewReader(`{"name":"Team","description":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("user_id", ownerID)
	if err := h.Create(c); err != nil {
		t.Fatalf("Create handler error: %v", err)
	}
	var resp GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	return resp.ID
}

func TestGetGroupHandlerForbiddenForNonOwner(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	req := httptest.NewRequest(http.MethodGet, "/groups/"+groupID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	c.Set("user_id", "bob-1")

	if err := h.Get(c); err != nil {
		t.Fatalf("Get handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestGetGroupHandlerNotFound(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()

	req := httptest.NewRequest(http.MethodGet, "/groups/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues("nonexistent")
	c.Set("user_id", "owner-1")

	if err := h.Get(c); err != nil {
		t.Fatalf("Get handler error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAddGroupMemberHandler(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"bob"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	c.Set("user_id", "owner-1")

	if err := h.AddMember(c); err != nil {
		t.Fatalf("AddMember handler error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp GroupMemberResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Username != "bob" || resp.UserID != "bob-1" {
		t.Fatalf("unexpected member response: %+v", resp)
	}
}

func TestAddGroupMemberHandlerUnknownUsername404(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"nonexistent"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	c.Set("user_id", "owner-1")

	if err := h.AddMember(c); err != nil {
		t.Fatalf("AddMember handler error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAddGroupMemberHandlerDuplicate409(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	addBob := func() int {
		req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"bob"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("groupId")
		c.SetParamValues(groupID)
		c.Set("user_id", "owner-1")
		if err := h.AddMember(c); err != nil {
			t.Fatalf("AddMember handler error: %v", err)
		}
		return rec.Code
	}

	if code := addBob(); code != http.StatusCreated {
		t.Fatalf("expected 201 on first add, got %d", code)
	}
	if code := addBob(); code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate add, got %d", code)
	}
}

func TestListGroupMembersHandler(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	for _, username := range []string{"bob", "carol"} {
		req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"`+username+`"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("groupId")
		c.SetParamValues(groupID)
		c.Set("user_id", "owner-1")
		if err := h.AddMember(c); err != nil {
			t.Fatalf("AddMember handler error: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/groups/"+groupID+"/members", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	c.Set("user_id", "owner-1")

	if err := h.ListMembers(c); err != nil {
		t.Fatalf("ListMembers handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp GroupMemberListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(resp.Members))
	}
}

func TestRemoveGroupMemberHandler(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	addReq := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"bob"}`))
	addReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	addRec := httptest.NewRecorder()
	addC := e.NewContext(addReq, addRec)
	addC.SetParamNames("groupId")
	addC.SetParamValues(groupID)
	addC.Set("user_id", "owner-1")
	if err := h.AddMember(addC); err != nil {
		t.Fatalf("AddMember handler error: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/groups/"+groupID+"/members/bob-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId", "userId")
	c.SetParamValues(groupID, "bob-1")
	c.Set("user_id", "owner-1")

	if err := h.RemoveMember(c); err != nil {
		t.Fatalf("RemoveMember handler error: %v", err)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestBatchInviteByGroupHandler200(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	for _, username := range []string{"bob", "carol"} {
		req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"`+username+`"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("groupId")
		c.SetParamValues(groupID)
		c.Set("user_id", "owner-1")
		if err := h.AddMember(c); err != nil {
			t.Fatalf("AddMember handler error: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations/batch-by-group",
		strings.NewReader(`{"group_id":"`+groupID+`","role":"member"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "owner-1")

	if err := h.BatchInviteByGroup(c); err != nil {
		t.Fatalf("BatchInviteByGroup handler error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchInviteByGroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if len(resp.Invited) != 2 {
		t.Fatalf("expected 2 invited, got %d: %+v", len(resp.Invited), resp)
	}
	if len(resp.Skipped) != 0 {
		t.Fatalf("expected 0 skipped, got %d: %+v", len(resp.Skipped), resp)
	}
}

func TestBatchInviteByGroupHandlerInvalidRole400(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations/batch-by-group",
		strings.NewReader(`{"group_id":"`+groupID+`","role":"master"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "owner-1")

	if err := h.BatchInviteByGroup(c); err != nil {
		t.Fatalf("BatchInviteByGroup handler error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for role=master, got %d", rec.Code)
	}
}

func TestBatchInviteByGroupHandlerForbiddenWhenCallerDoesNotOwnGroup(t *testing.T) {
	e, h, _, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations/batch-by-group",
		strings.NewReader(`{"group_id":"`+groupID+`","role":"member"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("roomId")
	c.SetParamValues("room-1")
	c.Set("user_id", "bob-1")

	if err := h.BatchInviteByGroup(c); err != nil {
		t.Fatalf("BatchInviteByGroup handler error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

// TestBatchInviteByGroupHandlerPartialFailure500 exercises the case where
// groupusecase.GroupUsecase.BatchInviteToRoom aborts partway through with a
// non-nil (result, err): the handler must render the accumulated partial
// result (here, bob's already-created invitation) with an explicit failure
// indication, rather than discarding it behind a bare error body.
func TestBatchInviteByGroupHandlerPartialFailure500(t *testing.T) {
	e, h, groupRepo, _, _ := setupGroupTest()
	groupID := createTestGroup(t, e, h, "owner-1")

	req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID+"/members", strings.NewReader(`{"username":"bob"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("groupId")
	c.SetParamValues(groupID)
	c.Set("user_id", "owner-1")
	if err := h.AddMember(c); err != nil {
		t.Fatalf("AddMember handler error: %v", err)
	}

	// Seed a second group member directly whose username does not resolve
	// to any registered user, so the second CreateInvitation call fails
	// with domain.ErrNotFound -- an unrecognized (non-skippable) error --
	// after the first (bob) has already succeeded.
	groupRepo.SeedMember(groupID, "ghost-1", "ghost")

	req2 := httptest.NewRequest(http.MethodPost, "/rooms/room-1/invitations/batch-by-group",
		strings.NewReader(`{"group_id":"`+groupID+`","role":"member"}`))
	req2.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("roomId")
	c2.SetParamValues("room-1")
	c2.Set("user_id", "owner-1")

	if err := h.BatchInviteByGroup(c2); err != nil {
		t.Fatalf("BatchInviteByGroup handler error: %v", err)
	}
	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var resp BatchInviteByGroupResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if !resp.Failed || resp.Error == "" {
		t.Fatalf("expected Failed=true with a non-empty Error, got %+v", resp)
	}
	if len(resp.Invited) != 1 {
		t.Fatalf("expected bob's invitation preserved in the partial result, got %+v", resp.Invited)
	}
}
